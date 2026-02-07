package copilotcli

import "errors"

var (
	// ErrNotConnected is returned when an operation requires an active connection to the sidecar.
	ErrNotConnected = errors.New("copilot client is not connected to the sidecar")

	// ErrAlreadyConnected is returned when Start is called on an already-connected client.
	ErrAlreadyConnected = errors.New("copilot client is already connected")

	// ErrEmptyPrompt is returned when an empty prompt is passed to Query.
	ErrEmptyPrompt = errors.New("prompt must not be empty")

	// ErrSidecarUnavailable is returned when the sidecar cannot be reached after retries.
	ErrSidecarUnavailable = errors.New("copilot CLI sidecar is unavailable after retries")

	// ErrMissingModel is returned when BYOK auth mode is used without specifying a model.
	ErrMissingModel = errors.New("model is required when using BYOK auth mode")

	// ErrMissingProviderBaseURL is returned when BYOK is used without a base URL.
	ErrMissingProviderBaseURL = errors.New("provider base URL is required when using BYOK auth mode")

	// ErrMissingCLIURL is returned when the CLI URL is empty after applying options.
	ErrMissingCLIURL = errors.New("CLI URL must not be empty")
)
