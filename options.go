package copilotcli

import (
	"errors"
	"fmt"
	"time"
)

// Option configures the Client. Pass options to New.
type Option func(*cfg) error

// WithCLIURL sets the address of the headless Copilot CLI server.
// In a K8s sidecar setup this is typically "localhost:4321" (the default).
func WithCLIURL(url string) Option {
	return func(c *cfg) error {
		if url == "" {
			return errors.New("CLI URL must not be empty")
		}
		c.cliURL = url
		return nil
	}
}

// WithLogLevel sets the SDK log verbosity ("error", "info", "debug").
// Default: "error".
func WithLogLevel(level string) Option {
	return func(c *cfg) error {
		c.logLevel = level
		return nil
	}
}

// WithModel sets the LLM model to use (e.g., "gpt-5", "claude-sonnet-4.5").
// Default: "gpt-4o". Required when using BYOK auth mode.
func WithModel(model string) Option {
	return func(c *cfg) error {
		if model == "" {
			return errors.New("model must not be empty")
		}
		c.model = model
		return nil
	}
}

// WithStreaming enables streaming delta events from the LLM.
func WithStreaming(enabled bool) Option {
	return func(c *cfg) error {
		c.streaming = enabled
		return nil
	}
}

// WithConnTimeout sets the maximum time to wait for the sidecar to respond
// on each connection attempt. Default: 10s.
func WithConnTimeout(d time.Duration) Option {
	return func(c *cfg) error {
		if d <= 0 {
			return errors.New("connection timeout must be positive")
		}
		c.connTimeout = d
		return nil
	}
}

// WithRetryAttempts sets how many times to retry connecting to the sidecar
// on startup. Default: 5.
func WithRetryAttempts(n int) Option {
	return func(c *cfg) error {
		if n <= 0 {
			return errors.New("retry attempts must be positive")
		}
		c.retryAttempts = n
		return nil
	}
}

// WithRetryDelay sets the base delay between connection retries.
// The delay doubles after each failed attempt (exponential backoff).
// Default: 500ms.
func WithRetryDelay(d time.Duration) Option {
	return func(c *cfg) error {
		if d <= 0 {
			return errors.New("retry delay must be positive")
		}
		c.retryDelay = d
		return nil
	}
}

// WithSystemMessage sets a system prompt prepended to every session.
func WithSystemMessage(msg string) Option {
	return func(c *cfg) error {
		c.systemMessage = msg
		return nil
	}
}

// WithTools registers custom tools that the LLM can invoke during a session.
// Tool handlers execute in-process (in your Go service), not in the sidecar.
func WithTools(tools ...ToolDefinition) Option {
	return func(c *cfg) error {
		c.tools = append(c.tools, tools...)
		return nil
	}
}

// WithGitHubAuth configures the client to authenticate via a GitHub token
// with Copilot access. This is the default auth mode.
func WithGitHubAuth() Option {
	return func(c *cfg) error {
		c.authMode = AuthModeGitHub
		return nil
	}
}

// WithBYOK configures the client to use a custom OpenAI-compatible provider
// (Bring Your Own Key). No GitHub auth required.
//
// providerType is one of ProviderOpenAI, ProviderAzure, or ProviderAnthropic.
// baseURL is the API endpoint (e.g., "https://api.openai.com/v1").
// apiKey is the provider API key (may be empty for local providers like Ollama).
func WithBYOK(providerType ProviderType, baseURL, apiKey string) Option {
	return func(c *cfg) error {
		if baseURL == "" {
			return fmt.Errorf("%w: base URL is required for BYOK", ErrMissingProviderBaseURL)
		}
		c.authMode = AuthModeBYOK
		c.providerType = providerType
		c.providerBaseURL = baseURL
		c.providerAPIKey = apiKey
		return nil
	}
}

// WithAzureAPIVersion sets the Azure API version when using ProviderAzure.
// Default: not set (SDK uses its own default).
func WithAzureAPIVersion(version string) Option {
	return func(c *cfg) error {
		c.azureAPIVersion = version
		return nil
	}
}
