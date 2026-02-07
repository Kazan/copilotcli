# copilotcli

Reusable Go package for integrating with the [GitHub Copilot CLI SDK](https://github.com/github/copilot-sdk) via a Kubernetes sidecar pattern.

> **Warning:** The Copilot CLI SDK is in **Technical Preview** (v0.1.x) and may not yet be suitable for production use.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                    Kubernetes Pod                     │
│                                                      │
│  ┌───────────────────┐     ┌──────────────────────┐  │
│  │   Your Go Service  │     │  copilot-cli sidecar │  │
│  │                    │     │                      │  │
│  │  pkg/copilotcli    │────▶│  copilot --headless  │  │
│  │  (this package)    │TCP  │  --port 4321         │  │
│  │                    │4321 │  --no-auto-update    │  │
│  │  POST /api/copilot │     │                      │  │
│  │       /query       │     │                      │  │
│  └───────────────────┘     └──────┬───────────────┘  │
│          ↑                        │                   │
│      Ingress/SVC             HTTPS (outbound)         │
│                                   ↓                   │
│                          GitHub API / LLM Provider    │
└─────────────────────────────────────────────────────┘
```

The Copilot CLI runs as a headless JSON-RPC server in a sidecar container. This package connects to it over `localhost:4321` (pod-internal TCP), sends prompts, and receives LLM responses. Custom tools execute in-process in your Go service.

## Installation

```bash
go get github.com/github/copilot-sdk/go
```

There is no separate installation for this package — add it to your module and it imports the SDK as a dependency.

```bash
go get github.com/kazan/copilotcli
```

## Quick Start

### 1. Create Client and Register HTTP Handlers

The package uses functional options — no config struct, no environment reading.

```go
package main

import (
    "context"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/kazan/copilotcli"
)

func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    // Create client with functional options
    client, err := copilotcli.New(
        copilotcli.WithCLIURL("localhost:4321"),   // default
        copilotcli.WithModel("gpt-4o"),            // default
        copilotcli.WithStreaming(true),
        copilotcli.WithConnTimeout(15*time.Second),
        copilotcli.WithSystemMessage("You are an inventory management assistant."),
        copilotcli.WithTools(copilotcli.ToolDefinition{
            Name:        "check_stock",
            Description: "Check inventory stock level for a given SKU",
            Parameters: []copilotcli.ToolParameter{
                {Name: "sku", Type: "string", Description: "Product SKU", Required: true},
            },
            Handler: func(args map[string]any) (string, error) {
                sku := args["sku"].(string)
                return `{"sku": "` + sku + `", "quantity": 42, "available": true}`, nil
            },
        }),
    )
    if err != nil {
        log.Fatalf("creating copilot client: %v", err)
    }

    // Connect to sidecar (with retry + exponential backoff)
    if err := client.Start(ctx); err != nil {
        log.Fatalf("connecting to copilot sidecar: %v", err)
    }
    defer client.Stop()

    // Register HTTP handlers
    mux := http.NewServeMux()
    mux.HandleFunc("POST /api/copilot/query", copilotcli.NewQueryHandler(client))
    mux.HandleFunc("POST /api/copilot/stream", copilotcli.NewStreamHandler(client))
    mux.HandleFunc("GET /api/copilot/health", copilotcli.NewHealthHandler(client))

    // Start server...
}
```

### 2. Query via HTTP

```bash
# Simple query
curl -X POST http://localhost:8080/api/copilot/query \
  -H 'Content-Type: application/json' \
  -d '{"prompt": "What is the stock level for SKU ABC123?"}'

# Response:
# {"content": "The stock level for SKU ABC123 is 42 units, and it is currently available.", "session_id": "sess_abc123"}

# Multi-turn conversation
curl -X POST http://localhost:8080/api/copilot/query \
  -H 'Content-Type: application/json' \
  -d '{"prompt": "And what about SKU DEF456?", "session_id": "sess_abc123"}'

# Streaming (SSE)
curl -X POST http://localhost:8080/api/copilot/stream \
  -H 'Content-Type: application/json' \
  -d '{"prompt": "Summarize inventory status"}'
