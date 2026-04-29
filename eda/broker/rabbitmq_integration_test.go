//go:build integration

package broker_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vincent-tien/gokit/eda/broker"
)

// rabbitTestAddr returns the RabbitMQ test endpoint, defaulting to localhost
// when RABBITMQ_TEST_ADDR is unset. CI overrides this to point at a docker
// service (e.g. amqp://guest:guest@rabbitmq:5672/).
func rabbitTestAddr() string {
	if v := os.Getenv("RABBITMQ_TEST_ADDR"); v != "" {
		return v
	}
	return "amqp://guest:guest@localhost:5672/"
}

// uniqueTopic returns a topic name unique per test run to avoid cross-test
// state in a long-lived broker.
func uniqueTopic(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("test.%s.%d", prefix, time.Now().UnixNano())
}

func newRabbitTestBroker(t *testing.T) (broker.Publisher, broker.Subscriber) {
	t.Helper()
	pub, sub, err := broker.New(broker.Config{
		Backend: "rabbitmq",
		Addr:    rabbitTestAddr(),
	})
	require.NoError(t, err, "failed to connect to RabbitMQ at %s — start docker compose first", rabbitTestAddr())
	t.Cleanup(func() {
		_ = pub.Close()
		_ = sub.Close()
	})
	return pub, sub
}

// TestRabbitMQ_PublishSubscribe — end-to-end: publish, queue receives, ack.
func TestRabbitMQ_PublishSubscribe(t *testing.T) {
	pub, sub := newRabbitTestBroker(t)
	topic := uniqueTopic(t, "pubsub")
	group := "ps-" + topic

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var (
		mu       sync.Mutex
		received []broker.Message
		done     = make(chan struct{}, 1)
	)
	require.NoError(t, sub.Subscribe(ctx, topic, func(_ context.Context, msg broker.Message) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
		return nil
	}, broker.WithGroup(group), broker.WithConcurrency(1)))

	// Allow consumer setup to land before publishing.
	time.Sleep(300 * time.Millisecond)

	require.NoError(t, pub.Publish(ctx, broker.Message{
		ID:      "msg-001",
		Topic:   topic,
		Key:     "order.123",
		Payload: []byte(`{"order_id":"123"}`),
		Headers: map[string]string{"correlation_id": "req-abc"},
	}))

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for delivery")
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 1)
	assert.Equal(t, "msg-001", received[0].ID)
	assert.Equal(t, topic, received[0].Topic)
	assert.Equal(t, "order.123", received[0].Key)
	assert.Equal(t, `{"order_id":"123"}`, string(received[0].Payload))
	assert.Equal(t, "req-abc", received[0].Headers["correlation_id"])
}

// TestRabbitMQ_ConsumerGroup_RoundRobin — two consumers on the same group split
// the work; each message is delivered once.
func TestRabbitMQ_ConsumerGroup_RoundRobin(t *testing.T) {
	pub, sub := newRabbitTestBroker(t)
	topic := uniqueTopic(t, "rr")
	group := "rr-" + topic

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var countA, countB int32
	handlerA := func(_ context.Context, _ broker.Message) error {
		atomic.AddInt32(&countA, 1)
		return nil
	}
	handlerB := func(_ context.Context, _ broker.Message) error {
		atomic.AddInt32(&countB, 1)
		return nil
	}

	require.NoError(t, sub.Subscribe(ctx, topic, handlerA, broker.WithGroup(group)))
	require.NoError(t, sub.Subscribe(ctx, topic, handlerB, broker.WithGroup(group)))
	time.Sleep(500 * time.Millisecond)

	const total = 20
	for i := 0; i < total; i++ {
		require.NoError(t, pub.Publish(ctx, broker.Message{
			ID:    fmt.Sprintf("msg-%d", i),
			Topic: topic,
		}))
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&countA)+atomic.LoadInt32(&countB) >= total {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	a := atomic.LoadInt32(&countA)
	b := atomic.LoadInt32(&countB)
	assert.Equal(t, int32(total), a+b, "all messages should be delivered exactly once")
	assert.Greater(t, a, int32(0), "consumer A should receive at least one message (round-robin)")
	assert.Greater(t, b, int32(0), "consumer B should receive at least one message (round-robin)")
}

// TestRabbitMQ_FanoutAcrossGroups — same topic, two distinct groups → each
// group receives its own copy of every message.
func TestRabbitMQ_FanoutAcrossGroups(t *testing.T) {
	pub, sub := newRabbitTestBroker(t)
	topic := uniqueTopic(t, "fanout")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var groupA, groupB int32
	require.NoError(t, sub.Subscribe(ctx, topic,
		func(_ context.Context, _ broker.Message) error { atomic.AddInt32(&groupA, 1); return nil },
		broker.WithGroup("fanout-a-"+topic)))
	require.NoError(t, sub.Subscribe(ctx, topic,
		func(_ context.Context, _ broker.Message) error { atomic.AddInt32(&groupB, 1); return nil },
		broker.WithGroup("fanout-b-"+topic)))
	time.Sleep(500 * time.Millisecond)

	const total = 5
	for i := 0; i < total; i++ {
		require.NoError(t, pub.Publish(ctx, broker.Message{ID: fmt.Sprintf("msg-%d", i), Topic: topic}))
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&groupA) >= total && atomic.LoadInt32(&groupB) >= total {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, int32(total), atomic.LoadInt32(&groupA), "group A should receive every message")
	assert.Equal(t, int32(total), atomic.LoadInt32(&groupB), "group B should receive every message")
}

// TestRabbitMQ_NackRequeue — handler error → nack(requeue) → message redelivered.
func TestRabbitMQ_NackRequeue(t *testing.T) {
	pub, sub := newRabbitTestBroker(t)
	topic := uniqueTopic(t, "nack")
	group := "nack-" + topic

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var attempts int32
	successCh := make(chan struct{}, 1)
	require.NoError(t, sub.Subscribe(ctx, topic, func(_ context.Context, _ broker.Message) error {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			return fmt.Errorf("simulated failure attempt %d", n)
		}
		select {
		case successCh <- struct{}{}:
		default:
		}
		return nil
	}, broker.WithGroup(group), broker.WithMaxRetries(5)))
	time.Sleep(500 * time.Millisecond)

	require.NoError(t, pub.Publish(ctx, broker.Message{ID: "nack-1", Topic: topic}))

	select {
	case <-successCh:
	case <-time.After(15 * time.Second):
		t.Fatalf("timeout: only %d attempts observed", atomic.LoadInt32(&attempts))
	}
	assert.GreaterOrEqual(t, atomic.LoadInt32(&attempts), int32(3), "handler should retry until success")
}

