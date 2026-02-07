package copilotcli

import (
	"context"
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
		tool1 := ToolDefinition{Name: "t1", Handler: func(_ map[string]any) (string, error) { return "", nil }}
		tool2 := ToolDefinition{Name: "t2", Handler: func(_ map[string]any) (string, error) { return "", nil }}

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
	t.Run("validate fails after options succeed", func(t *testing.T) {
		// Custom option that clears cliURL — option itself succeeds but validate fails.
		clearURL := func(c *cfg) error { c.cliURL = ""; return nil }
		_, err := New(clearURL)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid configuration")
		assert.ErrorIs(t, err, ErrMissingCLIURL)
	})

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

func TestClient_Stop(t *testing.T) {
	t.Run("stop on disconnected client is no-op", func(t *testing.T) {
		client, err := New()
		require.NoError(t, err)
		assert.False(t, client.IsConnected())

		err = client.Stop()
		require.NoError(t, err)
		assert.False(t, client.IsConnected())
	})
}

func TestClient_QueryStreamEmptyPrompt(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	ch, sid, err := client.QueryStream(t.Context(), "", "")
	require.ErrorIs(t, err, ErrEmptyPrompt)
	assert.Nil(t, ch)
	assert.Empty(t, sid)
}

func TestBuildSessionConfig(t *testing.T) {
	t.Run("basic config without system message or BYOK", func(t *testing.T) {
		client, err := New(WithModel("gpt-4o"), WithStreaming(true))
		require.NoError(t, err)

		sc := client.buildSessionConfig()
		assert.Equal(t, "gpt-4o", sc.Model)
		assert.True(t, sc.Streaming)
		assert.Nil(t, sc.SystemMessage)
		assert.Nil(t, sc.Provider)
		assert.Nil(t, sc.Tools)
	})

	t.Run("config with system message", func(t *testing.T) {
		client, err := New(WithSystemMessage("You are an assistant."))
		require.NoError(t, err)

		sc := client.buildSessionConfig()
		require.NotNil(t, sc.SystemMessage)
		assert.Equal(t, "append", sc.SystemMessage.Mode)
		assert.Equal(t, "You are an assistant.", sc.SystemMessage.Content)
	})

	t.Run("config with BYOK provider", func(t *testing.T) {
		client, err := New(
			WithBYOK(ProviderOpenAI, "https://api.openai.com/v1", "sk-test"),
		)
		require.NoError(t, err)

		sc := client.buildSessionConfig()
		require.NotNil(t, sc.Provider)
		assert.Equal(t, "openai", sc.Provider.Type)
		assert.Equal(t, "https://api.openai.com/v1", sc.Provider.BaseURL)
		assert.Equal(t, "sk-test", sc.Provider.APIKey)
	})

	t.Run("config with tools", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "lookup",
			Description: "Look up data",
			Handler:     func(_ map[string]any) (string, error) { return "ok", nil },
		}
		client, err := New(WithTools(tool))
		require.NoError(t, err)

		sc := client.buildSessionConfig()
		require.Len(t, sc.Tools, 1)
		assert.Equal(t, "lookup", sc.Tools[0].Name)
	})
}

func TestBuildProvider(t *testing.T) {
	t.Run("OpenAI provider", func(t *testing.T) {
		client, err := New(
			WithBYOK(ProviderOpenAI, "https://api.openai.com/v1", "sk-key"),
		)
		require.NoError(t, err)

		p := client.buildProvider()
		assert.Equal(t, "openai", p.Type)
		assert.Equal(t, "https://api.openai.com/v1", p.BaseURL)
		assert.Equal(t, "sk-key", p.APIKey)
		assert.Nil(t, p.Azure)
	})

	t.Run("Azure provider with API version", func(t *testing.T) {
		client, err := New(
			WithBYOK(ProviderAzure, "https://my-azure.openai.azure.com", "az-key"),
			WithAzureAPIVersion("2024-10-21"),
		)
		require.NoError(t, err)

		p := client.buildProvider()
		assert.Equal(t, "azure", p.Type)
		assert.Equal(t, "https://my-azure.openai.azure.com", p.BaseURL)
		assert.Equal(t, "az-key", p.APIKey)
		require.NotNil(t, p.Azure)
		assert.Equal(t, "2024-10-21", p.Azure.APIVersion)
	})

	t.Run("Azure provider without API version", func(t *testing.T) {
		client, err := New(
			WithBYOK(ProviderAzure, "https://my-azure.openai.azure.com", "az-key"),
		)
		require.NoError(t, err)

		p := client.buildProvider()
		assert.Nil(t, p.Azure)
	})

	t.Run("Anthropic provider", func(t *testing.T) {
		client, err := New(
			WithBYOK(ProviderAnthropic, "https://api.anthropic.com/v1", "ant-key"),
		)
		require.NoError(t, err)

		p := client.buildProvider()
		assert.Equal(t, "anthropic", p.Type)
		assert.Equal(t, "https://api.anthropic.com/v1", p.BaseURL)
		assert.Equal(t, "ant-key", p.APIKey)
		assert.Nil(t, p.Azure)
	})
}

