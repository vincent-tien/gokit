package broker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPublisher records published messages.
type mockPublisher struct {
	msgs []Message
	err  error
}

func (m *mockPublisher) Publish(_ context.Context, msgs ...Message) error {
	if m.err != nil {
		return m.err
	}
	m.msgs = append(m.msgs, msgs...)
	return nil
}

func (m *mockPublisher) Close() error { return nil }

// mockHandler records calls.
type mockHandler struct {
	msgs []Message
	err  error
}

func (h *mockHandler) Handle(_ context.Context, msg Message) error {
	h.msgs = append(h.msgs, msg)
	return h.err
}

func TestMessage_Fields(t *testing.T) {
	m := Message{
		ID:      "msg-1",
		Topic:   "orders.created",
		Key:     "order-123",
		Payload: []byte(`{"id":"123"}`),
		Headers: map[string]string{"X-Source": "test"},
	}

	assert.Equal(t, "msg-1", m.ID)
	assert.Equal(t, "orders.created", m.Topic)
	assert.Equal(t, "order-123", m.Key)
	assert.Equal(t, `{"id":"123"}`, string(m.Payload))
	assert.Equal(t, "test", m.Headers["X-Source"])
}

func TestMockPublisher_PublishesMessages(t *testing.T) {
	pub := &mockPublisher{}
	err := pub.Publish(context.Background(),
		Message{ID: "1", Topic: "t1", Payload: []byte("a")},
		Message{ID: "2", Topic: "t1", Payload: []byte("b")},
	)

	require.NoError(t, err)
	assert.Len(t, pub.msgs, 2)
	assert.Equal(t, "1", pub.msgs[0].ID)
	assert.Equal(t, "2", pub.msgs[1].ID)
}

func TestMockPublisher_ReturnsError(t *testing.T) {
	pub := &mockPublisher{err: errors.New("connection lost")}
	err := pub.Publish(context.Background(), Message{})
	assert.Error(t, err)
}

func TestHandler_CalledWithMessage(t *testing.T) {
	h := &mockHandler{}
	msg := Message{ID: "1", Topic: "t", Payload: []byte("data")}

	err := h.Handle(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, h.msgs, 1)
	assert.Equal(t, "1", h.msgs[0].ID)
}

func TestHandler_PropagatesError(t *testing.T) {
	h := &mockHandler{err: errors.New("processing failed")}
	err := h.Handle(context.Background(), Message{})
	assert.Error(t, err)
}

// Interface satisfaction checks.
func TestNATSPublisher_ImplementsPublisher(t *testing.T) {
	var _ Publisher = (*NATSPublisher)(nil)
}

func TestNATSSubscriber_ImplementsSubscriber(t *testing.T) {
	var _ Subscriber = (*NATSSubscriber)(nil)
}

// ── option.go coverage ───────────────────────────────────────────────────────

func TestApplyOpts_Defaults(t *testing.T) {
	cfg := applyOpts(nil)
	assert.Equal(t, 1, cfg.Concurrency)
	assert.Equal(t, 3, cfg.MaxRetries)
	assert.Equal(t, 30*time.Second, cfg.AckTimeout)
	assert.Equal(t, "", cfg.Group)
}

func TestApplyOpts_AllOptions(t *testing.T) {
	cfg := applyOpts([]SubscribeOption{
		WithGroup("workers"),
		WithConcurrency(5),
		WithMaxRetries(10),
		WithAckTimeout(60 * time.Second),
	})
	assert.Equal(t, "workers", cfg.Group)
	assert.Equal(t, 5, cfg.Concurrency)
	assert.Equal(t, 10, cfg.MaxRetries)
	assert.Equal(t, 60*time.Second, cfg.AckTimeout)
}

// ── config.go coverage ───────────────────────────────────────────────────────