```

## Authentication

### Option A: GitHub Copilot Token (default)

Requires a GitHub Copilot subscription. Set `GITHUB_TOKEN` on the **sidecar container**:

```yaml
- name: copilot-sidecar
  env:
    - name: GITHUB_TOKEN
      valueFrom:
        secretKeyRef:
          name: copilot-secrets
          key: github-token
```

No special option needed on the client — GitHub auth is the default:

```go
client, err := copilotcli.New() // uses WithGitHubAuth() by default
```

### Option B: BYOK (Bring Your Own Key)

No GitHub auth needed. Configure the provider via options:

```go
client, err := copilotcli.New(
    copilotcli.WithBYOK(copilotcli.ProviderOpenAI, "https://api.openai.com/v1", "sk-xxx"),
    copilotcli.WithModel("gpt-4"),
)
```

For Azure OpenAI:

```go
client, err := copilotcli.New(
    copilotcli.WithBYOK(copilotcli.ProviderAzure, "https://my-resource.openai.azure.com", "azure-key"),
    copilotcli.WithAzureAPIVersion("2024-10-21"),
)
```

## Kubernetes Deployment

See [example/deployment.yaml](example/deployment.yaml) for a complete reference manifest.

Key points:

- Sidecar listens on `localhost:4321` (pod-internal, no Service/Ingress needed)
- App connects via `COPILOT_CLI_URL=localhost:4321`
- TCP readiness probe on port 4321 ensures sidecar is ready
- Network policy allows HTTPS egress to GitHub API / LLM providers
- Use `--no-auto-update` to prevent unexpected CLI updates in production

## Local Development

```bash
# Start the sidecar locally
docker compose -f example/docker-compose.copilot.yml up

# Or build and run manually
docker build -t copilot-cli-sidecar:latest -f example/Dockerfile.copilot-sidecar .
docker run -p 4321:4321 -e GITHUB_TOKEN=$GITHUB_TOKEN copilot-cli-sidecar:latest
```

## Custom Tools

Tools are functions that execute **in your Go service** (not in the sidecar). The LLM decides when to call them based on the tool description and the user's prompt.

```go
tools := []copilotcli.ToolDefinition{
    {
        Name:        "lookup_order",
        Description: "Look up order details by order ID",
        Parameters: []copilotcli.ToolParameter{
            {Name: "order_id", Type: "string", Description: "Order identifier", Required: true},
        },
        Handler: func(args map[string]any) (string, error) {
            orderID := args["order_id"].(string)
            order, err := orderService.Get(orderID)
            if err != nil {
                return "", err
            }
            data, _ := json.Marshal(order)
            return string(data), nil
        },
    },
}
```

For type-safe tools with automatic JSON schema generation:

```go
type StockParams struct {
    SKU       string `json:"sku" jsonschema:"Product SKU"`
    Warehouse string `json:"warehouse" jsonschema:"Warehouse code"`
}

stockTool := copilotcli.DefineTypedTool("check_stock", "Check stock in a specific warehouse",
    func(params StockParams, inv copilot.ToolInvocation) (any, error) {
        return inventoryService.CheckStock(params.SKU, params.Warehouse)
    },
)
```

## Limitations

| Limitation                   | Details                                                       |
| ---------------------------- | ------------------------------------------------------------- |
| **Technical Preview**        | SDK v0.1.x — API may change, not production-grade             |
| **No official CLI image**    | Must build your own sidecar Docker image                      |
| **Protocol version pinning** | SDK and CLI must match protocol version (currently v2)        |
| **Copilot subscription**     | Required for GitHub auth mode; BYOK avoids this               |
| **Billing**                  | Each prompt counts toward premium request quota (GitHub auth) |
| **CLI auto-updates**         | Must use `--no-auto-update` in production                     |
| **No Windows sidecar**       | Sidecar pattern requires Linux containers                     |

## Package Structure

```
copilotcli/
├── config.go      # Internal cfg struct, defaults, auth/provider types
├── options.go     # Functional options (WithCLIURL, WithModel, WithBYOK, etc.)
├── client.go      # Core client: New, Start, Stop, Query, QueryStream
├── tools.go       # Tool definitions and SDK conversion
├── handler.go     # HTTP handlers (query, stream SSE, health)
├── errors.go      # Sentinel errors
├── example/       # Kubernetes & Docker deployment examples
└── README.md      # This file
```
