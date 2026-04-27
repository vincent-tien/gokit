// Package event tests EventMeta + Envelope + InProcessDispatcher behavior.
package event

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMeta_PopulatesIDAndTime(t *testing.T) {
	before := time.Now()
	m := NewMeta("invoice.published", 1)
	after := time.Now()

	assert.NotEmpty(t, m.ID)
	assert.Equal(t, "invoice.published", m.Name)
	assert.Equal(t, 1, m.Version)
	assert.True(t, !m.OccurredAt.Before(before) && !m.OccurredAt.After(after),
		"OccurredAt %v should be between %v and %v", m.OccurredAt, before, after)
}

func TestWithCorrelation_ReturnsNewValue(t *testing.T) {
	original := NewMeta("test.event", 1)
	updated := original.WithCorrelation("c1", "c2")

	// original is immutable
	assert.Empty(t, original.CorrelationID)
	assert.Empty(t, original.CausationID)

	assert.Equal(t, "c1", updated.CorrelationID)
	assert.Equal(t, "c2", updated.CausationID)
	// other fields preserved
	assert.Equal(t, original.ID, updated.ID)
	assert.Equal(t, original.Name, updated.Name)
}

func TestWithSource_ReturnsNewValue(t *testing.T) {
	original := NewMeta("test.event", 1)
	updated := original.WithSource("billing-service")

	// original is immutable
	assert.Empty(t, original.Source)
	assert.Equal(t, "billing-service", updated.Source)
	// other fields preserved
	assert.Equal(t, original.ID, updated.ID)
}

func TestNewEnvelope_RoundTrip(t *testing.T) {
	type Body struct {
		Foo string `json:"foo"`
		Bar int    `json:"bar"`
	}

	body := Body{Foo: "hello", Bar: 42}
	env, err := NewEnvelope(NewMeta("test.event", 1), body)
	require.NoError(t, err)
	require.NotEmpty(t, env.Payload)

	var got Body
	require.NoError(t, env.DecodePayload(&got))
	assert.Equal(t, body, got)
}

func TestEnvelope_ToBrokerMessage_HeadersSet(t *testing.T) {
	meta := NewMeta("foo.bar", 2).WithCorrelation("corr", "caus")
	env, err := NewEnvelope(meta, struct{}{})
	require.NoError(t, err)

	msg, err := env.ToBrokerMessage("topic.foo", "key-1")
	require.NoError(t, err)

	assert.Equal(t, "topic.foo", msg.Topic)
	assert.Equal(t, "key-1", msg.Key)
	assert.Equal(t, env.Meta.ID, msg.ID)
	assert.Equal(t, "foo.bar", msg.Headers["event_name"])
	assert.Equal(t, "2", msg.Headers["event_version"])
	assert.Equal(t, "corr", msg.Headers["correlation_id"])
	assert.Equal(t, "caus", msg.Headers["causation_id"])
}

func TestFromBrokerMessage_ParsesEnvelope(t *testing.T) {
	type Body struct {
		Value string `json:"value"`
	}

	body := Body{Value: "round-trip"}
	original, err := NewEnvelope(NewMeta("msg.received", 1).WithCorrelation("cx", "ca"), body)
	require.NoError(t, err)

	msg, err := original.ToBrokerMessage("topic.msg", "key-x")
	require.NoError(t, err)

	recovered, err := FromBrokerMessage(msg)
	require.NoError(t, err)

	assert.Equal(t, original.Meta.Name, recovered.Meta.Name)
	assert.Equal(t, original.Meta.ID, recovered.Meta.ID)
	assert.Equal(t, original.Meta.CorrelationID, recovered.Meta.CorrelationID)

	var gotBody Body
	require.NoError(t, recovered.DecodePayload(&gotBody))
	assert.Equal(t, body, gotBody)
}

func TestInProcessDispatcher_FanOut(t *testing.T) {
	d := NewInProcessDispatcher()
	var calls []string
	ctx := context.Background()

	d.On("e1", func(_ context.Context, _ Envelope) error {
		calls = append(calls, "h1")
		return nil
	})
	d.On("e1", func(_ context.Context, _ Envelope) error {
		calls = append(calls, "h2")
		return nil
	})
	d.On("e2", func(_ context.Context, _ Envelope) error {
		calls = append(calls, "h3")
		return nil
	})

	env1, err := NewEnvelope(NewMeta("e1", 1), struct{}{})
	require.NoError(t, err)

	require.NoError(t, d.Dispatch(ctx, env1))
	assert.Equal(t, []string{"h1", "h2"}, calls)
}

