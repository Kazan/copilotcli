package copilotcli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// QueryWithSession — event handling paths
// ---------------------------------------------------------------------------

func TestQueryWithSession_SuccessfulQuery(t *testing.T) {
	sess := &mockSDKSession{id: "sess-abc"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, opts copilot.MessageOptions) (string, error) {
		// Simulate async events after Send returns.
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("Hello, world!")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	result, err := client.QueryWithSession(t.Context(), "", "hi")

	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", result.Content)
	assert.Equal(t, "sess-abc", result.SessionID)
}

func TestQueryWithSession_SessionError(t *testing.T) {
	sess := &mockSDKSession{id: "sess-err"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.SessionError,
				Data: copilot.Data{Message: ptr("model overloaded")},
			})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	_, err := client.QueryWithSession(t.Context(), "", "hi")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "model overloaded")
}

func TestQueryWithSession_SessionErrorNilMessage(t *testing.T) {
	sess := &mockSDKSession{id: "sess-e2"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.SessionError,
				Data: copilot.Data{}, // Message is nil — should use default "session error"
			})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	_, err := client.QueryWithSession(t.Context(), "", "hi")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "session error")
}

func TestQueryWithSession_ContextCancellation(t *testing.T) {
	sess := &mockSDKSession{id: "sess-cancel"}
	abortCalled := false
	sess.abortFn = func(_ context.Context) error {
		abortCalled = true
		return nil
	}

	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		// Don't emit any events — context will be canceled.
		return "msg-1", nil
	}

	client := newTestClient(mock)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := client.QueryWithSession(ctx, "", "hi")

	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.True(t, abortCalled, "Abort should be called on context cancellation")
}

func TestQueryWithSession_SendError(t *testing.T) {
	sess := &mockSDKSession{id: "sess-senderr"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		return "", fmt.Errorf("connection reset")
	}

	client := newTestClient(mock)
	_, err := client.QueryWithSession(t.Context(), "", "hi")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sending message")
	assert.Contains(t, err.Error(), "connection reset")
}

func TestQueryWithSession_AssistantMessageNilContent(t *testing.T) {
	sess := &mockSDKSession{id: "sess-nil"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			// Content is nil — should not crash.
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	result, err := client.QueryWithSession(t.Context(), "", "hi")

	require.NoError(t, err)
	assert.Empty(t, result.Content)
}

func TestQueryWithSession_ResumeSession(t *testing.T) {
	sess := &mockSDKSession{id: "existing-sess"}
	mock := &mockSDKClient{
		resumeFn: func(_ context.Context, sessionID string, cfg *copilot.ResumeSessionConfig) (sdkSession, error) {
			assert.Equal(t, "existing-sess", sessionID)
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("resumed response")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock, WithSystemMessage("You are helpful."), WithBYOK(ProviderOpenAI, "https://api.openai.com/v1", "sk-key"))
	result, err := client.QueryWithSession(t.Context(), "existing-sess", "hello again")

	require.NoError(t, err)
	assert.Equal(t, "resumed response", result.Content)
	assert.Equal(t, "existing-sess", result.SessionID)
}

// ---------------------------------------------------------------------------
// QueryStream — event handling paths
// ---------------------------------------------------------------------------

func TestQueryStream_SuccessfulStream(t *testing.T) {
	sess := &mockSDKSession{id: "stream-sess"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessageDelta,
				Data: copilot.Data{DeltaContent: ptr("Hello")},
			})
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessageDelta,
				Data: copilot.Data{DeltaContent: ptr(", world!")},
			})
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("Hello, world!")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	events, sid, err := client.QueryStream(t.Context(), "", "hi")

	require.NoError(t, err)
	assert.Equal(t, "stream-sess", sid)

	var deltas []string
	var finalEvent StreamEvent
	for evt := range events {
		if evt.IsFinal {
			finalEvent = evt
		} else {
			deltas = append(deltas, evt.DeltaContent)
		}
	}

	assert.Equal(t, []string{"Hello", ", world!"}, deltas)
	assert.True(t, finalEvent.IsFinal)
	assert.Equal(t, "Hello, world!", finalEvent.Content)
}