// TestRabbitMQ_ConcurrencyDeliversInParallel — concurrency=4 → up to 4 in-flight.
func TestRabbitMQ_ConcurrencyDeliversInParallel(t *testing.T) {
	pub, sub := newRabbitTestBroker(t)
	topic := uniqueTopic(t, "conc")
	group := "conc-" + topic

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const workers = 4
	var inFlight, peak int32
	var mu sync.Mutex
	processed := make(chan struct{}, workers)

	require.NoError(t, sub.Subscribe(ctx, topic, func(_ context.Context, _ broker.Message) error {
		cur := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if cur > peak {
			peak = cur
		}
		mu.Unlock()
		time.Sleep(200 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		processed <- struct{}{}
		return nil
	}, broker.WithGroup(group), broker.WithConcurrency(workers)))
	time.Sleep(500 * time.Millisecond)

	for i := 0; i < workers*2; i++ {
		require.NoError(t, pub.Publish(ctx, broker.Message{ID: fmt.Sprintf("c-%d", i), Topic: topic}))
	}

	for i := 0; i < workers*2; i++ {
		select {
		case <-processed:
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout: only %d/%d processed", i, workers*2)
		}
	}

	mu.Lock()
	got := peak
	mu.Unlock()
	assert.GreaterOrEqual(t, got, int32(2), "expected ≥2 concurrent in-flight handlers, peak=%d", got)
}

// TestRabbitMQ_HeadersRoundtrip — headers preserved across the wire.
func TestRabbitMQ_HeadersRoundtrip(t *testing.T) {
	pub, sub := newRabbitTestBroker(t)
	topic := uniqueTopic(t, "hdr")
	group := "hdr-" + topic

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got := make(chan broker.Message, 1)
	require.NoError(t, sub.Subscribe(ctx, topic, func(_ context.Context, m broker.Message) error {
		got <- m
		return nil
	}, broker.WithGroup(group)))
	time.Sleep(300 * time.Millisecond)

	require.NoError(t, pub.Publish(ctx, broker.Message{
		ID:    "hdr-1",
		Topic: topic,
		Headers: map[string]string{
			"x-correlation-id": "corr-xyz",
			"x-source":         "orders",
			"x-trace":          "1.2.3",
		},
	}))

	select {
	case m := <-got:
		assert.Equal(t, "corr-xyz", m.Headers["x-correlation-id"])
		assert.Equal(t, "orders", m.Headers["x-source"])
		assert.Equal(t, "1.2.3", m.Headers["x-trace"])
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for delivery")
	}
}

// TestRabbitMQ_CloseGracefulDrain — Close cancels workers and exits cleanly.
func TestRabbitMQ_CloseGracefulDrain(t *testing.T) {
	pub, sub, err := broker.New(broker.Config{Backend: "rabbitmq", Addr: rabbitTestAddr()})
	require.NoError(t, err)
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	topic := uniqueTopic(t, "close")
	group := "close-" + topic
	require.NoError(t, sub.Subscribe(ctx, topic,
		func(_ context.Context, _ broker.Message) error { return nil },
		broker.WithGroup(group)))

	require.NoError(t, sub.Close())
	// Second Close must be idempotent.
	require.NoError(t, sub.Close())
}
