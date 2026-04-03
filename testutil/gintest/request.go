// Package gintest provides Gin-specific test helpers for HTTP handler testing.
package gintest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
)

// NewRequest creates an *http.Request suitable for gin handler testing.
// body can be: nil, string, []byte, io.Reader, or any struct (marshaled to JSON).
func NewRequest(method, path string, body any) *http.Request {
	var reader io.Reader
	switch v := body.(type) {
	case nil:
		reader = nil
	case string:
		reader = bytes.NewBufferString(v)
	case []byte:
		reader = bytes.NewBuffer(v)
	case io.Reader:
		reader = v
	default:
		b, _ := json.Marshal(v)
		reader = bytes.NewBuffer(b)
	}
	req, _ := http.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// Record sends a request through a gin.Engine and returns the response recorder.
// Shortcut for: create recorder → engine.ServeHTTP → return recorder.
func Record(engine *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	req := NewRequest(method, path, body)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}
