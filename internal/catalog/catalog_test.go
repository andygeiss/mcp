package catalog_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/catalog"
	"github.com/andygeiss/mcp/internal/inspect"
	"github.com/andygeiss/mcp/internal/schema"
	"github.com/andygeiss/mcp/internal/tools"
)

func sampleOutput() inspect.Output {
	return inspect.Output{
		Server:          inspect.ServerInfo{Name: "mcp", Version: "test"},
		ProtocolVersion: "2025-11-25",
		Tools: []tools.Tool{
			{
				Name:        "echo",
				Description: "Echoes the input message",
				InputSchema: schema.InputSchema{
					Type:     "object",
					Required: []string{"message"},
					Properties: map[string]schema.Property{
						"message": {Type: "string", Description: "The message to echo back"},
					},
				},
				OutputSchema: &schema.OutputSchema{
					Type:     "object",
					Required: []string{"echoed"},
					Properties: map[string]schema.Property{
						"echoed": {Type: "string", Description: "The message that was echoed back"},
					},
				},
			},
			{
				Name:        "alpha",
				Description: "Demonstration tool with optional input and array output",
				InputSchema: schema.InputSchema{
					Type: "object",
					Properties: map[string]schema.Property{
						"limit": {Type: "integer", Description: "max items"},
					},
				},
				OutputSchema: &schema.OutputSchema{
					Type: "object",
					Properties: map[string]schema.Property{
						"items": {
							Type:  "array",
							Items: &schema.Property{Type: "string"},
						},
					},
				},
			},
		},
	}
}

func Test_Render_With_FixtureOutput_Should_BeIdempotent(t *testing.T) {
	t.Parallel()

	// Arrange
	src := sampleOutput()

	// Act — render twice
	var first, second bytes.Buffer
	assert.That(t, "first render", catalog.Render(src, &first), nil)
	assert.That(t, "second render", catalog.Render(src, &second), nil)

	// Assert — byte-for-byte identical
	assert.That(t, "idempotent", first.String(), second.String())
}

func Test_Render_With_FixtureOutput_Should_SortToolsAlphabetically(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	assert.That(t, "render", catalog.Render(sampleOutput(), &buf), nil)
	out := buf.String()

	// "alpha" should appear before "echo" in the rendered output even though
	// the fixture lists echo first.
	alphaIdx := strings.Index(out, "`alpha`")
	echoIdx := strings.Index(out, "`echo`")
	if alphaIdx < 0 || echoIdx < 0 {
		t.Fatalf("expected both alpha and echo names in output:\n%s", out)
	}
	assert.That(t, "alpha precedes echo", alphaIdx < echoIdx, true)
}

func Test_Render_With_PipeInDescription_Should_EscapeMarkdownPipe(t *testing.T) {
	t.Parallel()

	src := inspect.Output{
		Tools: []tools.Tool{{
			Name:        "tricky",
			Description: "uses the | pipe operator",
			InputSchema: schema.InputSchema{Type: "object"},
		}},
	}

	var buf bytes.Buffer
	assert.That(t, "render", catalog.Render(src, &buf), nil)

	// The literal '|' in the description must be escaped as '\|' so it does
	// not break Markdown table parsing in renderers.
	assert.That(t, "pipe escaped", strings.Contains(buf.String(), `\|`), true)
}

func Test_Render_With_NoTools_Should_StillEmitFrame(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	assert.That(t, "render", catalog.Render(inspect.Output{}, &buf), nil)
	out := buf.String()

	assert.That(t, "header present", strings.Contains(out, "# Tools Catalog"), true)
	assert.That(t, "do-not-edit notice present", strings.Contains(out, "Do not edit by hand"), true)
}

func Test_Render_With_ArrayOutput_Should_RenderArrayOfElement(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	assert.That(t, "render", catalog.Render(sampleOutput(), &buf), nil)
	out := buf.String()

	// The "alpha" tool has output `items: array<string>` — render must
	// surface that nesting.
	assert.That(t, "renders array<string>", strings.Contains(out, "array<string>"), true)
}