func TestNew_UnknownBackend(t *testing.T) {
	pub, sub, err := New(Config{Backend: "this-backend-does-not-exist"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown backend")
	assert.Nil(t, pub)
	assert.Nil(t, sub)
}

func TestNew_MemoryBackend(t *testing.T) {
	pub, sub, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	require.NotNil(t, pub)
	require.NotNil(t, sub)
}

func TestRegister_AddsBackend(t *testing.T) {
	Register("test-fake-backend", func(_ Config) (Publisher, Subscriber, error) {
		return &mockPublisher{}, nil, nil
	})
	pub, _, err := New(Config{Backend: "test-fake-backend"})
	require.NoError(t, err)
	assert.NotNil(t, pub)
	delete(registry, "test-fake-backend") // clean up so other tests aren't affected
}

// ── memory.go coverage ───────────────────────────────────────────────────────

func TestMemory_PublishToOneSubscriber(t *testing.T) {
	pub, sub, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	var received []Message
	var mu sync.Mutex
	require.NoError(t, sub.Subscribe(context.Background(), "topic.t", func(_ context.Context, msg Message) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}))
	require.NoError(t, pub.Publish(context.Background(), Message{ID: "m1", Topic: "topic.t", Payload: []byte("hi")}))
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "m1", received[0].ID)
}

func TestMemory_ConsumerGroupDedup(t *testing.T) {
	pub, sub, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	var count int32
	handler := func(_ context.Context, _ Message) error {
		atomic.AddInt32(&count, 1)
		return nil
	}
	// Two handlers in the SAME group on the SAME topic — only one should receive each message.
	require.NoError(t, sub.Subscribe(context.Background(), "topic.cg", handler, WithGroup("workers")))
	require.NoError(t, sub.Subscribe(context.Background(), "topic.cg", handler, WithGroup("workers")))
	require.NoError(t, pub.Publish(context.Background(), Message{ID: "m1", Topic: "topic.cg"}))
	assert.Equal(t, int32(1), atomic.LoadInt32(&count), "consumer group should deliver to only one handler")
}

func TestMemory_FanoutAcrossGroups(t *testing.T) {
	pub, sub, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	var count int32
	handler := func(_ context.Context, _ Message) error {
		atomic.AddInt32(&count, 1)
		return nil
	}
	require.NoError(t, sub.Subscribe(context.Background(), "topic.fan", handler, WithGroup("group-a")))
	require.NoError(t, sub.Subscribe(context.Background(), "topic.fan", handler, WithGroup("group-b")))
	require.NoError(t, pub.Publish(context.Background(), Message{ID: "m1", Topic: "topic.fan"}))
	assert.Equal(t, int32(2), atomic.LoadInt32(&count), "different groups should each receive one copy")
}

func TestMemory_NoGroupFanout(t *testing.T) {
	// Empty group → fan-out to every handler regardless.
	pub, sub, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	var count int32
	handler := func(_ context.Context, _ Message) error {
		atomic.AddInt32(&count, 1)
		return nil
	}
	require.NoError(t, sub.Subscribe(context.Background(), "topic.nofan", handler))
	require.NoError(t, sub.Subscribe(context.Background(), "topic.nofan", handler))
	require.NoError(t, pub.Publish(context.Background(), Message{ID: "m1", Topic: "topic.nofan"}))
	assert.Equal(t, int32(2), atomic.LoadInt32(&count), "no group should fan out to every handler")
}

func TestMemory_Close_NoOp(t *testing.T) {
	pub, sub, err := New(Config{Backend: "memory"})
	require.NoError(t, err)
	require.NoError(t, pub.Close())
	require.NoError(t, sub.Close())
}

// ── nats.go constructor + close coverage (no live server required) ────────────

func TestNATSPublisher_NewAndClose(t *testing.T) {
	// NewNATSPublisher and Close do not touch the JetStream connection.
	p := NewNATSPublisher(nil)
	require.NotNil(t, p)
	require.NoError(t, p.Close())
}

func TestNATSSubscriber_NewAndClose(t *testing.T) {
	// NewNATSSubscriber and Close (empty cons) do not touch the JetStream connection.
	s := NewNATSSubscriber(nil)
	require.NotNil(t, s)
	require.NoError(t, s.Close())
}
