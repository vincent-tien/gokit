package seed

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDB records Exec calls for testing.
type mockDB struct {
	calls []string
	err   error // if set, Exec returns this error
}

func (m *mockDB) Exec(_ context.Context, sql string, _ ...any) error {
	m.calls = append(m.calls, sql)
	return m.err
}

// silentLogger suppresses output during tests.
type silentLogger struct{}

func (l *silentLogger) Info(_ string, _ ...any)  {}
func (l *silentLogger) Error(_ string, _ ...any) {}

func TestRunner_Run_ExecutesSeedsInOrder(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}))
	runner.Register(
		Seed{Name: "first", Run: func(ctx context.Context, db DB) error {
			return db.Exec(ctx, "INSERT INTO t1 VALUES (1)")
		}},
		Seed{Name: "second", Run: func(ctx context.Context, db DB) error {
			return db.Exec(ctx, "INSERT INTO t2 VALUES (2)")
		}},
		Seed{Name: "third", Run: func(ctx context.Context, db DB) error {
			return db.Exec(ctx, "INSERT INTO t3 VALUES (3)")
		}},
	)

	result, err := runner.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.Success)
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, 0, result.Skipped)
	assert.Empty(t, result.Errors)
	assert.True(t, result.Duration > 0)

	assert.Equal(t, []string{
		"INSERT INTO t1 VALUES (1)",
		"INSERT INTO t2 VALUES (2)",
		"INSERT INTO t3 VALUES (3)",
	}, db.calls)
}

func TestRunner_Run_StopsOnError(t *testing.T) {
	seedErr := errors.New("insert failed")
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}))

	callCount := 0
	runner.Register(
		Seed{Name: "ok", Run: func(ctx context.Context, db DB) error {
			callCount++
			return nil
		}},
		Seed{Name: "fail", Run: func(_ context.Context, _ DB) error {
			callCount++
			return seedErr
		}},
		Seed{Name: "never", Run: func(_ context.Context, _ DB) error {
			callCount++
			return nil
		}},
	)

	result, err := runner.Run(context.Background())

	require.Error(t, err)
	assert.ErrorIs(t, err, seedErr)
	assert.Contains(t, err.Error(), `seed "fail" failed`)
	assert.Equal(t, 1, result.Success)
	assert.Equal(t, 1, result.Failed)
	assert.Equal(t, 2, callCount, "third seed should not run")
}

func TestRunner_Run_ContinueOnError(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}), WithContinueOnError(true))

	runner.Register(
		Seed{Name: "first", Run: func(_ context.Context, _ DB) error {
			return nil
		}},
		Seed{Name: "fail", Run: func(_ context.Context, _ DB) error {
			return errors.New("boom")
		}},
		Seed{Name: "third", Run: func(_ context.Context, _ DB) error {
			return nil
		}},
	)

	result, err := runner.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 2, result.Success)
	assert.Equal(t, 1, result.Failed)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "fail", result.Errors[0].Name)
}

func TestRunner_Run_DryRun(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}), WithDryRun(true))

	executed := false
	runner.Register(
		Seed{Name: "should-skip", Run: func(_ context.Context, _ DB) error {
			executed = true
			return nil
		}},
	)

	result, err := runner.Run(context.Background())

	require.NoError(t, err)
	assert.False(t, executed, "seed should not execute in dry run")
	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 0, result.Success)
}

func TestRunner_Run_EmptySeeds(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}))

	result, err := runner.Run(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
}

func TestRunner_Register_Appends(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}))

	runner.Register(Seed{Name: "a", Run: func(_ context.Context, _ DB) error { return nil }})
	runner.Register(
		Seed{Name: "b", Run: func(_ context.Context, _ DB) error { return nil }},
		Seed{Name: "c", Run: func(_ context.Context, _ DB) error { return nil }},
	)

	result, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.Success)
}

func TestNewRunner_DefaultLogger(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db)
	assert.NotNil(t, runner.log, "default logger should be set")
}

func TestWithLogger(t *testing.T) {
	db := &mockDB{}
	custom := &silentLogger{}
	runner := NewRunner(db, WithLogger(custom))
	assert.Equal(t, custom, runner.log)
}

func TestWithLogger_NilIgnored(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(nil))
	assert.NotNil(t, runner.log, "nil logger should be ignored, default kept")
}

func TestRunner_Run_RespectsContextCancellation(t *testing.T) {
	db := &mockDB{}
	runner := NewRunner(db, WithLogger(&silentLogger{}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	runner.Register(Seed{Name: "should-not-run", Run: func(_ context.Context, _ DB) error {
		t.Fatal("seed should not execute with cancelled context")
		return nil
	}})

	result, err := runner.Run(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, result.Success)
}

func TestSeedError_ImplementsError(t *testing.T) {
	inner := errors.New("connection refused")
	se := SeedError{Name: "users", Err: inner}

	assert.Contains(t, se.Error(), "users")
	assert.Contains(t, se.Error(), "connection refused")
	assert.ErrorIs(t, se, inner, "Unwrap should expose inner error")
}
