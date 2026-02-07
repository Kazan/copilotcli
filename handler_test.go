package copilotcli

import (
	"bytes"
	"encoding/json"
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

	t.Run("returns 503 when not connected", func(t *testing.T) {
		body := `{"prompt": "tell me something"}`
		req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
		rec := httptest.NewRecorder()

		handler(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})
}

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
	})
}
