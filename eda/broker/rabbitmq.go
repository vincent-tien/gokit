package broker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ backend defaults. Override via Config.Options.
const (
	rabbitDefaultAddr          = "amqp://guest:guest@localhost:5672/"
	rabbitDefaultHeartbeat     = 10 * time.Second
	rabbitDefaultExchangeType  = "topic"
	rabbitDefaultBindingKey    = "#"
	rabbitDefaultDLQPrefix     = "dlq."
	rabbitReconnectBaseBackoff = 1 * time.Second
	rabbitMaxReconnectAttempts = 5
	rabbitCloseDrainTimeout    = 30 * time.Second
)

// rabbitOptions holds parsed RabbitMQ-specific settings from Config.Options.
type rabbitOptions struct {
	vhost          string
	heartbeat      time.Duration
	exchangeType   string
	bindingKey     string
	prefetchGlobal bool
	nativeDLQ      bool
	dlqPrefix      string
	durable        bool
}

// parseRabbitOptions extracts RabbitMQ knobs from Config.Options with safe defaults.
// Recognised keys: "vhost", "heartbeat" (int seconds, time.Duration, or duration string),
// "exchange_type" ("topic"/"direct"/"fanout"/"headers"), "binding_key",
// "prefetch_global" (bool), "native_dlq" (bool), "dlq_prefix" (string), "durable" (bool).
func parseRabbitOptions(opts map[string]any) rabbitOptions {
	r := rabbitOptions{
		heartbeat:    rabbitDefaultHeartbeat,
		exchangeType: rabbitDefaultExchangeType,
		bindingKey:   rabbitDefaultBindingKey,
		dlqPrefix:    rabbitDefaultDLQPrefix,
		durable:      true,
	}
	if opts == nil {
		return r
	}
	if v, ok := opts["vhost"].(string); ok {
		r.vhost = v
	}
	switch hb := opts["heartbeat"].(type) {
	case int:
		r.heartbeat = time.Duration(hb) * time.Second
	case int64:
		r.heartbeat = time.Duration(hb) * time.Second
	case float64:
		r.heartbeat = time.Duration(hb) * time.Second
	case time.Duration:
		r.heartbeat = hb
	case string:
		if d, err := time.ParseDuration(hb); err == nil {
			r.heartbeat = d
		}
	}
	if v, ok := opts["exchange_type"].(string); ok {
		switch v {
		case "topic", "direct", "fanout", "headers":
			r.exchangeType = v
		}
	}
	if v, ok := opts["binding_key"].(string); ok && v != "" {
		r.bindingKey = v
	}
	if v, ok := opts["prefetch_global"].(bool); ok {
		r.prefetchGlobal = v
	}
	if v, ok := opts["native_dlq"].(bool); ok {
		r.nativeDLQ = v
	}
	if v, ok := opts["dlq_prefix"].(string); ok && v != "" {
		r.dlqPrefix = v
	}
	if v, ok := opts["durable"].(bool); ok {
		r.durable = v
	}
	return r
}

// newRabbitMQ constructs a RabbitMQ-backed Publisher and Subscriber.
//
// Publisher and Subscriber use separate AMQP connections so a stuck consumer
// cannot stall publishes (and vice-versa). Each Subscribe call opens its own
// channel on the subscriber connection.
func newRabbitMQ(cfg Config) (Publisher, Subscriber, error) {
	addr := cfg.Addr
	if addr == "" {
		addr = rabbitDefaultAddr
	}
	rOpts := parseRabbitOptions(cfg.Options)

	dialCfg := amqp.Config{
		Heartbeat: rOpts.heartbeat,
		Locale:    "en_US",
	}
	if rOpts.vhost != "" {
		dialCfg.Vhost = rOpts.vhost
	}

	pubConn, err := amqp.DialConfig(addr, dialCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("rabbitmq: dial publisher %s: %w", addr, err)
	}
	pubCh, err := pubConn.Channel()
	if err != nil {
		_ = pubConn.Close()
		return nil, nil, fmt.Errorf("rabbitmq: open publisher channel: %w", err)
	}
	if err := pubCh.Confirm(false); err != nil {
		_ = pubCh.Close()
		_ = pubConn.Close()
		return nil, nil, fmt.Errorf("rabbitmq: enable publisher confirms: %w", err)
	}

	subConn, err := amqp.DialConfig(addr, dialCfg)
	if err != nil {
		_ = pubCh.Close()
		_ = pubConn.Close()
		return nil, nil, fmt.Errorf("rabbitmq: dial subscriber %s: %w", addr, err)
	}

	pub := &rabbitPublisher{
		addr:     addr,
		dial:     dialCfg,
		rOpts:    rOpts,
		conn:     pubConn,
		ch:       pubCh,
		declared: make(map[string]struct{}),
	}
	sub := &rabbitSubscriber{
		addr:  addr,
		dial:  dialCfg,
		rOpts: rOpts,
		conn:  subConn,
	}
	return pub, sub, nil
}

