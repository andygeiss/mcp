package tools

import "context"

// EchoInput defines the parameters for the echo tool.
type EchoInput struct {
	Message string `json:"message" description:"The message to echo back"`
}

// Echo returns the input message as a text result.
func Echo(_ context.Context, input EchoInput) Result {
	return TextResult(input.Message)
}
