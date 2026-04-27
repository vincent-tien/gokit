package broker

import "time"

// SubscribeConfig holds configuration for a subscription.
type SubscribeConfig struct {
	Group       string
	Concurrency int
	MaxRetries  int
	AckTimeout  time.Duration
}

// defaultConfig returns the default SubscribeConfig.
func defaultConfig() SubscribeConfig {
	return SubscribeConfig{
		Concurrency: 1,
		MaxRetries:  3,
		AckTimeout:  30 * time.Second,
	}
}

// SubscribeOption is a functional option for SubscribeConfig.
type SubscribeOption func(*SubscribeConfig)

// WithGroup sets the consumer group name for the subscription.
func WithGroup(group string) SubscribeOption {
	return func(c *SubscribeConfig) { c.Group = group }
}

// WithConcurrency sets the number of concurrent message processors.
func WithConcurrency(n int) SubscribeOption {
	return func(c *SubscribeConfig) { c.Concurrency = n }
}

// WithMaxRetries sets the maximum number of delivery retries before dead-lettering.
func WithMaxRetries(n int) SubscribeOption {
	return func(c *SubscribeConfig) { c.MaxRetries = n }
}

// WithAckTimeout sets the deadline for handler acknowledgment.
func WithAckTimeout(d time.Duration) SubscribeOption {
	return func(c *SubscribeConfig) { c.AckTimeout = d }
}

// applyOpts applies all options to a default config and returns the result.
func applyOpts(opts []SubscribeOption) SubscribeConfig {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}
