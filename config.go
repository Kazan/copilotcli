package copilotcli

import "time"

const (
	defaultCLIURL        = "localhost:4321"
	defaultLogLevel      = "error"
	defaultModel         = "gpt-4o"
	defaultConnTimeout   = 10 * time.Second
	defaultRetryAttempts = 5
	defaultRetryDelay    = 500 * time.Millisecond
)

// AuthMode defines how the Copilot CLI sidecar authenticates with the LLM provider.
type AuthMode string

const (
	// AuthModeGitHub authenticates via a GitHub token with Copilot access.
	// Requires a valid GitHub Copilot subscription.
	AuthModeGitHub AuthMode = "github"

	// AuthModeBYOK uses a custom OpenAI-compatible provider (Bring Your Own Key).
	// No GitHub auth required — uses provider-specific API keys directly.
	AuthModeBYOK AuthMode = "byok"
)

// ProviderType identifies the BYOK provider type.
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAzure     ProviderType = "azure"
	ProviderAnthropic ProviderType = "anthropic"
)

// cfg is the internal resolved configuration built from functional options.
type cfg struct {
	cliURL          string
	logLevel        string
	model           string
	authMode        AuthMode
	streaming       bool
	connTimeout     time.Duration
	retryAttempts   int
	retryDelay      time.Duration
	systemMessage   string
	tools           []ToolDefinition
	providerType    ProviderType
	providerBaseURL string
	providerAPIKey  string
	azureAPIVersion string
}

func defaultCfg() *cfg {
	return &cfg{
		cliURL:        defaultCLIURL,
		logLevel:      defaultLogLevel,
		model:         defaultModel,
		authMode:      AuthModeGitHub,
		connTimeout:   defaultConnTimeout,
		retryAttempts: defaultRetryAttempts,
		retryDelay:    defaultRetryDelay,
		providerType:  ProviderOpenAI,
	}
}

func (c *cfg) validate() error {
	if c.cliURL == "" {
		return ErrMissingCLIURL
	}
	if c.authMode == AuthModeBYOK {
		if c.model == "" {
			return ErrMissingModel
		}
		if c.providerBaseURL == "" {
			return ErrMissingProviderBaseURL
		}
	}
	return nil
}