func init() { Register("rabbitmq", newRabbitMQ) }

// ── Helpers ──────────────────────────────────────────────────────────────────

// toAMQPHeaders converts a string-string map into amqp.Table for the wire.
// Returns nil for empty input to keep AMQP frames compact.
func toAMQPHeaders(headers map[string]string) amqp.Table {
	if len(headers) == 0 {
		return nil
	}
	table := make(amqp.Table, len(headers))
	for k, v := range headers {
		table[k] = v
	}
	return table
}

// fromAMQPHeaders flattens an amqp.Table into map[string]string. AMQP headers
// can be many scalar types (numeric, time.Time, []byte, bool); this function
// stringifies known types and falls back to fmt.Sprintf for the rest. Nil
// values are dropped so callers see only meaningful headers.
func fromAMQPHeaders(table amqp.Table) map[string]string {
	if len(table) == 0 {
		return nil
	}
	headers := make(map[string]string, len(table))
	for k, v := range table {
		if v == nil {
			continue
		}
		switch val := v.(type) {
		case string:
			headers[k] = val
		case []byte:
			headers[k] = string(val)
		case bool:
			headers[k] = strconv.FormatBool(val)
		case int:
			headers[k] = strconv.Itoa(val)
		case int8:
			headers[k] = strconv.FormatInt(int64(val), 10)
		case int16:
			headers[k] = strconv.FormatInt(int64(val), 10)
		case int32:
			headers[k] = strconv.FormatInt(int64(val), 10)
		case int64:
			headers[k] = strconv.FormatInt(val, 10)
		case uint8:
			headers[k] = strconv.FormatUint(uint64(val), 10)
		case uint16:
			headers[k] = strconv.FormatUint(uint64(val), 10)
		case uint32:
			headers[k] = strconv.FormatUint(uint64(val), 10)
		case uint64:
			headers[k] = strconv.FormatUint(val, 10)
		case float32:
			headers[k] = strconv.FormatFloat(float64(val), 'f', -1, 32)
		case float64:
			headers[k] = strconv.FormatFloat(val, 'f', -1, 64)
		case time.Time:
			headers[k] = val.UTC().Format(time.RFC3339Nano)
		default:
			headers[k] = fmt.Sprintf("%v", val)
		}
	}
	return headers
}

// fromAMQPDelivery maps an AMQP delivery into the broker.Message contract.
func fromAMQPDelivery(d amqp.Delivery) Message {
	return Message{
		ID:      d.MessageId,
		Topic:   d.Exchange,
		Key:     d.RoutingKey,
		Payload: d.Body,
		Headers: fromAMQPHeaders(d.Headers),
	}
}

// getDeliveryCount returns how many times this message has been delivered.
// Quorum queues set the x-delivery-count header (0 on first delivery, ≥1 on
// redelivery). Classic queues only expose the boolean Redelivered flag, so we
// approximate as 0 (fresh) or 1 (redelivered) when the header is missing.
func getDeliveryCount(d amqp.Delivery) int {
	if d.Headers != nil {
		if v, ok := d.Headers["x-delivery-count"]; ok {
			switch n := v.(type) {
			case int64:
				return int(n)
			case int32:
				return int(n)
			case int:
				return n
			case uint8:
				return int(n)
			case uint16:
				return int(n)
			case uint32:
				return int(n)
			case uint64:
				return int(n)
			}
		}
	}
	if d.Redelivered {
		return 1
	}
	return 0
}

// isRetryableAMQPError reports whether err looks like a transient channel or
// connection failure that warrants a reconnect-and-retry attempt. Non-retryable
// errors (auth, access refused, precondition) bubble up to the caller.
func isRetryableAMQPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, amqp.ErrClosed) {
		return true
	}
	var amqpErr *amqp.Error
	if !errors.As(err, &amqpErr) {
		return false
	}
	switch amqpErr.Code {
	case amqp.ChannelError,
		amqp.ConnectionForced,
		amqp.FrameError,
		amqp.InternalError,
		amqp.ResourceError:
		return true
	}
	return false
}

// safeInvokeHandler protects worker goroutines from handler panics so a single
// bad message cannot terminate the subscriber loop.
func safeInvokeHandler(ctx context.Context, h Handler, m Message) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("rabbitmq: handler panic: %v", r)
		}
	}()
	return h(ctx, m)
}
