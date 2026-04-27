package outbox

import "time"

// Logger is the structured logging interface Relay and DLQ accept.
// Implement to bridge into your project's logger (zap, observability.Logger, etc.).
// nil-safe: NewRelay/NewDLQ install a no-op default when not provided.
type Logger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// nopLogger drops every call. Default for Relay/DLQ when WithRelayLogger/WithDLQLogger is not used.
type nopLogger struct{}

func (nopLogger) Info(string, ...any)  {}
func (nopLogger) Error(string, ...any) {}

// RelayOption configures a Relay.
type RelayOption func(*Relay)

// WithMinInterval sets the lower bound of the adaptive poll interval. Default: 10ms.
func WithMinInterval(d time.Duration) RelayOption { return func(r *Relay) { r.minInterval = d } }

// WithMaxInterval sets the upper bound of the adaptive poll interval. Default: 1s.
func WithMaxInterval(d time.Duration) RelayOption { return func(r *Relay) { r.maxInterval = d } }

// WithBatchSize sets how many rows are processed per poll. Default: 100.
func WithBatchSize(n int) RelayOption { return func(r *Relay) { r.batchSize = n } }

// WithRelayLogger sets the structured logger for Relay events.
func WithRelayLogger(l Logger) RelayOption { return func(r *Relay) { r.logger = l } }

// DLQOption configures a DLQ wrapper.
type DLQOption func(*DLQ)

// WithMaxRetries sets the retry threshold before routing to the DLQ topic. Default: 3.
func WithMaxRetries(n int) DLQOption { return func(d *DLQ) { d.maxRetries = n } }

// WithDLQPrefix sets the topic prefix for dead-lettered messages. Default: "dlq.".
func WithDLQPrefix(p string) DLQOption { return func(d *DLQ) { d.dlqPrefix = p } }

// WithDLQLogger sets the structured logger for DLQ events.
func WithDLQLogger(l Logger) DLQOption { return func(d *DLQ) { d.logger = l } }
