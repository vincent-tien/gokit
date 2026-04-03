package breaker

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBreaker_Execute_Success(t *testing.T) {
	b := New("test")

	err := b.Execute(context.Background(), func() error {
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, StateClosed, b.State())
}

func TestBreaker_Execute_PropagatesError(t *testing.T) {
	b := New("test")
	opErr := errors.New("operation failed")

	err := b.Execute(context.Background(), func() error {
		return opErr
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, opErr)
}

func TestBreaker_TripsAfterConsecutiveFailures(t *testing.T) {
	b := New("test", WithReadyToTrip(func(counts Counts) bool {
		return counts.ConsecutiveFailures >= 3
	}))

	opErr := errors.New("fail")
	for range 3 {
		_ = b.Execute(context.Background(), func() error { return opErr })
	}

	assert.Equal(t, StateOpen, b.State())

	// Next call should be rejected immediately.
	err := b.Execute(context.Background(), func() error { return nil })
	require.Error(t, err)
}

func TestBreaker_Execute_RespectsContext(t *testing.T) {
	b := New("test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Execute(ctx, func() error {
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestBreaker_State_InitiallyClosed(t *testing.T) {
	b := New("test")
	assert.Equal(t, StateClosed, b.State())
}

func TestBreaker_DefaultTripsAt5(t *testing.T) {
	b := New("test")
	opErr := errors.New("fail")

	for range 4 {
		_ = b.Execute(context.Background(), func() error { return opErr })
	}
	assert.Equal(t, StateClosed, b.State(), "should still be closed after 4 failures")

	_ = b.Execute(context.Background(), func() error { return opErr })
	assert.Equal(t, StateOpen, b.State(), "should trip after 5 failures")
}

func TestBreaker_WithMaxRequests(t *testing.T) {
	b := New("test", WithMaxRequests(3))
	assert.NotNil(t, b)
}
