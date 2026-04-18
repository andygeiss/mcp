package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

func Test_ValidateModulePath_With_VariousInputs_Should_EnforceHostOwnerRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{name: "standard github path", input: "github.com/myorg/myrepo", wantError: false},
		{name: "custom host three segments", input: "example.com/a/b", wantError: false},
		{name: "four segments", input: "example.com/a/b/c", wantError: false},
		{name: "two segments non-github", input: "atruvia.de/sia-mcp", wantError: true},
		{name: "single segment", input: "single", wantError: true},
		{name: "empty string", input: "", wantError: true},
		{name: "trailing empty segment", input: "github.com/myorg/", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Act
			err := validateModulePath(tt.input)

			// Assert
			assert.That(t, "error presence", err != nil, tt.wantError)
			if tt.wantError && err != nil {
				assert.That(t, "error mentions canonical form", strings.Contains(err.Error(), "host/owner/repo"), true)
			}
		})
	}
}

func Test_EmitWelcome_Should_WriteExactBanner(t *testing.T) {
	t.Parallel()

	// Arrange
	var stderr bytes.Buffer

	// Act
	emitWelcome(&stderr)

	// Assert — golden-string match catches accidental wording drift.
	expected := "Your MCP server is running.\n\n  Edit:   internal/tools/echo.go\n  Wire:   cmd/mcp/main.go\n  Verify: make smoke\n\nFull guide: README.md\n"
	if stderr.String() != expected {
		t.Fatalf("welcome banner mismatch.\n--- got ---\n%s\n--- want ---\n%s", stderr.String(), expected)
	}
}

func Test_WelcomeBanner_Should_ContainExpectedSteps(t *testing.T) {
	t.Parallel()

	// Arrange / Act / Assert — banner content ships exactly the three
	// imperative steps + README pointer (FR5c).
	required := []string{
		"Your MCP server is running.",
		"Edit:",
		"internal/tools/echo.go",
		"Wire:",
		"cmd/mcp/main.go",
		"Verify:",
		"make smoke",
		"Full guide:",
		"README.md",
	}
	for _, want := range required {
		if !strings.Contains(welcomeBanner, want) {
			t.Errorf("welcome banner missing %q", want)
		}
	}
}
