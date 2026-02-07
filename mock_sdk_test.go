package copilotcli

import (
	"context"
	"sync"

	copilot "github.com/github/copilot-sdk/go"
)

// mockSDKClient is a test double implementing sdkClient.
type mockSDKClient struct {
	startFn  func(ctx context.Context) error
	stopFn   func() error
	pingFn   func(ctx context.Context, message string) (*copilot.PingResponse, error)
	createFn func(ctx context.Context, config *copilot.SessionConfig) (sdkSession, error)
	resumeFn func(ctx context.Context, sessionID string, config *copilot.ResumeSessionConfig) (sdkSession, error)
	deleteFn func(ctx context.Context, sessionID string) error
}

func (m *mockSDKClient) Start(ctx context.Context) error {
	if m.startFn != nil {
		return m.startFn(ctx)
	}
	return nil
}

func (m *mockSDKClient) Stop() error {
	if m.stopFn != nil {
		return m.stopFn()
	}
	return nil
}

func (m *mockSDKClient) Ping(ctx context.Context, message string) (*copilot.PingResponse, error) {
	if m.pingFn != nil {
		return m.pingFn(ctx, message)
	}
	return &copilot.PingResponse{}, nil
}

func (m *mockSDKClient) CreateSession(ctx context.Context, config *copilot.SessionConfig) (sdkSession, error) {
	if m.createFn != nil {
		return m.createFn(ctx, config)
	}
	return nil, nil
}

func (m *mockSDKClient) ResumeSessionWithOptions(ctx context.Context, sessionID string, config *copilot.ResumeSessionConfig) (sdkSession, error) {
	if m.resumeFn != nil {
		return m.resumeFn(ctx, sessionID, config)
	}
	return nil, nil
}

func (m *mockSDKClient) DeleteSession(ctx context.Context, sessionID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, sessionID)
	}
	return nil
}

// mockSDKSession is a test double implementing sdkSession.
type mockSDKSession struct {
	id      string
	onFn    func(handler func(event copilot.SessionEvent)) func()
	sendFn  func(ctx context.Context, options copilot.MessageOptions) (string, error)
	abortFn func(ctx context.Context) error

	// mu protects handlers for concurrent access.
	mu       sync.Mutex
	handlers []func(event copilot.SessionEvent)
}

func (m *mockSDKSession) ID() string { return m.id }

func (m *mockSDKSession) On(handler func(event copilot.SessionEvent)) func() {
	if m.onFn != nil {
		return m.onFn(handler)
	}

	// Default: store handler, return unsubscribe.
	m.mu.Lock()
	m.handlers = append(m.handlers, handler)
	idx := len(m.handlers) - 1
	m.mu.Unlock()

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if idx < len(m.handlers) {
			m.handlers[idx] = nil
		}
	}
}

func (m *mockSDKSession) Send(ctx context.Context, options copilot.MessageOptions) (string, error) {
	if m.sendFn != nil {
		return m.sendFn(ctx, options)
	}
	return "msg-1", nil
}

func (m *mockSDKSession) Abort(ctx context.Context) error {
	if m.abortFn != nil {
		return m.abortFn(ctx)
	}
	return nil
}

// emit dispatches an event to all registered handlers. Thread-safe.
func (m *mockSDKSession) emit(event *copilot.SessionEvent) {
	m.mu.Lock()
	handlers := make([]func(event copilot.SessionEvent), len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	for _, h := range handlers {
		if h != nil {
			h(*event)
		}
	}
}

// newTestClient creates a Client with a mock SDK already in connected state.
func newTestClient(mock *mockSDKClient, opts ...Option) *Client {
	c := defaultCfg()
	for _, opt := range opts {
		_ = opt(c)
	}

	return &Client{
		cfg:       c,
		sdk:       mock,
		connected: true,
	}
}

// ptr returns a pointer to the given value. Useful for optional fields.
func ptr[T any](v T) *T { return &v }
