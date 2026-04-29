package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// rabbitSubscriber owns one AMQP connection and opens a fresh channel per
// Subscribe call. Workers are tracked in a WaitGroup so Close drains in-flight
// handlers before tearing down resources.
type rabbitSubscriber struct {
	addr  string
	dial  amqp.Config
	rOpts rabbitOptions

	mu       sync.Mutex // guards conn, channels, cancels, closed
	conn     *amqp.Connection
	channels []*amqp.Channel
	cancels  []context.CancelFunc
	wg       sync.WaitGroup
	closed   bool
}

// Subscribe declares the topic exchange, the consumer-group queue (a quorum
// queue by default for replicated durability and built-in poison-message
// handling), binds them, sets QoS to match cfg.Concurrency, and spawns a
// worker pool that consumes deliveries with manual acknowledgement.
//
// Each Subscribe call opens its own AMQP channel because amqp091-go channels
// are not goroutine-safe and Qos is per-channel. Workers spawned for this
// subscription share the channel's deliveries via the Go channel returned by
// Consume; ack/nack/reject methods on amqp.Delivery are safe under the
// library's internal serialisation.
func (s *rabbitSubscriber) Subscribe(
	ctx context.Context,
	topic string,
	handler Handler,
	opts ...SubscribeOption,
) error {
	if ctx == nil {
		return errors.New("rabbitmq: subscribe: nil context")
	}
	if topic == "" {
		return errors.New("rabbitmq: subscribe: empty topic")
	}
	if handler == nil {
		return errors.New("rabbitmq: subscribe: nil handler")
	}

	cfg := applyOpts(opts)
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("rabbitmq: subscriber closed")
	}
	conn := s.conn
	s.mu.Unlock()

	if conn == nil || conn.IsClosed() {
		return errors.New("rabbitmq: subscriber connection unavailable")
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq: open subscriber channel: %w", err)
	}

	queueName, err := s.declareTopology(ch, topic, cfg)
	if err != nil {
		_ = ch.Close()
		return err
	}

	if err := ch.Qos(cfg.Concurrency, 0, s.rOpts.prefetchGlobal); err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq: set qos for %s: %w", queueName, err)
	}

	deliveries, err := ch.Consume(
		queueName,
		"",    // consumer tag (auto-generated)
		false, // auto-ack: manual for reliability
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		_ = ch.Close()
		return fmt.Errorf("rabbitmq: consume %s: %w", queueName, err)
	}

	workerCtx, cancel := context.WithCancel(ctx)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		_ = ch.Close()
		return errors.New("rabbitmq: subscriber closed mid-subscribe")
	}
	s.cancels = append(s.cancels, cancel)
	s.channels = append(s.channels, ch)
	s.mu.Unlock()

	for i := 0; i < cfg.Concurrency; i++ {
		s.wg.Add(1)
		go s.consumeLoop(workerCtx, deliveries, handler, cfg)
	}
	return nil
}

// declareTopology declares the exchange, the queue (quorum + optional native
// DLX wiring), and the binding. Returns the resolved queue name (RabbitMQ may
// assign one when cfg.Group is empty).
func (s *rabbitSubscriber) declareTopology(ch *amqp.Channel, topic string, cfg SubscribeConfig) (string, error) {
	if err := ch.ExchangeDeclare(
		topic,
		s.rOpts.exchangeType,
		s.rOpts.durable,
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return "", fmt.Errorf("rabbitmq: declare exchange %s: %w", topic, err)
	}

	queueArgs := amqp.Table{"x-queue-type": "quorum"}
	if s.rOpts.nativeDLQ {
		dlxName := s.rOpts.dlqPrefix + topic
		if err := ch.ExchangeDeclare(
			dlxName,
			s.rOpts.exchangeType,
			s.rOpts.durable,
			false, false, false, nil,
		); err != nil {
			return "", fmt.Errorf("rabbitmq: declare DLX %s: %w", dlxName, err)
		}
		queueArgs["x-dead-letter-exchange"] = dlxName
		if cfg.MaxRetries > 0 {
			// Quorum queues use x-delivery-limit (not x-max-retries).
			queueArgs["x-delivery-limit"] = int32(cfg.MaxRetries)
		}
	}

	queueName := cfg.Group
	durable := s.rOpts.durable
	exclusive := false
	autoDelete := false
	if queueName == "" {
		// No group → ephemeral exclusive queue with server-generated name.
		// Quorum queues require an explicit name + durable, so fall back to
		// classic queue semantics for the ephemeral case.
		delete(queueArgs, "x-queue-type")
		durable = false
		exclusive = true
		autoDelete = true
	}

	q, err := ch.QueueDeclare(
		queueName,
		durable,
		autoDelete,
		exclusive,
		false, // no-wait
		queueArgs,
	)
	if err != nil {
		return "", fmt.Errorf("rabbitmq: declare queue %q: %w", queueName, err)
	}

	if err := ch.QueueBind(q.Name, s.rOpts.bindingKey, topic, false, nil); err != nil {
		return "", fmt.Errorf("rabbitmq: bind %s -> %s: %w", q.Name, topic, err)
	}
	return q.Name, nil
}

// consumeLoop dispatches deliveries to the handler until the worker context is
// cancelled or the deliveries channel is closed.
func (s *rabbitSubscriber) consumeLoop(
	ctx context.Context,
	deliveries <-chan amqp.Delivery,
	handler Handler,
	cfg SubscribeConfig,
) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-deliveries:
			if !ok {
				return
			}
			s.handleDelivery(ctx, d, handler, cfg)
		}
	}
}

// handleDelivery runs the handler under an optional ack timeout, recovers from
// panics, and routes the delivery to ack / nack-requeue / reject(DLQ) based on
// the outcome and current delivery count.
func (s *rabbitSubscriber) handleDelivery(
	ctx context.Context,
	d amqp.Delivery,
	handler Handler,
	cfg SubscribeConfig,
) {
	msg := fromAMQPDelivery(d)

	hctx := ctx
	if cfg.AckTimeout > 0 {
		var cancel context.CancelFunc
		hctx, cancel = context.WithTimeout(ctx, cfg.AckTimeout)
		defer cancel()
	}

	if err := safeInvokeHandler(hctx, handler, msg); err != nil {
		count := getDeliveryCount(d)
		if cfg.MaxRetries > 0 && count >= cfg.MaxRetries {
			_ = d.Reject(false) // requeue=false → dead-letter or drop
			return
		}
		_ = d.Nack(false, true) // multiple=false, requeue=true
		return
	}

	if err := d.Ack(false); err != nil {
		// Channel likely closed; broker will redeliver. Swallow to keep loop alive.
		_ = err
	}
}

// Close cancels worker contexts, closes consumer channels, waits up to
// rabbitCloseDrainTimeout for in-flight handlers to finish, then closes the
// connection. Idempotent.
func (s *rabbitSubscriber) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	cancels := s.cancels
	channels := s.channels
	conn := s.conn
	s.cancels = nil
	s.channels = nil
	s.mu.Unlock()

	for _, c := range cancels {
		c()
	}

	var errs []error
	for _, ch := range channels {
		if err := ch.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = append(errs, fmt.Errorf("rabbitmq: close subscriber channel: %w", err))
		}
	}

	drained := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(rabbitCloseDrainTimeout):
		errs = append(errs, fmt.Errorf("rabbitmq: timeout waiting %s for workers to drain", rabbitCloseDrainTimeout))
	}

	if conn != nil && !conn.IsClosed() {
		if err := conn.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = append(errs, fmt.Errorf("rabbitmq: close subscriber connection: %w", err))
		}
	}
	return errors.Join(errs...)
}