func TestQueryStream_ErrorEvent(t *testing.T) {
	sess := &mockSDKSession{id: "stream-err"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessageDelta,
				Data: copilot.Data{DeltaContent: ptr("partial")},
			})
			sess.emit(copilot.SessionEvent{
				Type: copilot.SessionError,
				Data: copilot.Data{Message: ptr("rate limited")},
			})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	events, sid, err := client.QueryStream(t.Context(), "", "hi")

	require.NoError(t, err)
	assert.Equal(t, "stream-err", sid)

	var collected []StreamEvent
	for evt := range events {
		collected = append(collected, evt)
	}

	require.Len(t, collected, 2, "should have delta + error")
	assert.Equal(t, "partial", collected[0].DeltaContent)
	assert.Error(t, collected[1].Error)
	assert.Contains(t, collected[1].Error.Error(), "rate limited")
}

func TestQueryStream_ErrorEventNilMessage(t *testing.T) {
	sess := &mockSDKSession{id: "stream-e2"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.SessionError,
				Data: copilot.Data{},
			})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	events, _, err := client.QueryStream(t.Context(), "", "hi")
	require.NoError(t, err)

	var collected []StreamEvent
	for evt := range events {
		collected = append(collected, evt)
	}

	require.Len(t, collected, 1)
	assert.Contains(t, collected[0].Error.Error(), "session error")
}

func TestQueryStream_SendError(t *testing.T) {
	sess := &mockSDKSession{id: "stream-senderr"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		return "", fmt.Errorf("broken pipe")
	}

	client := newTestClient(mock)
	ch, sid, err := client.QueryStream(t.Context(), "", "hi")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "sending message")
	assert.Nil(t, ch)
	assert.Empty(t, sid)
}

func TestQueryStream_ResumeSession(t *testing.T) {
	sess := &mockSDKSession{id: "resume-stream"}
	mock := &mockSDKClient{
		resumeFn: func(_ context.Context, sessionID string, _ *copilot.ResumeSessionConfig) (sdkSession, error) {
			assert.Equal(t, "resume-stream", sessionID)
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("done")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock, WithSystemMessage("sys"))
	events, sid, err := client.QueryStream(t.Context(), "resume-stream", "hello")

	require.NoError(t, err)
	assert.Equal(t, "resume-stream", sid)

	var final StreamEvent
	for evt := range events {
		if evt.IsFinal {
			final = evt
		}
	}
	assert.Equal(t, "done", final.Content)
}

func TestQueryStream_DeltaWithNilContent(t *testing.T) {
	sess := &mockSDKSession{id: "stream-nil-delta"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			// Delta with nil DeltaContent — should be skipped.
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessageDelta,
				Data: copilot.Data{},
			})
			// AssistantMessage with nil Content.
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	events, _, err := client.QueryStream(t.Context(), "", "hi")
	require.NoError(t, err)

	var collected []StreamEvent
	for evt := range events {
		collected = append(collected, evt)
	}

	// Only the final event should be received (no delta with nil content).
	require.Len(t, collected, 1)
	assert.True(t, collected[0].IsFinal)
	assert.Empty(t, collected[0].Content)
}

// ---------------------------------------------------------------------------
// Start — success path
// ---------------------------------------------------------------------------

func TestClient_Start_Success(t *testing.T) {
	mock := &mockSDKClient{
		startFn: func(_ context.Context) error { return nil },
	}

	client := &Client{
		cfg:       defaultCfg(),
		sdk:       mock,
		connected: false,
	}

	err := client.Start(t.Context())
	require.NoError(t, err)
	assert.True(t, client.IsConnected())
}

