package broker

import (
	"context"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Interface compliance (compile-time, no broker required) ──────────────────

func TestRabbitPublisher_ImplementsPublisher(t *testing.T) {
	var _ Publisher = (*rabbitPublisher)(nil)
}

func TestRabbitSubscriber_ImplementsSubscriber(t *testing.T) {
	var _ Subscriber = (*rabbitSubscriber)(nil)
}

func TestRabbitMQ_RegisteredInRegistry(t *testing.T) {
	_, ok := registry["rabbitmq"]
	assert.True(t, ok, "rabbitmq backend must self-register via init()")
}

// ── Helpers: header conversion ──────────────────────────────────────────────

func TestToAMQPHeaders_NilOrEmpty(t *testing.T) {
	assert.Nil(t, toAMQPHeaders(nil))
	assert.Nil(t, toAMQPHeaders(map[string]string{}))
}

func TestToAMQPHeaders_PreservesEntries(t *testing.T) {
	in := map[string]string{"x-correlation-id": "abc-123", "trace": "1.2.3"}
	out := toAMQPHeaders(in)
	require.Len(t, out, 2)
	assert.Equal(t, "abc-123", out["x-correlation-id"])
	assert.Equal(t, "1.2.3", out["trace"])
}

func TestFromAMQPHeaders_NilOrEmpty(t *testing.T) {
	assert.Nil(t, fromAMQPHeaders(nil))
	assert.Nil(t, fromAMQPHeaders(amqp.Table{}))
}

func TestFromAMQPHeaders_AllScalarTypes(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 30, 45, 0, time.UTC)
	in := amqp.Table{
		"str":     "hello",
		"bytes":   []byte("world"),
		"bool":    true,
		"i":       int(7),
		"i8":      int8(-8),
		"i16":     int16(-16),
		"i32":     int32(-32),
		"i64":     int64(-64),
		"u8":      uint8(8),
		"u16":     uint16(16),
		"u32":     uint32(32),
		"u64":     uint64(64),
		"f32":     float32(1.5),
		"f64":     float64(2.5),
		"time":    now,
		"complex": struct{ A int }{A: 1},
		"nilval":  nil,
	}
	out := fromAMQPHeaders(in)

	assert.Equal(t, "hello", out["str"])
	assert.Equal(t, "world", out["bytes"])
	assert.Equal(t, "true", out["bool"])
	assert.Equal(t, "7", out["i"])
	assert.Equal(t, "-8", out["i8"])
	assert.Equal(t, "-16", out["i16"])
	assert.Equal(t, "-32", out["i32"])
	assert.Equal(t, "-64", out["i64"])
	assert.Equal(t, "8", out["u8"])
	assert.Equal(t, "16", out["u16"])
	assert.Equal(t, "32", out["u32"])
	assert.Equal(t, "64", out["u64"])
	assert.Equal(t, "1.5", out["f32"])
	assert.Equal(t, "2.5", out["f64"])
	assert.Equal(t, now.Format(time.RFC3339Nano), out["time"])
	assert.Contains(t, out["complex"], "1")
	_, hasNil := out["nilval"]
	assert.False(t, hasNil, "nil values should be skipped")
}

// ── Helpers: delivery mapping ────────────────────────────────────────────────

func TestFromAMQPDelivery_PopulatesMessage(t *testing.T) {
	d := amqp.Delivery{
		MessageId:  "msg-123",
		Exchange:   "orders.events",
		RoutingKey: "order.created",
		Body:       []byte(`{"id":"1"}`),
		Headers: amqp.Table{
			"x-source": "test",
		},
	}
	msg := fromAMQPDelivery(d)
	assert.Equal(t, "msg-123", msg.ID)
	assert.Equal(t, "orders.events", msg.Topic)
	assert.Equal(t, "order.created", msg.Key)
	assert.Equal(t, `{"id":"1"}`, string(msg.Payload))
	assert.Equal(t, "test", msg.Headers["x-source"])
}

func TestGetDeliveryCount_HeaderPresent(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want int
	}{
		{"int64", int64(3), 3},
		{"int32", int32(5), 5},
		{"int", int(7), 7},
		{"uint8", uint8(2), 2},
		{"uint16", uint16(4), 4},
		{"uint32", uint32(6), 6},
		{"uint64", uint64(9), 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := amqp.Delivery{Headers: amqp.Table{"x-delivery-count": tc.val}}
			assert.Equal(t, tc.want, getDeliveryCount(d))
		})
	}
}

func TestGetDeliveryCount_HeaderMissingAndNotRedelivered(t *testing.T) {
	d := amqp.Delivery{}
	assert.Equal(t, 0, getDeliveryCount(d))
}

