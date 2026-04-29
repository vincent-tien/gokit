package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// rabbitPublisher publishes messages over a single AMQP channel guarded by a
// mutex. amqp091-go channels are not goroutine-safe; callers may invoke
// Publish concurrently and the mutex serialises access to the channel.
type rabbitPublisher struct {
	addr  string
	dial  amqp.Config
	rOpts rabbitOptions

	mu       sync.Mutex // guards conn, ch, declared, closed
	conn     *amqp.Connection
	ch       *amqp.Channel
	declared map[string]struct{} // exchanges already declared on the current channel
	closed   bool
}

// Publish sends each message to its Topic exchange using the configured routing
// key. Messages are persistent so they survive a broker restart. On a
// retryable channel/connection error the publisher reconnects with linear
// backoff and retries the failing message once before giving up.
func (p *rabbitPublisher) Publish(ctx context.Context, msgs ...Message) error {
	if ctx == nil {
		return errors.New("rabbitmq: publish: nil context")
	}
	if len(msgs) == 0 {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return errors.New("rabbitmq: publisher closed")
	}

	for i, msg := range msgs {
		if msg.Topic == "" {
			return fmt.Errorf("rabbitmq: publish msg[%d]: empty Topic", i)
		}
		if err := p.publishLocked(ctx, msg); err != nil {
			return err
		}
	}
	return nil
}

// publishLocked declares the exchange (idempotent, cached) and publishes one
// message. On retryable errors it reconnects once and retries. Caller must
// hold p.mu.
func (p *rabbitPublisher) publishLocked(ctx context.Context, msg Message) error {
	if err := p.ensureExchangeLocked(msg.Topic); err != nil {
		if !isRetryableAMQPError(err) {
			return err
		}
		if rErr := p.reconnectLocked(); rErr != nil {
			return rErr
		}
		if err := p.ensureExchangeLocked(msg.Topic); err != nil {
			return err
		}
	}

	publishing := amqp.Publishing{
		MessageId:    msg.ID,
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		Body:         msg.Payload,
		Headers:      toAMQPHeaders(msg.Headers),
	}

	err := p.ch.PublishWithContext(ctx,
		msg.Topic, // exchange
		msg.Key,   // routing key
		false,     // mandatory
		false,     // immediate (deprecated; required by API)
		publishing,
	)
	if err == nil {
		return nil
	}
	if !isRetryableAMQPError(err) {
		return fmt.Errorf("rabbitmq: publish to %s: %w", msg.Topic, err)
	}
	if rErr := p.reconnectLocked(); rErr != nil {
		return rErr
	}
	if err := p.ensureExchangeLocked(msg.Topic); err != nil {
		return err
	}
	if err := p.ch.PublishWithContext(ctx, msg.Topic, msg.Key, false, false, publishing); err != nil {
		return fmt.Errorf("rabbitmq: publish to %s after reconnect: %w", msg.Topic, err)
	}
	return nil
}

// ensureExchangeLocked idempotently declares an exchange and caches the result
// per channel. reconnectLocked clears the cache so the new channel re-declares
// what it needs.
func (p *rabbitPublisher) ensureExchangeLocked(name string) error {
	if _, ok := p.declared[name]; ok {
		return nil
	}
	if err := p.ch.ExchangeDeclare(
		name,
		p.rOpts.exchangeType,
		p.rOpts.durable, // durable
		false,           // auto-delete
		false,           // internal
		false,           // no-wait
		nil,             // args
	); err != nil {
		return fmt.Errorf("rabbitmq: declare exchange %s: %w", name, err)
	}
	p.declared[name] = struct{}{}
	return nil
}

// reconnectLocked tears down the current connection and channel then dials
// again with linear backoff (1s, 2s, 3s, 4s, 5s). Caller must hold p.mu.
func (p *rabbitPublisher) reconnectLocked() error {
	p.declared = make(map[string]struct{})
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}

	var lastErr error
	for attempt := 1; attempt <= rabbitMaxReconnectAttempts; attempt++ {
		conn, err := amqp.DialConfig(p.addr, p.dial)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * rabbitReconnectBaseBackoff)
			continue
		}
		ch, err := conn.Channel()
		if err != nil {
			_ = conn.Close()
			lastErr = err
			time.Sleep(time.Duration(attempt) * rabbitReconnectBaseBackoff)
			continue
		}
		if err := ch.Confirm(false); err != nil {
			_ = ch.Close()
			_ = conn.Close()
			lastErr = err
			time.Sleep(time.Duration(attempt) * rabbitReconnectBaseBackoff)
			continue
		}
		p.conn, p.ch = conn, ch
		return nil
	}
	return fmt.Errorf("rabbitmq: reconnect failed after %d attempts: %w", rabbitMaxReconnectAttempts, lastErr)
}

// Close closes the channel then the connection. Idempotent; subsequent calls
// return nil.
func (p *rabbitPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true

	var errs []error
	if p.ch != nil {
		if err := p.ch.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = append(errs, fmt.Errorf("rabbitmq: close publisher channel: %w", err))
		}
		p.ch = nil
	}
	if p.conn != nil && !p.conn.IsClosed() {
		if err := p.conn.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = append(errs, fmt.Errorf("rabbitmq: close publisher connection: %w", err))
		}
	}
	p.conn = nil
	return errors.Join(errs...)
}