func TestInProcessDispatcher_HandlerError(t *testing.T) {
	d := NewInProcessDispatcher()
	var called []string
	ctx := context.Background()

	d.On("e1", func(_ context.Context, _ Envelope) error {
		called = append(called, "h1")
		return errors.New("boom")
	})
	d.On("e1", func(_ context.Context, _ Envelope) error {
		called = append(called, "h2")
		return nil
	})

	env1, err := NewEnvelope(NewMeta("e1", 1), struct{}{})
	require.NoError(t, err)

	dispatchErr := d.Dispatch(ctx, env1)
	require.Error(t, dispatchErr)
	assert.Contains(t, dispatchErr.Error(), "dispatch e1")
	assert.Equal(t, []string{"h1"}, called) // h2 must NOT have run
}

// ---------------------------------------------------------------------------
// OutboxDispatcher tests — use fake Storer + fake TxFromContext to avoid
// importing the outbox package (which would create an import cycle).
// ---------------------------------------------------------------------------

// fakeStorer captures Store calls and optionally returns a configured error.
type fakeStorer struct {
	calls []fakeStoreCall
	err   error
}

type fakeStoreCall struct {
	topic string
	key   string
	envs  []Envelope
}

func (f *fakeStorer) Store(_ context.Context, _ *sql.Tx, topic, key string, events ...Envelope) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, fakeStoreCall{
		topic: topic,
		key:   key,
		envs:  append([]Envelope(nil), events...),
	})
	return nil
}

// fakeTxCtx returns a fixed (*sql.Tx, bool) pair.
type fakeTxCtx struct {
	tx *sql.Tx
	ok bool
}

func (f *fakeTxCtx) TxFromContext(_ context.Context) (*sql.Tx, bool) {
	return f.tx, f.ok
}

func TestOutboxDispatcher_NoTx_ReturnsError(t *testing.T) {
	storer := &fakeStorer{}
	txCtx := &fakeTxCtx{tx: nil, ok: false}
	d := NewOutboxDispatcher(storer, txCtx, "billing-service")

	env, err := NewEnvelope(NewMeta("invoice.published", 1), struct{}{})
	require.NoError(t, err)

	dispatchErr := d.Dispatch(context.Background(), env)
	require.Error(t, dispatchErr)
	assert.Contains(t, dispatchErr.Error(), "no tx in context")
	assert.Empty(t, storer.calls, "must not call Storer when no tx")
}

func TestOutboxDispatcher_StoresPerEvent(t *testing.T) {
	storer := &fakeStorer{}
	// new(sql.Tx) is a non-nil placeholder; fakeStorer never dereferences it.
	tx := new(sql.Tx)
	txCtx := &fakeTxCtx{tx: tx, ok: true}
	d := NewOutboxDispatcher(storer, txCtx, "billing-service")

	env1, err := NewEnvelope(NewMeta("invoice.published", 1), map[string]any{"id": "inv-1"})
	require.NoError(t, err)
	env1.Meta.Headers = map[string]string{"entity_id": "inv-1"}

	env2, err := NewEnvelope(NewMeta("invoice.paid", 1), map[string]any{"id": "inv-2"})
	require.NoError(t, err)
	env2.Meta.Headers = map[string]string{"entity_id": "inv-2"}

	require.NoError(t, d.Dispatch(context.Background(), env1, env2))

	require.Len(t, storer.calls, 2)
	assert.Equal(t, "invoice.published", storer.calls[0].topic)
	assert.Equal(t, "inv-1", storer.calls[0].key)
	assert.Equal(t, "billing-service", storer.calls[0].envs[0].Meta.Source)

	assert.Equal(t, "invoice.paid", storer.calls[1].topic)
	assert.Equal(t, "inv-2", storer.calls[1].key)
	assert.Equal(t, "billing-service", storer.calls[1].envs[0].Meta.Source)
}

func TestOutboxDispatcher_AppliesSource(t *testing.T) {
	storer := &fakeStorer{}
	tx := new(sql.Tx)
	txCtx := &fakeTxCtx{tx: tx, ok: true}
	d := NewOutboxDispatcher(storer, txCtx, "shipping-service")

	env, err := NewEnvelope(NewMeta("shipment.created", 1), struct{}{})
	require.NoError(t, err)

	require.NoError(t, d.Dispatch(context.Background(), env))
	require.Len(t, storer.calls, 1)
	assert.Equal(t, "shipping-service", storer.calls[0].envs[0].Meta.Source)
}