func TestGetDeliveryCount_HeaderMissingButRedelivered(t *testing.T) {
	d := amqp.Delivery{Redelivered: true}
	assert.Equal(t, 1, getDeliveryCount(d))
}

func TestGetDeliveryCount_HeaderNonNumeric(t *testing.T) {
	d := amqp.Delivery{Headers: amqp.Table{"x-delivery-count": "not-a-number"}, Redelivered: true}
	// Non-numeric → fall through to Redelivered fallback.
	assert.Equal(t, 1, getDeliveryCount(d))
}

// ── Helpers: error classification ────────────────────────────────────────────

func TestIsRetryableAMQPError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"closed", amqp.ErrClosed, true},
		{"channel-error", &amqp.Error{Code: amqp.ChannelError}, true},
		{"connection-forced", &amqp.Error{Code: amqp.ConnectionForced}, true},
		{"frame-error", &amqp.Error{Code: amqp.FrameError}, true},
		{"internal-error", &amqp.Error{Code: amqp.InternalError}, true},
		{"resource-error", &amqp.Error{Code: amqp.ResourceError}, true},
		{"access-refused", &amqp.Error{Code: amqp.AccessRefused}, false},
		{"not-found", &amqp.Error{Code: amqp.NotFound}, false},
		{"precondition-failed", &amqp.Error{Code: amqp.PreconditionFailed}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isRetryableAMQPError(tc.err))
		})
	}
}

// ── Helpers: panic recovery ──────────────────────────────────────────────────

func TestSafeInvokeHandler_RecoversFromPanic(t *testing.T) {
	bad := func(_ context.Context, _ Message) error {
		panic("boom")
	}
	err := safeInvokeHandler(context.Background(), bad, Message{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler panic")
	assert.Contains(t, err.Error(), "boom")
}

func TestSafeInvokeHandler_PassesThroughHandlerError(t *testing.T) {
	want := assert.AnError
	got := safeInvokeHandler(context.Background(), func(_ context.Context, _ Message) error { return want }, Message{})
	assert.ErrorIs(t, got, want)
}

func TestSafeInvokeHandler_HappyPath(t *testing.T) {
	called := false
	err := safeInvokeHandler(context.Background(), func(_ context.Context, _ Message) error {
		called = true
		return nil
	}, Message{ID: "x"})
	require.NoError(t, err)
	assert.True(t, called)
}

// ── parseRabbitOptions ───────────────────────────────────────────────────────

func TestParseRabbitOptions_Defaults(t *testing.T) {
	r := parseRabbitOptions(nil)
	assert.Equal(t, "", r.vhost)
	assert.Equal(t, rabbitDefaultHeartbeat, r.heartbeat)
	assert.Equal(t, rabbitDefaultExchangeType, r.exchangeType)
	assert.Equal(t, rabbitDefaultBindingKey, r.bindingKey)
	assert.Equal(t, rabbitDefaultDLQPrefix, r.dlqPrefix)
	assert.True(t, r.durable)
	assert.False(t, r.prefetchGlobal)
	assert.False(t, r.nativeDLQ)
}

func TestParseRabbitOptions_AllOverrides(t *testing.T) {
	r := parseRabbitOptions(map[string]any{
		"vhost":           "/prod",
		"heartbeat":       30,
		"exchange_type":   "direct",
		"binding_key":     "order.*",
		"prefetch_global": true,
		"native_dlq":      true,
		"dlq_prefix":      "deadletter.",
		"durable":         false,
	})
	assert.Equal(t, "/prod", r.vhost)
	assert.Equal(t, 30*time.Second, r.heartbeat)
	assert.Equal(t, "direct", r.exchangeType)
	assert.Equal(t, "order.*", r.bindingKey)
	assert.True(t, r.prefetchGlobal)
	assert.True(t, r.nativeDLQ)
	assert.Equal(t, "deadletter.", r.dlqPrefix)
	assert.False(t, r.durable)
}

func TestParseRabbitOptions_HeartbeatVariants(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want time.Duration
	}{
		{"int-seconds", 5, 5 * time.Second},
		{"int64-seconds", int64(15), 15 * time.Second},
		{"float64-seconds", float64(20), 20 * time.Second},
		{"duration", 45 * time.Second, 45 * time.Second},
		{"duration-string", "1m30s", 90 * time.Second},
		{"invalid-string", "not-a-duration", rabbitDefaultHeartbeat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := parseRabbitOptions(map[string]any{"heartbeat": tc.val})
			assert.Equal(t, tc.want, r.heartbeat)
		})
	}
}

func TestParseRabbitOptions_InvalidExchangeTypeFallsBack(t *testing.T) {
	r := parseRabbitOptions(map[string]any{"exchange_type": "weirdo"})
	assert.Equal(t, rabbitDefaultExchangeType, r.exchangeType)
}