func TestSDKTools(t *testing.T) {
	t.Run("returns nil when no tools configured", func(t *testing.T) {
		client, err := New()
		require.NoError(t, err)

		tools := client.sdkTools()
		assert.Nil(t, tools)
	})

	t.Run("converts tool definitions", func(t *testing.T) {
		td1 := ToolDefinition{
			Name:        "tool_a",
			Description: "Tool A",
			Parameters: []ToolParameter{
				{Name: "x", Type: "string", Description: "param x", Required: true},
			},
			Handler: func(_ map[string]any) (string, error) { return "a", nil },
		}
		td2 := ToolDefinition{
			Name:        "tool_b",
			Description: "Tool B",
			Handler:     func(_ map[string]any) (string, error) { return "b", nil },
		}

		client, err := New(WithTools(td1, td2))
		require.NoError(t, err)

		tools := client.sdkTools()
		require.Len(t, tools, 2)
		assert.Equal(t, "tool_a", tools[0].Name)
		assert.Equal(t, "tool_b", tools[1].Name)
	})
}

func TestWithGitHubAuth(t *testing.T) {
	client, err := New(WithGitHubAuth())
	require.NoError(t, err)
	assert.Equal(t, AuthModeGitHub, client.cfg.authMode)
}

func TestWithGitHubAuth_OverridesBYOK(t *testing.T) {
	client, err := New(
		WithBYOK(ProviderOpenAI, "https://api.openai.com/v1", "sk-key"),
		WithGitHubAuth(),
	)
	require.NoError(t, err)
	assert.Equal(t, AuthModeGitHub, client.cfg.authMode)
}

func TestClient_Start(t *testing.T) {
	t.Run("returns ErrAlreadyConnected when already connected", func(t *testing.T) {
		client, err := New()
		require.NoError(t, err)

		// Simulate already connected state.
		client.connected = true

		err = client.Start(t.Context())
		assert.ErrorIs(t, err, ErrAlreadyConnected)
	})

	t.Run("returns ErrSidecarUnavailable after retry exhaustion", func(t *testing.T) {
		client, err := New(
			WithRetryAttempts(1),
			WithConnTimeout(50*time.Millisecond),
			WithRetryDelay(10*time.Millisecond),
		)
		require.NoError(t, err)

		err = client.Start(t.Context())
		require.ErrorIs(t, err, ErrSidecarUnavailable)
		assert.False(t, client.IsConnected())
	})

	t.Run("returns ErrSidecarUnavailable with multiple retries", func(t *testing.T) {
		client, err := New(
			WithRetryAttempts(2),
			WithConnTimeout(50*time.Millisecond),
			WithRetryDelay(10*time.Millisecond),
		)
		require.NoError(t, err)

		err = client.Start(t.Context())
		assert.ErrorIs(t, err, ErrSidecarUnavailable)
	})

	t.Run("context cancellation during retries", func(t *testing.T) {
		client, err := New(
			WithRetryAttempts(5),
			WithConnTimeout(50*time.Millisecond),
			WithRetryDelay(50*time.Millisecond),
		)
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
		defer cancel()

		err = client.Start(ctx)
		require.Error(t, err)
		assert.False(t, client.IsConnected())
	})
}

func TestClient_StopConnected(t *testing.T) {
	// Force connected=true and call Stop. The underlying SDK wasn't started
	// so sdk.Stop() may return an error, but connected should become false.
	client, err := New()
	require.NoError(t, err)

	client.connected = true
	assert.True(t, client.IsConnected())

	_ = client.Stop()
	assert.False(t, client.IsConnected())
}

func TestClient_PingConnected(t *testing.T) {
	// Force connected=true — Ping will call the SDK which isn't actually connected,
	// so it will likely error. But this exercises the non-ErrNotConnected path.
	client, err := New()
	require.NoError(t, err)

	client.connected = true

	err = client.Ping(t.Context())
	// The SDK isn't started so this should return an error (but NOT ErrNotConnected).
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotConnected)
}

func TestClient_QueryWithSession_ConnectedButSDKNotStarted(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	client.connected = true

	// New session (empty sessionID) — will fail at CreateSession since SDK isn't started.
	_, err = client.QueryWithSession(t.Context(), "", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session setup")
}

func TestClient_QueryWithSession_ResumeSession(t *testing.T) {
	client, err := New(
		WithSystemMessage("You are a bot."),
		WithBYOK(ProviderOpenAI, "https://api.openai.com/v1", "sk-key"),
	)
	require.NoError(t, err)

	client.connected = true

	// Resuming session — will fail at ResumeSessionWithOptions since SDK isn't started.
	_, err = client.QueryWithSession(t.Context(), "existing-session-id", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session setup")
}

func TestClient_QueryStream_ConnectedButSDKNotStarted(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	client.connected = true

	ch, sid, err := client.QueryStream(t.Context(), "", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session setup")
	assert.Nil(t, ch)
	assert.Empty(t, sid)
}

func TestClient_QueryStream_ResumeSession(t *testing.T) {
	client, err := New(
		WithSystemMessage("You help."),
	)
	require.NoError(t, err)

	client.connected = true

	ch, sid, err := client.QueryStream(t.Context(), "some-session", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session setup")
	assert.Nil(t, ch)
	assert.Empty(t, sid)
}

func TestClient_DestroySession_Connected(t *testing.T) {
	client, err := New()
	require.NoError(t, err)

	client.connected = true

	// Will fail because SDK not started, but exercises the non-ErrNotConnected path.
	err = client.DestroySession(t.Context(), "sess-123")
	assert.Error(t, err)
}
