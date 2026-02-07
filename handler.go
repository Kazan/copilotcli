package copilotcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// queryRequest is the JSON body for the query endpoint.
type queryRequest struct {
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id,omitempty"`
}

// queryResponse is the JSON response for a non-streaming query.
type queryResponse struct {
	Content   string `json:"content"`
	SessionID string `json:"session_id"`
}

// errorResponse is the standard error JSON response.
type errorResponse struct {
	Error string `json:"error"`
}

// NewQueryHandler returns an http.HandlerFunc that accepts POST requests with a JSON body
// containing a "prompt" field, queries the Copilot LLM, and returns the response as JSON.
//
// This handler supports multi-turn conversations via an optional "session_id" field.
// If no session_id is provided, a new session is created for each request.
//
// Example registration:
//
//	mux.HandleFunc("POST /api/copilot/query", copilotcli.NewQueryHandler(client))
func NewQueryHandler(client *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req queryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if strings.TrimSpace(req.Prompt) == "" {
			writeError(w, http.StatusBadRequest, "prompt is required")
			return
		}

		result, err := client.QueryWithSession(r.Context(), req.SessionID, req.Prompt)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrNotConnected) || errors.Is(err, ErrSidecarUnavailable) {
				status = http.StatusServiceUnavailable
			}
			writeError(w, status, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, queryResponse{
			Content:   result.Content,
			SessionID: result.SessionID,
		})
	}
}

// NewStreamHandler returns an http.HandlerFunc that streams the LLM response
// via Server-Sent Events (SSE).
//
// The client must accept "text/event-stream". Each event has the format:
//
//	data: {"delta":"...", "session_id":"..."}
//
// The final event includes "final":true with the complete content.
//
// Example registration:
//
//	mux.HandleFunc("POST /api/copilot/stream", copilotcli.NewStreamHandler(client))
func NewStreamHandler(client *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, http.StatusInternalServerError, "streaming not supported")
			return
		}

		var req queryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if strings.TrimSpace(req.Prompt) == "" {
			writeError(w, http.StatusBadRequest, "prompt is required")
			return
		}

		events, sessionID, err := client.QueryStream(r.Context(), req.SessionID, req.Prompt)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, ErrNotConnected) || errors.Is(err, ErrSidecarUnavailable) {
				status = http.StatusServiceUnavailable
			}
			writeError(w, status, err.Error())
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		for event := range events {
			if event.Error != nil {
				writeSSE(w, flusher, map[string]any{
					"error":      event.Error.Error(),
					"session_id": sessionID,
				})
				return
			}

			if event.IsFinal {
				writeSSE(w, flusher, map[string]any{
					"content":    event.Content,
					"session_id": sessionID,
					"final":      true,
				})
				return
			}

			writeSSE(w, flusher, map[string]any{
				"delta":      event.DeltaContent,
				"session_id": sessionID,
			})
		}
	}
}

// NewHealthHandler returns an http.HandlerFunc that reports the sidecar health.
// Returns 200 if connected and responsive, 503 otherwise.
//
// Example registration:
//
//	mux.HandleFunc("GET /api/copilot/health", copilotcli.NewHealthHandler(client))
func NewHealthHandler(client *Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := client.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unhealthy",
				"error":  err.Error(),
			})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "healthy",
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
	flusher.Flush()
}
