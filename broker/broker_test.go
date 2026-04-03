package broker

import (
	"context"
	"errors"
	"testing"

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