func TestClient_Start_SuccesAfterRetries(t *testing.T) {
	attempts := 0
	mock := &mockSDKClient{
		startFn: func(_ context.Context) error {
			attempts++
			if attempts < 3 {
				return fmt.Errorf("not ready yet")
			}
			return nil
		},
	}

	c := defaultCfg()
	c.retryAttempts = 5
	c.connTimeout = 50 * time.Millisecond
	c.retryDelay = 10 * time.Millisecond

	client := &Client{
		cfg:       c,
		sdk:       mock,
		connected: false,
	}

	err := client.Start(t.Context())
	require.NoError(t, err)
	assert.True(t, client.IsConnected())
	assert.Equal(t, 3, attempts)
}

func TestClient_Stop_WithMock(t *testing.T) {
	stopCalled := false
	mock := &mockSDKClient{
		stopFn: func() error {
			stopCalled = true
			return nil
		},
	}

	client := newTestClient(mock)
	assert.True(t, client.IsConnected())

	err := client.Stop()
	require.NoError(t, err)
	assert.False(t, client.IsConnected())
	assert.True(t, stopCalled)
}

func TestClient_Stop_WithError(t *testing.T) {
	mock := &mockSDKClient{
		stopFn: func() error {
			return fmt.Errorf("stop failed")
		},
	}

	client := newTestClient(mock)
	err := client.Stop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stop failed")
	assert.False(t, client.IsConnected())
}

// ---------------------------------------------------------------------------
// Ping — connected path
// ---------------------------------------------------------------------------

func TestClient_Ping_Success(t *testing.T) {
	mock := &mockSDKClient{
		pingFn: func(_ context.Context, _ string) (*copilot.PingResponse, error) {
			return &copilot.PingResponse{Message: "pong"}, nil
		},
	}

	client := newTestClient(mock)
	err := client.Ping(t.Context())
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// DestroySession — connected path
// ---------------------------------------------------------------------------

func TestClient_DestroySession_Success(t *testing.T) {
	deleted := ""
	mock := &mockSDKClient{
		deleteFn: func(_ context.Context, sessionID string) error {
			deleted = sessionID
			return nil
		},
	}

	client := newTestClient(mock)
	err := client.DestroySession(t.Context(), "sess-to-delete")

	assert.NoError(t, err)
	assert.Equal(t, "sess-to-delete", deleted)
}

// ---------------------------------------------------------------------------
// NewHealthHandler — healthy path
// ---------------------------------------------------------------------------

func TestNewHealthHandler_Healthy(t *testing.T) {
	mock := &mockSDKClient{
		pingFn: func(_ context.Context, _ string) (*copilot.PingResponse, error) {
			return &copilot.PingResponse{}, nil
		},
	}

	client := newTestClient(mock)
	handler := NewHealthHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/api/copilot/health", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]string
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "healthy", resp["status"])
}

// ---------------------------------------------------------------------------
// NewQueryHandler — with mock (success path)
// ---------------------------------------------------------------------------

func TestNewQueryHandler_Success(t *testing.T) {
	sess := &mockSDKSession{id: "handler-sess"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("the answer is 42")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	handler := NewQueryHandler(client)

	body := `{"prompt": "what is the meaning of life?"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp queryResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "the answer is 42", resp.Content)
	assert.Equal(t, "handler-sess", resp.SessionID)
}

func TestNewQueryHandler_WithSessionID(t *testing.T) {
	sess := &mockSDKSession{id: "existing-handler-sess"}
	mock := &mockSDKClient{
		resumeFn: func(_ context.Context, sessionID string, _ *copilot.ResumeSessionConfig) (sdkSession, error) {
			assert.Equal(t, "existing-handler-sess", sessionID)
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("follow-up answer")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	handler := NewQueryHandler(client)

	body := `{"prompt": "tell me more", "session_id": "existing-handler-sess"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp queryResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "follow-up answer", resp.Content)
}

// ---------------------------------------------------------------------------
// NewStreamHandler — with mock (SSE streaming)
// ---------------------------------------------------------------------------

func TestNewStreamHandler_SuccessfulStream(t *testing.T) {
	sess := &mockSDKSession{id: "sse-sess"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessageDelta,
				Data: copilot.Data{DeltaContent: ptr("chunk1")},
			})
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessageDelta,
				Data: copilot.Data{DeltaContent: ptr("chunk2")},
			})
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("chunk1chunk2")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	handler := NewStreamHandler(client)

	body := `{"prompt": "stream me"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))

	sseBody := rec.Body.String()
	assert.Contains(t, sseBody, `"delta":"chunk1"`)
	assert.Contains(t, sseBody, `"delta":"chunk2"`)
	assert.Contains(t, sseBody, `"final":true`)
	assert.Contains(t, sseBody, `"content":"chunk1chunk2"`)
}

func TestNewStreamHandler_ErrorEvent(t *testing.T) {
	sess := &mockSDKSession{id: "sse-err-sess"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.SessionError,
				Data: copilot.Data{Message: ptr("something broke")},
			})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	handler := NewStreamHandler(client)

	body := `{"prompt": "fail me"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code) // SSE headers already sent.
	sseBody := rec.Body.String()
	assert.Contains(t, sseBody, `"error"`)
	assert.Contains(t, sseBody, "something broke")
}

