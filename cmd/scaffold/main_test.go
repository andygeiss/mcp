package main

import (
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
