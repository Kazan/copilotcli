// Package copilotcli provides a high-level Go client for the headless Copilot CLI sidecar.
// It wraps the Copilot SDK and offers query, streaming, retry, and HTTP handler
// functionality for integrating Copilot into backend services.
package copilotcli

import (
	"context"
	"fmt"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

// QueryResult holds the response from a single LLM query.
type QueryResult struct {
	Content   string
	SessionID string
}

// StreamEvent represents a single streaming event (a delta or the final result).
type StreamEvent struct {
	DeltaContent string
	Content      string // populated only in the final event
	IsFinal      bool
	Error        error
}

// Client wraps the Copilot CLI SDK client and manages connectivity to a
// headless Copilot CLI sidecar.
type Client struct {
	cfg       *cfg
	sdk       sdkClient
	connected bool
	mu        sync.RWMutex
}

// New creates a new Client with the supplied functional options.
// It validates the resolved configuration but does not connect to the sidecar.
// Call Start to establish connectivity.
func New(opts ...Option) (*Client, error) {
	c := defaultCfg()

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("applying option: %w", err)
		}
	}

	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	sdkClient := copilot.NewClient(&copilot.ClientOptions{
		CLIUrl:   c.cliURL,
		LogLevel: c.logLevel,
	})

	return &Client{
		cfg: c,
		sdk: &sdkClientAdapter{c: sdkClient},
	}, nil
}

// Start connects to the Copilot CLI sidecar with retry and exponential backoff.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return ErrAlreadyConnected
	}

	var lastErr error
	delay := c.cfg.retryDelay

	for attempt := range c.cfg.retryAttempts {
		connCtx, cancel := context.WithTimeout(ctx, c.cfg.connTimeout)
		err := c.sdk.Start(connCtx)
		cancel()

		if err == nil {
			c.connected = true
			return nil
		}

		lastErr = err

		// Don't sleep after the last attempt.
		if attempt < c.cfg.retryAttempts-1 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("%w: %w", ErrSidecarUnavailable, ctx.Err())
			case <-time.After(delay):
			}
			delay *= 2
		}
	}

	return fmt.Errorf("%w: %w", ErrSidecarUnavailable, lastErr)
}

// Stop disconnects from the Copilot CLI sidecar.
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	err := c.sdk.Stop()
	c.connected = false
	return err
}

// IsConnected reports whether the client has an active connection to the sidecar.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connected
}

// Ping checks that the sidecar is responsive. Returns an error if it is not.
func (c *Client) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.connected {
		return ErrNotConnected
	}

	_, err := c.sdk.Ping(ctx, "health")
	return err
}

// Query sends a prompt to the LLM in a new session and returns the complete response.
func (c *Client) Query(ctx context.Context, prompt string) (*QueryResult, error) {
	return c.QueryWithSession(ctx, "", prompt)
}

// QueryWithSession sends a prompt in an existing session (multi-turn) or creates
// a new one when sessionID is empty.
func (c *Client) QueryWithSession(ctx context.Context, sessionID, prompt string) (*QueryResult, error) {
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}

	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, ErrNotConnected
	}
	c.mu.RUnlock()

	session, err := c.getOrCreateSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session setup: %w", err)
	}

	var (
		content string
		done    = make(chan struct{})
		mu      sync.Mutex
		evtErr  error
	)

	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch event.Type {
		case copilot.AssistantMessage:
			mu.Lock()
			if event.Data.Content != nil {
				content = *event.Data.Content
			}
			mu.Unlock()
		case copilot.SessionIdle:
			close(done)
		case copilot.SessionError:
			mu.Lock()
			msg := "session error"
			if event.Data.Message != nil {
				msg = *event.Data.Message
			}
			evtErr = fmt.Errorf("copilot: %s", msg)
			mu.Unlock()
			close(done)
		default:
			// Ignore other event types.
		}
	})
	defer unsubscribe()

	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: prompt}); err != nil {
		return nil, fmt.Errorf("sending message: %w", err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		_ = session.Abort(ctx)
		return nil, ctx.Err()
	}

	mu.Lock()
	defer mu.Unlock()

	if evtErr != nil {
		return nil, evtErr
	}

	return &QueryResult{
		Content:   content,
		SessionID: session.ID(),
	}, nil
}

