package otel

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsConfig configures the Prometheus metrics exporter.
type MetricsConfig struct {
	Registry *promclient.Registry // nil → uses default registry
}

// NewMetrics creates a Prometheus metrics exporter and returns an http.Handler
// for the /metrics endpoint. Also returns the MeterProvider for creating instruments.
func NewMetrics(cfg MetricsConfig) (http.Handler, *sdkmetric.MeterProvider, error) {
	registry := cfg.Registry
	if registry == nil {
		registry = promclient.DefaultRegisterer.(*promclient.Registry)
	}

	exporter, err := prometheus.New(prometheus.WithRegisterer(registry))
	if err != nil {
		return nil, nil, fmt.Errorf("otel: create prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true})

	return handler, mp, nil
}

// REDMetrics holds Rate-Error-Duration instruments for a service or handler group.
type REDMetrics struct {
	RequestCount    otelmetric.Int64Counter
	RequestDuration otelmetric.Float64Histogram
	ErrorCount      otelmetric.Int64Counter
}

// NewREDMetrics creates standard RED (Rate, Error, Duration) instruments.
// Prefix is prepended to metric names (e.g. "http" → "http.request.count").
func NewREDMetrics(meter otelmetric.Meter, prefix string) (*REDMetrics, error) {
	rc, err := meter.Int64Counter(prefix+".request.count",
		otelmetric.WithDescription("Total request count"))
	if err != nil {
		return nil, fmt.Errorf("otel: create request counter: %w", err)
	}

	rd, err := meter.Float64Histogram(prefix+".request.duration",
		otelmetric.WithDescription("Request duration in seconds"),
		otelmetric.WithUnit("s"))
	if err != nil {
		return nil, fmt.Errorf("otel: create request duration: %w", err)
	}

	ec, err := meter.Int64Counter(prefix+".error.count",
		otelmetric.WithDescription("Total error count"))
	if err != nil {
		return nil, fmt.Errorf("otel: create error counter: %w", err)
	}

	return &REDMetrics{
		RequestCount:    rc,
		RequestDuration: rd,
		ErrorCount:      ec,
	}, nil
}
