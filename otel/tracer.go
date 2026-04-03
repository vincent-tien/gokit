// Package otel provides OpenTelemetry and Prometheus observability setup.
package otel

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// TracerConfig configures the OpenTelemetry TracerProvider.
type TracerConfig struct {
	ServiceName string  // required: service name for traces
	Endpoint    string  // OTLP collector endpoint (e.g. "localhost:4317")
	Env         string  // "dev" → stdout exporter, else → OTLP gRPC
	SampleRate  float64 // 0.0-1.0, default 1.0
}

// NewTracer creates an OpenTelemetry TracerProvider and registers it globally.
// Caller must defer tp.Shutdown(ctx) for clean flush.
func NewTracer(cfg TracerConfig) (*sdktrace.TracerProvider, error) {
	ctx := context.Background()

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.DeploymentEnvironmentKey.String(cfg.Env),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}

	var exporter sdktrace.SpanExporter
	if cfg.Env == "dev" {
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	} else {
		opts := []otlptracegrpc.Option{otlptracegrpc.WithInsecure()}
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("otel: create exporter: %w", err)
	}

	sampleRate := cfg.SampleRate
	if sampleRate <= 0 {
		sampleRate = 1.0
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRate))),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp, nil
}
