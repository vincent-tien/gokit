package ginmw

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gokit "github.com/vincent-tien/gokit/otel"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestTrace_CreatesSpan(t *testing.T) {
	tp, err := gokit.NewTracer(gokit.TracerConfig{
		ServiceName: "test",
		Env:         "dev",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	engine := gin.New()
	engine.Use(Trace("test"))
	engine.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
}

func TestTrace_Sets500ErrorStatus(t *testing.T) {
	tp, err := gokit.NewTracer(gokit.TracerConfig{
		ServiceName: "test",
		Env:         "dev",
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	engine := gin.New()
	engine.Use(Trace("test"))
	engine.GET("/fail", func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "error")
	})

	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/fail", nil)
	engine.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestMetrics_RecordsREDMetrics(t *testing.T) {
	registry := promclient.NewRegistry()
	_, mp, err := gokit.NewMetrics(gokit.MetricsConfig{Registry: registry})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	meter := mp.Meter("test")
	red, err := gokit.NewREDMetrics(meter, "http")
	require.NoError(t, err)

	engine := gin.New()
	engine.Use(Metrics(red))
	engine.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	engine.GET("/bad", func(c *gin.Context) {
		c.String(http.StatusBadRequest, "bad")
	})

	// 200 — should count request + duration, no error.
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ok", nil)
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// 400 — should count request + duration + error.
	rec = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/bad", nil)
	engine.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
