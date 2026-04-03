// Package ginmw provides Gin middleware for OpenTelemetry tracing and RED metrics.
package ginmw

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	gokit "github.com/vincent-tien/gokit/otel"
)

// Trace returns Gin middleware that creates a span for each request.
func Trace(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)

	return func(c *gin.Context) {
		spanName := fmt.Sprintf("%s %s", c.Request.Method, c.FullPath())
		ctx, span := tracer.Start(c.Request.Context(), spanName,
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(c.Request.Method),
				semconv.URLPath(c.Request.URL.Path),
			),
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		status := c.Writer.Status()
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
		}
	}
}

// Metrics returns Gin middleware that records RED (Rate, Error, Duration) metrics.
func Metrics(red *gokit.REDMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := c.Writer.Status()
		attrs := attribute.NewSet(
			semconv.HTTPRequestMethodKey.String(c.Request.Method),
			semconv.HTTPRouteKey.String(c.FullPath()),
			attribute.Int("http.response.status_code", status),
		)
		opt := otelmetric.WithAttributeSet(attrs)

		red.RequestCount.Add(c.Request.Context(), 1, opt)
		red.RequestDuration.Record(c.Request.Context(), duration, opt)
		if status >= 400 {
			red.ErrorCount.Add(c.Request.Context(), 1, opt)
		}
	}
}
