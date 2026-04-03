// Package breaker provides a circuit breaker abstraction backed by gobreaker.
package breaker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"
)

// ErrOpen is returned when the circuit breaker is open and rejecting calls.
var ErrOpen = errors.New("breaker: circuit open")

// State represents the circuit breaker state.
type State string

const (
	StateClosed   State = "closed"    // normal — requests flow through
	StateOpen     State = "open"      // failing — requests rejected immediately
	StateHalfOpen State = "half-open" // testing recovery — limited requests allowed
)

// Counts holds circuit breaker request counters. Passed to ReadyToTrip.
type Counts = gobreaker.Counts

// Breaker wraps an operation with circuit breaker logic.
type Breaker interface {
	Execute(ctx context.Context, fn func() error) error
	State() State
}

type breaker struct {
	cb *gobreaker.CircuitBreaker[any]
}

type config struct {
	maxRequests uint32
	interval    time.Duration
	timeout     time.Duration
	readyToTrip func(counts Counts) bool
}

// Option configures a Breaker.
type Option func(*config)

// WithMaxRequests sets max requests allowed in half-open state.
func WithMaxRequests(n uint32) Option { return func(c *config) { c.maxRequests = n } }

// WithInterval sets the cyclic period in closed state to clear counts.
// If 0, counts are never cleared while closed.
func WithInterval(d time.Duration) Option { return func(c *config) { c.interval = d } }

// WithTimeout sets how long the breaker stays open before transitioning to half-open.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithReadyToTrip sets a custom function to decide when to trip the breaker.
// Default: trip after 5 consecutive failures.
func WithReadyToTrip(fn func(counts Counts) bool) Option {
	return func(c *config) { c.readyToTrip = fn }
}

// New creates a Breaker backed by gobreaker.
func New(name string, opts ...Option) Breaker {
	cfg := &config{
		maxRequests: 1,
		interval:    0,
		timeout:     60 * time.Second,
		readyToTrip: func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}
	for _, opt := range opts {
		opt(cfg)
	}

	cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        name,
		MaxRequests: cfg.maxRequests,
		Interval:    cfg.interval,
		Timeout:     cfg.timeout,
		ReadyToTrip: cfg.readyToTrip,
	})

	return &breaker{cb: cb}
}

func (b *breaker) Execute(ctx context.Context, fn func() error) error {
	_, err := b.cb.Execute(func() (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, fn()
	})
	if err == nil {
		return nil
	}
	// Return context errors unwrapped — the breaker didn't cause the cancellation.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Fast path for open-circuit rejection — avoid fmt.Errorf allocation.
	if errors.Is(err, gobreaker.ErrOpenState) {
		return ErrOpen
	}
	return fmt.Errorf("breaker: %w", err)
}

func (b *breaker) State() State {
	switch b.cb.State() {
	case gobreaker.StateClosed:
		return StateClosed
	case gobreaker.StateOpen:
		return StateOpen
	case gobreaker.StateHalfOpen:
		return StateHalfOpen
	default:
		return StateClosed
	}
}
