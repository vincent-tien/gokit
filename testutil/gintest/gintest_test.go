package gintest

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestNewRequest_NilBody(t *testing.T) {
	req := NewRequest(http.MethodGet, "/test", nil)

	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "/test", req.URL.Path)
	assert.Empty(t, req.Header.Get("Content-Type"))
	assert.Nil(t, req.Body)
}

func TestNewRequest_StringBody(t *testing.T) {
	req := NewRequest(http.MethodPost, "/test", `{"name":"Alice"}`)

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"name":"Alice"}`, string(body))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestNewRequest_ByteSliceBody(t *testing.T) {
	data := []byte(`{"id":1}`)
	req := NewRequest(http.MethodPost, "/test", data)

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"id":1}`, string(body))
}

func TestNewRequest_IOReader(t *testing.T) {
	reader := bytes.NewBufferString(`{"key":"value"}`)
	req := NewRequest(http.MethodPut, "/test", reader)

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"key":"value"}`, string(body))
}

func TestNewRequest_StructBody(t *testing.T) {
	payload := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{Name: "Bob", Age: 25}

	req := NewRequest(http.MethodPost, "/test", payload)

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.JSONEq(t, `{"name":"Bob","age":25}`, string(body))
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestRecord_ReturnsResponse(t *testing.T) {
	engine := gin.New()
	engine.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	rec := Record(engine, http.MethodGet, "/ping", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"message":"pong"}`, rec.Body.String())
}

func TestRecord_PostWithBody(t *testing.T) {
	engine := gin.New()
	engine.POST("/echo", func(c *gin.Context) {
		var req map[string]any
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, req)
	})

	body := map[string]string{"hello": "world"}
	rec := Record(engine, http.MethodPost, "/echo", body)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"hello":"world"}`, rec.Body.String())
}

func TestRecord_NotFound(t *testing.T) {
	engine := gin.New()
	rec := Record(engine, http.MethodGet, "/nonexistent", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