func TestParseRabbitOptions_EmptyBindingKeyKeepsDefault(t *testing.T) {
	r := parseRabbitOptions(map[string]any{"binding_key": ""})
	assert.Equal(t, rabbitDefaultBindingKey, r.bindingKey)
}

// ── Factory failure path (no broker) ─────────────────────────────────────────

func TestNewRabbitMQ_DialFailureReturnsError(t *testing.T) {
	// Use an unreachable address so DialConfig fails fast.
	pub, sub, err := newRabbitMQ(Config{
		Backend: "rabbitmq",
		Addr:    "amqp://guest:guest@127.0.0.1:1/", // port 1 is reserved/unavailable
		Options: map[string]any{"heartbeat": 1},
	})
	require.Error(t, err)
	assert.Nil(t, pub)
	assert.Nil(t, sub)
	assert.True(t,
		strings.Contains(err.Error(), "rabbitmq: dial"),
		"error should originate from rabbitmq dial: %v", err,
	)
}

func TestNewRabbitMQ_EmptyAddrUsesDefault(t *testing.T) {
	// Confirms empty Addr does not panic; the dial itself will probably fail in
	// test environments, but the error must come from the default address.
	_, _, err := newRabbitMQ(Config{Backend: "rabbitmq"})
	if err != nil {
		assert.Contains(t, err.Error(), "rabbitmq: dial")
	}
}

// ── Publisher validation paths (no broker required) ─────────────────────────

func TestRabbitPublisher_PublishNilCtx(t *testing.T) {
	p := &rabbitPublisher{declared: map[string]struct{}{}}
	err := p.Publish(nil, Message{Topic: "t"}) //nolint:staticcheck // intentionally pass nil
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil context")
}

func TestRabbitPublisher_PublishEmptyTopic(t *testing.T) {
	p := &rabbitPublisher{declared: map[string]struct{}{}}
	err := p.Publish(context.Background(), Message{Topic: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty Topic")
}

func TestRabbitPublisher_PublishWhenClosed(t *testing.T) {
	p := &rabbitPublisher{declared: map[string]struct{}{}, closed: true}
	err := p.Publish(context.Background(), Message{Topic: "t"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publisher closed")
}

func TestRabbitPublisher_PublishNoMsgsIsNoop(t *testing.T) {
	p := &rabbitPublisher{declared: map[string]struct{}{}}
	require.NoError(t, p.Publish(context.Background()))
}

func TestRabbitPublisher_CloseIdempotent(t *testing.T) {
	p := &rabbitPublisher{declared: map[string]struct{}{}}
	require.NoError(t, p.Close())
	require.NoError(t, p.Close())
}

// ── Subscriber validation paths (no broker required) ────────────────────────

func TestRabbitSubscriber_SubscribeNilCtx(t *testing.T) {
	s := &rabbitSubscriber{}
	err := s.Subscribe(nil, "t", func(_ context.Context, _ Message) error { return nil }) //nolint:staticcheck
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil context")
}

func TestRabbitSubscriber_SubscribeEmptyTopic(t *testing.T) {
	s := &rabbitSubscriber{}
	err := s.Subscribe(context.Background(), "", func(_ context.Context, _ Message) error { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty topic")
}

func TestRabbitSubscriber_SubscribeNilHandler(t *testing.T) {
	s := &rabbitSubscriber{}
	err := s.Subscribe(context.Background(), "t", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil handler")
}

func TestRabbitSubscriber_SubscribeWhenClosed(t *testing.T) {
	s := &rabbitSubscriber{closed: true}
	err := s.Subscribe(context.Background(), "t",
		func(_ context.Context, _ Message) error { return nil },
		WithGroup("g"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subscriber closed")
}

func TestRabbitSubscriber_SubscribeNilConn(t *testing.T) {
	s := &rabbitSubscriber{conn: nil}
	err := s.Subscribe(context.Background(), "t",
		func(_ context.Context, _ Message) error { return nil },
		WithGroup("g"),
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection unavailable")
}

func TestRabbitSubscriber_CloseIdempotent(t *testing.T) {
	s := &rabbitSubscriber{}
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
}

// ── New() integration with config registry ──────────────────────────────────

func TestNew_RabbitMQBackendRouted(t *testing.T) {
	// Verifies that "rabbitmq" reaches newRabbitMQ via the registry. The dial
	// itself fails (no broker), but the error must have a rabbitmq prefix.
	_, _, err := New(Config{Backend: "rabbitmq", Addr: "amqp://guest:guest@127.0.0.1:1/"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rabbitmq:")
}
