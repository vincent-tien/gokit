package otel

import (
	"context"
	"testing"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTracer_Dev_StdoutExporter(t *testing.T) {
	tp, err := NewTracer(TracerConfig{
		ServiceName: "test-service",
		Env:         "dev",
		SampleRate:  1.0,
	})
	require.NoError(t, err)
	require.NotNil(t, tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
}

func TestNewTracer_DefaultSampleRate(t *testing.T) {
	tp, err := NewTracer(TracerConfig{
		ServiceName: "test-service",
		Env:         "dev",
	})
	require.NoError(t, err)
	require.NotNil(t, tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
}

func TestNewMetrics_ReturnsHandlerAndProvider(t *testing.T) {
	registry := promclient.NewRegistry()
	handler, mp, err := NewMetrics(MetricsConfig{Registry: registry})
	require.NoError(t, err)
	require.NotNil(t, handler)
	require.NotNil(t, mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
}

func TestNewREDMetrics_CreatesInstruments(t *testing.T) {
	registry := promclient.NewRegistry()
	_, mp, err := NewMetrics(MetricsConfig{Registry: registry})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("test")
	red, err := NewREDMetrics(meter, "http")
	require.NoError(t, err)
	assert.NotNil(t, red.RequestCount)
	assert.NotNil(t, red.RequestDuration)
	assert.NotNil(t, red.ErrorCount)

	// Verify instruments work without panic.
	red.RequestCount.Add(context.Background(), 1)
	red.RequestDuration.Record(context.Background(), 0.5)
	red.ErrorCount.Add(context.Background(), 1)
}