// QueryStream sends a prompt and returns a channel of streaming events plus
// the session ID. The channel is closed when the response completes.
func (c *Client) QueryStream(ctx context.Context, sessionID, prompt string) (<-chan StreamEvent, string, error) { //nolint:gocritic // named returns not used to keep internal channel writable
	if prompt == "" {
		return nil, "", ErrEmptyPrompt
	}

	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return nil, "", ErrNotConnected
	}
	c.mu.RUnlock()

	session, err := c.getOrCreateSession(ctx, sessionID)
	if err != nil {
		return nil, "", fmt.Errorf("session setup: %w", err)
	}

	events := make(chan StreamEvent, 64)

	var (
		fullContent string
		mu          sync.Mutex
	)

	unsubscribe := session.On(func(event copilot.SessionEvent) {
		switch event.Type {
		case copilot.AssistantMessageDelta:
			if event.Data.DeltaContent != nil {
				mu.Lock()
				fullContent += *event.Data.DeltaContent
				mu.Unlock()
				events <- StreamEvent{DeltaContent: *event.Data.DeltaContent}
			}
		case copilot.AssistantMessage:
			mu.Lock()
			if event.Data.Content != nil {
				fullContent = *event.Data.Content
			}
			mu.Unlock()
		case copilot.SessionIdle:
			mu.Lock()
			events <- StreamEvent{Content: fullContent, IsFinal: true}
			mu.Unlock()
			close(events)
		case copilot.SessionError:
			msg := "session error"
			if event.Data.Message != nil {
				msg = *event.Data.Message
			}
			events <- StreamEvent{Error: fmt.Errorf("copilot: %s", msg)}
			close(events)
		default:
			// Ignore other event types.
		}
	})

	go func() {
		<-ctx.Done()
		unsubscribe()
	}()

	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: prompt}); err != nil {
		unsubscribe()
		close(events)
		return nil, "", fmt.Errorf("sending message: %w", err)
	}

	return events, session.ID(), nil
}

// DestroySession deletes a session on the sidecar.
func (c *Client) DestroySession(ctx context.Context, sessionID string) error {
	c.mu.RLock()
	if !c.connected {
		c.mu.RUnlock()
		return ErrNotConnected
	}
	c.mu.RUnlock()

	return c.sdk.DeleteSession(ctx, sessionID)
}

// getOrCreateSession resumes an existing session or creates a new one with
// the client's configured tools, model, and provider settings.
func (c *Client) getOrCreateSession(ctx context.Context, sessionID string) (sdkSession, error) {
	if sessionID != "" {
		resumeCfg := &copilot.ResumeSessionConfig{
			Model:     c.cfg.model,
			Streaming: c.cfg.streaming,
			Tools:     c.sdkTools(),
		}
		if c.cfg.systemMessage != "" {
			resumeCfg.SystemMessage = &copilot.SystemMessageConfig{
				Mode:    "append",
				Content: c.cfg.systemMessage,
			}
		}
		if c.cfg.authMode == AuthModeBYOK {
			resumeCfg.Provider = c.buildProvider()
		}
		return c.sdk.ResumeSessionWithOptions(ctx, sessionID, resumeCfg)
	}

	sessionCfg := c.buildSessionConfig()
	return c.sdk.CreateSession(ctx, sessionCfg)
}

// buildSessionConfig assembles a SessionConfig from the client's resolved cfg.
func (c *Client) buildSessionConfig() *copilot.SessionConfig {
	sc := &copilot.SessionConfig{
		Model:     c.cfg.model,
		Streaming: c.cfg.streaming,
		Tools:     c.sdkTools(),
	}

	if c.cfg.systemMessage != "" {
		sc.SystemMessage = &copilot.SystemMessageConfig{
			Mode:    "append",
			Content: c.cfg.systemMessage,
		}
	}

	if c.cfg.authMode == AuthModeBYOK {
		sc.Provider = c.buildProvider()
	}

	return sc
}

// buildProvider creates a ProviderConfig from the client's resolved cfg.
func (c *Client) buildProvider() *copilot.ProviderConfig {
	p := &copilot.ProviderConfig{
		Type:    string(c.cfg.providerType),
		BaseURL: c.cfg.providerBaseURL,
		APIKey:  c.cfg.providerAPIKey,
	}

	if c.cfg.providerType == ProviderAzure && c.cfg.azureAPIVersion != "" {
		p.Azure = &copilot.AzureProviderOptions{
			APIVersion: c.cfg.azureAPIVersion,
		}
	}

	return p
}

// sdkTools converts the configured ToolDefinitions to SDK Tool values.
func (c *Client) sdkTools() []copilot.Tool {
	if len(c.cfg.tools) == 0 {
		return nil
	}

	tools := make([]copilot.Tool, len(c.cfg.tools))
	for i, td := range c.cfg.tools {
		tools[i] = td.toSDKTool()
	}
	return tools
}
