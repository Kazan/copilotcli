package copilotcli

import (
	"testing"

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
