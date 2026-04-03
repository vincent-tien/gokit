// Package seed provides a framework for running idempotent database seed operations.
//
// seed.go imports ONLY stdlib — no database driver imports.
// Database adapters are in adapters.go.
package seed

import (
	"context"
	"fmt"
	"time"
)

// DB is the minimal database interface for seed operations.
// Satisfied by: *sql.DB (via StdDB), *sql.Tx (via StdTx),
// *pgxpool.Pool (via PgxPool), pgx.Tx (via PgxTx).
type DB interface {
	Exec(ctx context.Context, sql string, args ...any) error
}

// Seed is a named, idempotent data operation.
type Seed struct {
	Name string
	Run  func(ctx context.Context, db DB) error
}

// Logger is a minimal logging interface for seed operations.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

// Runner executes seeds sequentially.
type Runner struct {
	db      DB
	seeds   []Seed
	log     Logger
	dryRun  bool
	contErr bool
}

// NewRunner creates a Runner with the given DB and options.
func NewRunner(db DB, opts ...Option) *Runner {
	r := &Runner{db: db, log: &defaultLogger{}}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Register adds seeds to the runner.
func (r *Runner) Register(seeds ...Seed) {
	r.seeds = append(r.seeds, seeds...)
}

// Run executes all registered seeds sequentially.
// Returns a Result summarizing the run. Returns an error if a seed fails
// and ContinueOnError is not set.
func (r *Runner) Run(ctx context.Context) (*Result, error) {
	result := &Result{Total: len(r.seeds)}
	start := time.Now()

	for _, s := range r.seeds {
		if err := ctx.Err(); err != nil {
			result.Duration = time.Since(start)
			return result, fmt.Errorf("seed run cancelled: %w", err)
		}

		r.log.Info("running seed", "name", s.Name)

		if r.dryRun {
			r.log.Info("dry run — skipped", "name", s.Name)
			result.Skipped++
			continue
		}

		if err := s.Run(ctx, r.db); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, SeedError{Name: s.Name, Err: err})
			r.log.Error("seed failed", "name", s.Name, "error", err)

			if !r.contErr {
				result.Duration = time.Since(start)
				return result, fmt.Errorf("seed %q failed: %w", s.Name, err)
			}
			continue
		}

		result.Success++
		r.log.Info("seed completed", "name", s.Name)
	}

	result.Duration = time.Since(start)
	return result, nil
}

// Option configures a Runner.
type Option func(*Runner)

// WithLogger sets a custom logger for the runner. Nil is ignored.
func WithLogger(log Logger) Option {
	return func(r *Runner) {
		if log != nil {
			r.log = log
		}
	}
}

// WithDryRun enables dry-run mode — seeds are logged but not executed.
func WithDryRun(dry bool) Option { return func(r *Runner) { r.dryRun = dry } }

// WithContinueOnError allows the runner to continue executing remaining seeds
// after a failure instead of stopping immediately.
func WithContinueOnError(cont bool) Option { return func(r *Runner) { r.contErr = cont } }

// Result summarizes a seed run.
type Result struct {
	Total    int
	Success  int
	Failed   int
	Skipped  int
	Duration time.Duration
	Errors   []SeedError
}

// SeedError records a failed seed. Implements error and supports errors.Unwrap.
type SeedError struct {
	Name string
	Err  error
}

func (e SeedError) Error() string { return fmt.Sprintf("seed %q: %v", e.Name, e.Err) }
func (e SeedError) Unwrap() error { return e.Err }

// defaultLogger writes to stdout using fmt.
type defaultLogger struct{}

func (l *defaultLogger) Info(msg string, args ...any) {
	if len(args) > 0 {
		fmt.Printf("[seed] %s %v\n", msg, args)
	} else {
		fmt.Printf("[seed] %s\n", msg)
	}
}

func (l *defaultLogger) Error(msg string, args ...any) {
	if len(args) > 0 {
		fmt.Printf("[seed] ERROR: %s %v\n", msg, args)
	} else {
		fmt.Printf("[seed] ERROR: %s\n", msg)
	}
}
