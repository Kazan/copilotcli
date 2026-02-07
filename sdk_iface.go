package copilotcli

import (
	"context"

	copilot "github.com/github/copilot-sdk/go"
)

// sdkClient abstracts the Copilot SDK client for testability.
type sdkClient interface {
	Start(ctx context.Context) error
	Stop() error
	Ping(ctx context.Context, message string) (*copilot.PingResponse, error)
	CreateSession(ctx context.Context, config *copilot.SessionConfig) (sdkSession, error)
	ResumeSessionWithOptions(ctx context.Context, sessionID string, config *copilot.ResumeSessionConfig) (sdkSession, error)
	DeleteSession(ctx context.Context, sessionID string) error
}

// sdkSession abstracts a Copilot SDK session for testability.
type sdkSession interface {
	On(handler func(event copilot.SessionEvent)) func()
	Send(ctx context.Context, options copilot.MessageOptions) (string, error)
	Abort(ctx context.Context) error
	ID() string
}

// sdkClientAdapter wraps *copilot.Client to satisfy sdkClient.
type sdkClientAdapter struct {
	c *copilot.Client
}

func (a *sdkClientAdapter) Start(ctx context.Context) error {
	return a.c.Start(ctx)
}

func (a *sdkClientAdapter) Stop() error {
	return a.c.Stop()
}

func (a *sdkClientAdapter) Ping(ctx context.Context, message string) (*copilot.PingResponse, error) {
	return a.c.Ping(ctx, message)
}

func (a *sdkClientAdapter) CreateSession(ctx context.Context, config *copilot.SessionConfig) (sdkSession, error) {
	s, err := a.c.CreateSession(ctx, config)
	if err != nil {
		return nil, err
	}
	return &sdkSessionAdapter{s: s}, nil
}

func (a *sdkClientAdapter) ResumeSessionWithOptions(ctx context.Context, sessionID string, config *copilot.ResumeSessionConfig) (sdkSession, error) {
	s, err := a.c.ResumeSessionWithOptions(ctx, sessionID, config)
	if err != nil {
		return nil, err
	}
	return &sdkSessionAdapter{s: s}, nil
}

func (a *sdkClientAdapter) DeleteSession(ctx context.Context, sessionID string) error {
	return a.c.DeleteSession(ctx, sessionID)
}

// sdkSessionAdapter wraps *copilot.Session to satisfy sdkSession.
type sdkSessionAdapter struct {
	s *copilot.Session
}

func (a *sdkSessionAdapter) On(handler func(event copilot.SessionEvent)) func() {
	return a.s.On(handler)
}

func (a *sdkSessionAdapter) Send(ctx context.Context, options copilot.MessageOptions) (string, error) {
	return a.s.Send(ctx, options)
}

func (a *sdkSessionAdapter) Abort(ctx context.Context) error {
	return a.s.Abort(ctx)
}

func (a *sdkSessionAdapter) ID() string {
	return a.s.SessionID
}
