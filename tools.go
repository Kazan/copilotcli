package copilotcli

import (
	"fmt"

	copilot "github.com/github/copilot-sdk/go"
)

// ToolHandler is the function signature for custom tool handlers.
// The handler receives the raw arguments from the LLM and returns a result string
// that is sent back as context. Handlers execute in-process (in your Go service).
type ToolHandler func(args map[string]any) (string, error)

// ToolParameter describes a single parameter for a custom tool.
type ToolParameter struct {
	Name        string
	Type        string // "string", "number", "boolean", "object", "array"
	Description string
	Required    bool
}

// ToolDefinition describes a custom tool that the LLM can invoke.
type ToolDefinition struct {
	// Name is the tool identifier (e.g., "lookup_inventory").
	Name string

	// Description explains what the tool does — this is what the LLM reads.
	Description string

	// Parameters defines the expected input schema.
	Parameters []ToolParameter

	// Handler is called when the LLM invokes this tool.
	Handler ToolHandler
}

// toSDKTool converts a ToolDefinition into the Copilot SDK's Tool type.
func (td ToolDefinition) toSDKTool() copilot.Tool {
	properties := make(map[string]any, len(td.Parameters))
	required := make([]string, 0)

	for _, p := range td.Parameters {
		properties[p.Name] = map[string]any{
			"type":        p.Type,
			"description": p.Description,
		}
		if p.Required {
			required = append(required, p.Name)
		}
	}

	return copilot.Tool{
		Name:        td.Name,
		Description: td.Description,
		Parameters: map[string]any{
			"type":       "object",
			"properties": properties,
			"required":   required,
		},
		Handler: func(invocation copilot.ToolInvocation) (copilot.ToolResult, error) {
			args, ok := invocation.Arguments.(map[string]any)
			if !ok {
				return copilot.ToolResult{}, fmt.Errorf("unexpected arguments type: %T", invocation.Arguments)
			}

			result, err := td.Handler(args)
			if err != nil {
				return copilot.ToolResult{
					TextResultForLLM: fmt.Sprintf("error: %s", err.Error()),
					ResultType:       "error",
					SessionLog:       fmt.Sprintf("Tool %s failed: %s", td.Name, err.Error()),
				}, nil // return nil to avoid SDK retrying; the LLM sees the error message
			}

			return copilot.ToolResult{
				TextResultForLLM: result,
				ResultType:       "success",
				SessionLog:       fmt.Sprintf("Tool %s executed successfully", td.Name),
			}, nil
		},
	}
}

// DefineTypedTool creates a ToolDefinition using the copilot.DefineTool helper
// for automatic JSON schema generation from a typed struct.
// This is a convenience wrapper — use ToolDefinition directly for more control.
func DefineTypedTool[T any](name, description string, handler func(params T, inv copilot.ToolInvocation) (any, error)) copilot.Tool {
	return copilot.DefineTool(name, description, handler)
}
