package prompts_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/prompts"
)

type greetInput struct {
	Name string `json:"name" description:"Name to greet"`
}

func TestRegister_With_ValidPrompt_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Arrange
	r := prompts.NewRegistry()

	// Act
	err := prompts.Register(r, "greet", "A greeting prompt",
		func(_ context.Context, input greetInput) prompts.Result {
			return prompts.Result{
				Messages: []prompts.Message{
					prompts.UserMessage("Hello " + input.Name),
				},
			}
		},
	)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "prompt count", len(r.Prompts()), 1)
	assert.That(t, "prompt name", r.Prompts()[0].Name, "greet")
	assert.That(t, "argument count", len(r.Prompts()[0].Arguments), 1)
	assert.That(t, "argument name", r.Prompts()[0].Arguments[0].Name, "name")
	assert.That(t, "argument required", r.Prompts()[0].Arguments[0].Required, true)
	assert.That(t, "argument desc", r.Prompts()[0].Arguments[0].Description, "Name to greet")
}

func TestRegister_With_DuplicateName_Should_ReturnError(t *testing.T) {
	t.Parallel()

	// Arrange
	r := prompts.NewRegistry()
	handler := func(_ context.Context, _ greetInput) prompts.Result {
		return prompts.Result{}
	}
	_ = prompts.Register(r, "greet", "desc", handler)

	// Act
	err := prompts.Register(r, "greet", "desc2", handler)

	// Assert
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestLookup_With_RegisteredName_Should_ReturnPrompt(t *testing.T) {
	t.Parallel()

	// Arrange
	r := prompts.NewRegistry()
	_ = prompts.Register(r, "greet", "desc",
		func(_ context.Context, _ greetInput) prompts.Result {
			return prompts.Result{}
		},
	)

	// Act
	p, ok := r.Lookup("greet")

	// Assert
	assert.That(t, "found", ok, true)
	assert.That(t, "name", p.Name, "greet")
}

func TestLookup_With_UnknownName_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange
	r := prompts.NewRegistry()

	// Act
	_, ok := r.Lookup("unknown")

	// Assert
	assert.That(t, "found", ok, false)
}

func TestPrompts_Should_ReturnSortedByName(t *testing.T) {
	t.Parallel()

	// Arrange
	r := prompts.NewRegistry()
	handler := func(_ context.Context, _ greetInput) prompts.Result {
		return prompts.Result{}
	}
	_ = prompts.Register(r, "zeta", "desc", handler)
	_ = prompts.Register(r, "alpha", "desc", handler)

	// Act
	all := r.Prompts()

	// Assert
	assert.That(t, "count", len(all), 2)
	assert.That(t, "first", all[0].Name, "alpha")
	assert.That(t, "second", all[1].Name, "zeta")
}

func TestUserMessage_Should_ReturnCorrectContent(t *testing.T) {
	t.Parallel()

	// Act
	msg := prompts.UserMessage("hello")

	// Assert
	assert.That(t, "role", msg.Role, "user")
	assert.That(t, "text", msg.Content.Text, "hello")
	assert.That(t, "type", msg.Content.Type, "text")
}

func TestRegister_With_Handler_Should_ExecuteCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	r := prompts.NewRegistry()
	_ = prompts.Register(r, "greet", "A greeting",
		func(_ context.Context, input greetInput) prompts.Result {
			return prompts.Result{
				Description: "Greeting for " + input.Name,
				Messages:    []prompts.Message{prompts.UserMessage("Hello " + input.Name)},
			}
		},
	)

	// Act
	p, _ := r.Lookup("greet")
	result, err := p.Handler(context.Background(), map[string]string{"name": "Andy"})

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "description", result.Description, "Greeting for Andy")
	assert.That(t, "message count", len(result.Messages), 1)
	assert.That(t, "message text", result.Messages[0].Content.Text, "Hello Andy")
}

func TestAssistantMessage_Should_ReturnCorrectContent(t *testing.T) {
	t.Parallel()

	// Act
	msg := prompts.AssistantMessage("hi there")

	// Assert
	assert.That(t, "role", msg.Role, "assistant")
	assert.That(t, "text", msg.Content.Text, "hi there")
}