func TestNewStreamHandler_WithSessionID(t *testing.T) {
	sess := &mockSDKSession{id: "sse-resume"}
	mock := &mockSDKClient{
		resumeFn: func(_ context.Context, _ string, _ *copilot.ResumeSessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("resumed stream")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	handler := NewStreamHandler(client)

	body := `{"prompt": "continue", "session_id": "sse-resume"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	handler(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	sseBody := rec.Body.String()
	assert.Contains(t, sseBody, `"final":true`)
	assert.Contains(t, sseBody, `"content":"resumed stream"`)
}

// ---------------------------------------------------------------------------
// getOrCreateSession — various configs
// ---------------------------------------------------------------------------

func TestGetOrCreateSession_CreateWithTools(t *testing.T) {
	expectedSess := &mockSDKSession{id: "tools-sess"}
	var capturedConfig *copilot.SessionConfig

	mock := &mockSDKClient{
		createFn: func(_ context.Context, cfg *copilot.SessionConfig) (sdkSession, error) {
			capturedConfig = cfg
			return expectedSess, nil
		},
	}

	tool := ToolDefinition{
		Name:        "search",
		Description: "Search",
		Handler:     func(args map[string]any) (string, error) { return "ok", nil },
	}

	client := newTestClient(mock, WithTools(tool), WithStreaming(true), WithModel("gpt-5"))
	sess, err := client.getOrCreateSession(t.Context(), "")

	require.NoError(t, err)
	assert.Equal(t, "tools-sess", sess.ID())
	require.NotNil(t, capturedConfig)
	assert.Equal(t, "gpt-5", capturedConfig.Model)
	assert.True(t, capturedConfig.Streaming)
	require.Len(t, capturedConfig.Tools, 1)
	assert.Equal(t, "search", capturedConfig.Tools[0].Name)
}

func TestGetOrCreateSession_ResumeWithBYOK(t *testing.T) {
	expectedSess := &mockSDKSession{id: "byok-sess"}
	var capturedConfig *copilot.ResumeSessionConfig

	mock := &mockSDKClient{
		resumeFn: func(_ context.Context, _ string, cfg *copilot.ResumeSessionConfig) (sdkSession, error) {
			capturedConfig = cfg
			return expectedSess, nil
		},
	}

	client := newTestClient(mock,
		WithBYOK(ProviderAzure, "https://azure.openai.com", "az-key"),
		WithAzureAPIVersion("2024-10-21"),
		WithSystemMessage("You help."),
	)
	sess, err := client.getOrCreateSession(t.Context(), "existing")

	require.NoError(t, err)
	assert.Equal(t, "byok-sess", sess.ID())
	require.NotNil(t, capturedConfig)
	require.NotNil(t, capturedConfig.SystemMessage)
	assert.Equal(t, "append", capturedConfig.SystemMessage.Mode)
	require.NotNil(t, capturedConfig.Provider)
	assert.Equal(t, "azure", capturedConfig.Provider.Type)
}

func TestGetOrCreateSession_ResumeWithoutBYOK(t *testing.T) {
	expectedSess := &mockSDKSession{id: "gh-sess"}
	var capturedConfig *copilot.ResumeSessionConfig

	mock := &mockSDKClient{
		resumeFn: func(_ context.Context, _ string, cfg *copilot.ResumeSessionConfig) (sdkSession, error) {
			capturedConfig = cfg
			return expectedSess, nil
		},
	}

	client := newTestClient(mock) // default GitHub auth
	sess, err := client.getOrCreateSession(t.Context(), "resume-id")

	require.NoError(t, err)
	assert.Equal(t, "gh-sess", sess.ID())
	require.NotNil(t, capturedConfig)
	assert.Nil(t, capturedConfig.Provider)
	assert.Nil(t, capturedConfig.SystemMessage)
}

// ---------------------------------------------------------------------------
// Query (convenience wrapper)
// ---------------------------------------------------------------------------

func TestQuery_DelegatesToQueryWithSession(t *testing.T) {
	sess := &mockSDKSession{id: "query-sess"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, opts copilot.MessageOptions) (string, error) {
		assert.Equal(t, "hello world", opts.Prompt)
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("response")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	client := newTestClient(mock)
	result, err := client.Query(t.Context(), "hello world")

	require.NoError(t, err)
	assert.Equal(t, "response", result.Content)
}

// ---------------------------------------------------------------------------
// Edge cases — ErrSidecarUnavailable error message wrapping
// ---------------------------------------------------------------------------

func TestNewQueryHandler_ErrSidecarUnavailable(t *testing.T) {
	sess := &mockSDKSession{id: "sidecar-sess"}
	mock := &mockSDKClient{
		createFn: func(_ context.Context, _ *copilot.SessionConfig) (sdkSession, error) {
			return sess, nil
		},
	}

	sess.sendFn = func(_ context.Context, _ copilot.MessageOptions) (string, error) {
		go func() {
			sess.emit(copilot.SessionEvent{
				Type: copilot.AssistantMessage,
				Data: copilot.Data{Content: ptr("ok")},
			})
			sess.emit(copilot.SessionEvent{Type: copilot.SessionIdle})
		}()
		return "msg-1", nil
	}

	// Simulate disconnected client to trigger ErrSidecarUnavailable in handler.
	client := &Client{cfg: defaultCfg(), sdk: mock, connected: false}
	handler := NewQueryHandler(client)

	body := `{"prompt": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/query", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestNewStreamHandler_ErrSidecarUnavailable(t *testing.T) {
	mock := &mockSDKClient{}
	client := &Client{cfg: defaultCfg(), sdk: mock, connected: false}
	handler := NewStreamHandler(client)

	body := `{"prompt": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/copilot/stream", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// ---------------------------------------------------------------------------
// sdkTools and buildSessionConfig edge cases
// ---------------------------------------------------------------------------

func TestBuildSessionConfig_WithAllOptions(t *testing.T) {
	tool := ToolDefinition{
		Name:    "t1",
		Handler: func(args map[string]any) (string, error) { return "", nil },
	}

	client := newTestClient(&mockSDKClient{},
		WithModel("gpt-5"),
		WithStreaming(true),
		WithSystemMessage("Be concise."),
		WithBYOK(ProviderAnthropic, "https://api.anthropic.com/v1", "ant-key"),
		WithTools(tool),
	)

	sc := client.buildSessionConfig()

	assert.Equal(t, "gpt-5", sc.Model)
	assert.True(t, sc.Streaming)
	require.NotNil(t, sc.SystemMessage)
	assert.Equal(t, "Be concise.", sc.SystemMessage.Content)
	require.NotNil(t, sc.Provider)
	assert.Equal(t, "anthropic", sc.Provider.Type)
	require.Len(t, sc.Tools, 1)
}
