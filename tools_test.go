package copilotcli

import (
	"fmt"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolDefinitionToSDKTool(t *testing.T) {
	t.Run("converts simple tool", func(t *testing.T) {
		td := ToolDefinition{
			Name:        "my_tool",
			Description: "Does something useful",
			Parameters: []ToolParameter{
				{Name: "input", Type: "string", Description: "The input value", Required: true},
				{Name: "verbose", Type: "boolean", Description: "Enable verbose output", Required: false},
			},
			Handler: func(args map[string]any) (string, error) {
				return "result", nil
			},
		}

		tool := td.toSDKTool()

		assert.Equal(t, "my_tool", tool.Name)
		assert.Equal(t, "Does something useful", tool.Description)

		params := tool.Parameters
		assert.Equal(t, "object", params["type"])

		props, ok := params["properties"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, props, "input")
		assert.Contains(t, props, "verbose")

		inputProp := props["input"].(map[string]any)
		assert.Equal(t, "string", inputProp["type"])
		assert.Equal(t, "The input value", inputProp["description"])

		required := params["required"].([]string)
		assert.Contains(t, required, "input")
		assert.NotContains(t, required, "verbose")
	})

	t.Run("converts tool with no parameters", func(t *testing.T) {
		td := ToolDefinition{
			Name:        "no_params",
			Description: "No params needed",
			Handler:     func(args map[string]any) (string, error) { return "ok", nil },
		}

		tool := td.toSDKTool()

		params := tool.Parameters

		props := params["properties"].(map[string]any)
		assert.Empty(t, props)

		required := params["required"].([]string)
		assert.Empty(t, required)
	})
}

func TestToolHandler_Invocation(t *testing.T) {
	t.Run("successful handler invocation", func(t *testing.T) {
		td := ToolDefinition{
			Name:        "greet",
			Description: "Greets a user",
			Parameters: []ToolParameter{
				{Name: "name", Type: "string", Description: "User name", Required: true},
			},
			Handler: func(args map[string]any) (string, error) {
				return fmt.Sprintf("Hello, %s!", args["name"]), nil
			},
		}

		tool := td.toSDKTool()
		result, err := tool.Handler(copilot.ToolInvocation{
			Arguments: map[string]any{"name": "Alice"},
		})
		require.NoError(t, err)
		assert.Equal(t, "Hello, Alice!", result.TextResultForLLM)
		assert.Equal(t, "success", result.ResultType)
		assert.Contains(t, result.SessionLog, "greet")
		assert.Contains(t, result.SessionLog, "successfully")
	})

	t.Run("handler returns error", func(t *testing.T) {
		td := ToolDefinition{
			Name:        "failing_tool",
			Description: "Always fails",
			Handler: func(args map[string]any) (string, error) {
				return "", fmt.Errorf("database connection lost")
			},
		}

		tool := td.toSDKTool()
		result, err := tool.Handler(copilot.ToolInvocation{
			Arguments: map[string]any{},
		})
		// The SDK handler returns nil error (so SDK doesn't retry), but the result
		// contains the error message for the LLM.
		require.NoError(t, err)
		assert.Equal(t, "error", result.ResultType)
		assert.Contains(t, result.TextResultForLLM, "database connection lost")
		assert.Contains(t, result.SessionLog, "failing_tool")
		assert.Contains(t, result.SessionLog, "failed")
	})

	t.Run("unexpected arguments type returns error", func(t *testing.T) {
		td := ToolDefinition{
			Name:        "typed_tool",
			Description: "Expects map args",
			Handler: func(args map[string]any) (string, error) {
				return "ok", nil
			},
		}

		tool := td.toSDKTool()
		_, err := tool.Handler(copilot.ToolInvocation{
			Arguments: "not-a-map",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected arguments type")
	})

	t.Run("nil arguments returns error", func(t *testing.T) {
		td := ToolDefinition{
			Name:    "nil_args",
			Handler: func(args map[string]any) (string, error) { return "ok", nil },
		}

		tool := td.toSDKTool()
		_, err := tool.Handler(copilot.ToolInvocation{
			Arguments: nil,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected arguments type")
	})
}

func TestDefineTypedTool(t *testing.T) {
	type lookupParams struct {
		Query string `json:"query" description:"The search query"`
	}

	tool := DefineTypedTool("search", "Search for items", func(params lookupParams, inv copilot.ToolInvocation) (any, error) {
		return map[string]string{"result": "found: " + params.Query}, nil
	})

	assert.Equal(t, "search", tool.Name)
	assert.Equal(t, "Search for items", tool.Description)
	assert.NotNil(t, tool.Handler)
}

func TestToolDefinition_AllRequiredParams(t *testing.T) {
	td := ToolDefinition{
		Name:        "all_required",
		Description: "All params required",
		Parameters: []ToolParameter{
			{Name: "a", Type: "string", Description: "param a", Required: true},
			{Name: "b", Type: "number", Description: "param b", Required: true},
			{Name: "c", Type: "boolean", Description: "param c", Required: true},
		},
		Handler: func(args map[string]any) (string, error) { return "ok", nil },
	}

	tool := td.toSDKTool()
	required := tool.Parameters["required"].([]string)
	assert.Len(t, required, 3)
	assert.Contains(t, required, "a")
	assert.Contains(t, required, "b")
	assert.Contains(t, required, "c")
}

func TestToolDefinition_NoRequiredParams(t *testing.T) {
	td := ToolDefinition{
		Name:        "all_optional",
		Description: "All params optional",
		Parameters: []ToolParameter{
			{Name: "x", Type: "string", Description: "optional x", Required: false},
			{Name: "y", Type: "number", Description: "optional y", Required: false},
		},
		Handler: func(args map[string]any) (string, error) { return "ok", nil },
	}

	tool := td.toSDKTool()
	required := tool.Parameters["required"].([]string)
	assert.Empty(t, required)
}
