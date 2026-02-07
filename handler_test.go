package copilotcli

import (
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewQueryHandler(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	handler := NewQueryHandler(client)

	t.Run("rejects empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte("{}")))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp errorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "prompt is required", resp.Error)
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte("not json")))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("returns 503 when not connected", func(t *testing.T) {
		body := `{"prompt": "What is the stock level?"}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("rejects whitespace-only prompt", func(t *testing.T) {
		body := `{"prompt": "   "}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp errorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "prompt is required", resp.Error)
	})
}

func TestNewStreamHandler(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	handler := NewStreamHandler(client)

	t.Run("rejects empty prompt", func(t *testing.T) {
		body := `{"prompt": ""}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte("{bad")))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp errorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid request body", resp.Error)
	})

	t.Run("returns 503 when not connected", func(t *testing.T) {
		body := `{"prompt": "tell me something"}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("rejects whitespace-only prompt", func(t *testing.T) {
		body := `{"prompt": "\t\n  "}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("streaming not supported by ResponseWriter", func(t *testing.T) {
		body := `{"prompt": "hello"}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
		// Use a writer that does NOT implement http.Flusher.
		rec := &nonFlushableWriter{header: make(http.Header)}

		handler(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.statusCode)
	})
}

// nonFlushableWriter is an http.ResponseWriter that does NOT implement http.Flusher.
type nonFlushableWriter struct {
	header     http.Header
	statusCode int
	body       bytes.Buffer
}

func (w *nonFlushableWriter) Header() http.Header         { return w.header }
func (w *nonFlushableWriter) WriteHeader(statusCode int)  { w.statusCode = statusCode }
func (w *nonFlushableWriter) Write(b []byte) (int, error) { return w.body.Write(b) }

func TestNewHealthHandler(t *testing.T) {
	client, err := New()
	require.NoError(t, err)
	handler := NewHealthHandler(client)

	t.Run("returns unhealthy when not connected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/copilot/health", nil)
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

		var resp map[string]string
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "unhealthy", resp["status"])
		assert.NotEmpty(t, resp["error"])
	})
}

func TestWriteJSON(t *testing.T) {
	t.Run("writes valid JSON with correct content type", func(t *testing.T) {
		rec := httptest.NewRecorder()

		writeJSON(rec, http.StatusOK, map[string]string{"key": "value"})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

		var resp map[string]string
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "value", resp["key"])
	})

	t.Run("returns 500 when marshal fails", func(t *testing.T) {
		rec := httptest.NewRecorder()

		// math.NaN() cannot be marshaled to JSON
		writeJSON(rec, http.StatusOK, math.NaN())

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestNewQueryHandler_InternalError(t *testing.T) {
	// Simulate a connected client that fails at session setup (500, not 503).
	client, err := New()
	require.NoError(t, err)
	client.connected = true
	handler := NewQueryHandler(client)

	body := `{"prompt": "hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	// Should get 500 because the error is "session setup: ..." not ErrNotConnected.
	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var resp errorResponse
	err = json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Contains(t, resp.Error, "session setup")
}

func TestNewStreamHandler_InternalError(t *testing.T) {
	// Simulate a connected client that fails at session setup (500, not 503).
	client, err := New()
	require.NoError(t, err)
	client.connected = true
	handler := NewStreamHandler(client)

	body := `{"prompt": "hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()

	writeError(rec, http.StatusBadRequest, "something went wrong")

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp errorResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "something went wrong", resp.Error)
}

func TestWriteSSE(t *testing.T) {
	t.Run("writes SSE event with data prefix", func(t *testing.T) {
		rec := httptest.NewRecorder()
		flusher := rec // httptest.ResponseRecorder implements http.Flusher

		writeSSE(rec, flusher, map[string]string{"delta": "hello"})

		body := rec.Body.String()
		assert.Contains(t, body, "data: ")
		assert.Contains(t, body, `"delta":"hello"`)
		assert.True(t, len(body) > 0)
	})

	t.Run("handles unmarshalable data gracefully", func(t *testing.T) {
		rec := httptest.NewRecorder()
		flusher := rec

		// Should not panic; json.Marshal will fail silently.
		writeSSE(rec, flusher, math.NaN())

		// Nothing should be written since marshal failed.
		assert.Empty(t, rec.Body.String())
	})
}
