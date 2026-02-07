package copilotcli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("defaults produce a valid client", func(t *testing.T) {
		client, err := New()
		require.NoError(t, err)
		require.NotNil(t, client)

		assert.Equal(t, defaultCLIURL, client.cfg.cliURL)
		assert.Equal(t, defaultModel, client.cfg.model)
		assert.Equal(t, defaultLogLevel, client.cfg.logLevel)
		assert.Equal(t, AuthModeGitHub, client.cfg.authMode)
		assert.Equal(t, defaultConnTimeout, client.cfg.connTimeout)
		assert.Equal(t, defaultRetryAttempts, client.cfg.retryAttempts)
		assert.Equal(t, defaultRetryDelay, client.cfg.retryDelay)
		assert.False(t, client.IsConnected())
	})

	t.Run("options override defaults", func(t *testing.T) {
		client, err := New(
			WithCLIURL("remote-host:9999"),
			WithModel("claude-sonnet-4"),
			WithLogLevel("debug"),
			WithStreaming(true),
			WithConnTimeout(30*time.Second),
			WithRetryAttempts(10),
			WithRetryDelay(2*time.Second),
			WithSystemMessage("You are a helpful inventory assistant."),
		)
		require.NoError(t, err)

		assert.Equal(t, "remote-host:9999", client.cfg.cliURL)
		assert.Equal(t, "claude-sonnet-4", client.cfg.model)
		assert.Equal(t, "debug", client.cfg.logLevel)
		assert.True(t, client.cfg.streaming)
		assert.Equal(t, 30*time.Second, client.cfg.connTimeout)
		assert.Equal(t, 10, client.cfg.retryAttempts)
		assert.Equal(t, 2*time.Second, client.cfg.retryDelay)
		assert.Equal(t, "You are a helpful inventory assistant.", client.cfg.systemMessage)
	})

	t.Run("WithBYOK configures provider", func(t *testing.T) {
		client, err := New(
			WithBYOK(ProviderAzure, "https://my-azure.openai.azure.com", "az-key-123"),
			WithAzureAPIVersion("2024-10-21"),
		)
		require.NoError(t, err)

		assert.Equal(t, AuthModeBYOK, client.cfg.authMode)
		assert.Equal(t, ProviderAzure, client.cfg.providerType)
		assert.Equal(t, "https://my-azure.openai.azure.com", client.cfg.providerBaseURL)
		assert.Equal(t, "az-key-123", client.cfg.providerAPIKey)
		assert.Equal(t, "2024-10-21", client.cfg.azureAPIVersion)
	})

	t.Run("WithTools appends tool definitions", func(t *testing.T) {
		tool1 := ToolDefinition{Name: "t1", Handler: func(args map[string]any) (string, error) { return "", nil }}
		tool2 := ToolDefinition{Name: "t2", Handler: func(args map[string]any) (string, error) { return "", nil }}

		client, err := New(
			WithTools(tool1),
			WithTools(tool2),
		)
		require.NoError(t, err)
		assert.Len(t, client.cfg.tools, 2)
		assert.Equal(t, "t1", client.cfg.tools[0].Name)
		assert.Equal(t, "t2", client.cfg.tools[1].Name)
	})
}

func TestNew_ValidationErrors(t *testing.T) {
	t.Run("empty CLI URL", func(t *testing.T) {
		_, err := New(WithCLIURL(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "CLI URL must not be empty")
	})

	t.Run("empty model", func(t *testing.T) {
		_, err := New(WithModel(""))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model must not be empty")
	})

	t.Run("negative connection timeout", func(t *testing.T) {
		_, err := New(WithConnTimeout(-1 * time.Second))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "connection timeout must be positive")
	})

	t.Run("zero retry attempts", func(t *testing.T) {
		_, err := New(WithRetryAttempts(0))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retry attempts must be positive")
	})

	t.Run("negative retry delay", func(t *testing.T) {
		_, err := New(WithRetryDelay(-1 * time.Millisecond))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retry delay must be positive")
	})

	t.Run("BYOK without base URL", func(t *testing.T) {
		_, err := New(WithBYOK(ProviderOpenAI, "", "key"))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrMissingProviderBaseURL)
	})
}

func TestClient_DisconnectedState(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	t.Run("Query returns ErrNotConnected", func(t *testing.T) {
		_, err := client.Query(t.Context(), "hello")
		assert.ErrorIs(t, err, ErrNotConnected)
	})

	t.Run("QueryStream returns ErrNotConnected", func(t *testing.T) {
		_, _, err := client.QueryStream(t.Context(), "", "hello")
		assert.ErrorIs(t, err, ErrNotConnected)
	})

	t.Run("Ping returns ErrNotConnected", func(t *testing.T) {
		err := client.Ping(t.Context())
		assert.ErrorIs(t, err, ErrNotConnected)
	})

	t.Run("DestroySession returns ErrNotConnected", func(t *testing.T) {
		err := client.DestroySession(t.Context(), "some-id")
		assert.ErrorIs(t, err, ErrNotConnected)
	})

	t.Run("Query with empty prompt returns ErrEmptyPrompt", func(t *testing.T) {
		_, err := client.Query(t.Context(), "")
		assert.ErrorIs(t, err, ErrEmptyPrompt)
	})
}

func TestDefaultCfg(t *testing.T) {
	c := defaultCfg()

	assert.Equal(t, defaultCLIURL, c.cliURL)
	assert.Equal(t, defaultLogLevel, c.logLevel)
	assert.Equal(t, defaultModel, c.model)
	assert.Equal(t, AuthModeGitHub, c.authMode)
	assert.Equal(t, defaultConnTimeout, c.connTimeout)
	assert.Equal(t, defaultRetryAttempts, c.retryAttempts)
	assert.Equal(t, defaultRetryDelay, c.retryDelay)
	assert.Equal(t, ProviderOpenAI, c.providerType)
}

func TestCfgValidate(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		c := defaultCfg()
		assert.NoError(t, c.validate())
	})

	t.Run("empty CLI URL fails", func(t *testing.T) {
		c := defaultCfg()
		c.cliURL = ""
		assert.ErrorIs(t, c.validate(), ErrMissingCLIURL)
	})

	t.Run("BYOK without model fails", func(t *testing.T) {
		c := defaultCfg()
		c.authMode = AuthModeBYOK
		c.model = ""
		c.providerBaseURL = "https://api.example.com"
		assert.ErrorIs(t, c.validate(), ErrMissingModel)
	})

	t.Run("BYOK without base URL fails", func(t *testing.T) {
		c := defaultCfg()
		c.authMode = AuthModeBYOK
		c.providerBaseURL = ""
		assert.ErrorIs(t, c.validate(), ErrMissingProviderBaseURL)
	})

	t.Run("BYOK with model and base URL passes", func(t *testing.T) {
		c := defaultCfg()
		c.authMode = AuthModeBYOK
		c.providerBaseURL = "https://api.example.com"
		assert.NoError(t, c.validate())
	})
}
