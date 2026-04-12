package schema

import (
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

// Test_hasOption_With_OmitemptyLookalike_Should_NotMatch guards the comma-split
// semantics: an option name that merely contains the substring "omitempty"
// (e.g., a future sibling option) must not be treated as omitempty. Prior to
// the fix, the code used strings.Contains on the raw options string, which
// would misclassify fields carrying such tags as optional.
func Test_hasOption_With_OmitemptyLookalike_Should_NotMatch(t *testing.T) {
	t.Parallel()

	// Arrange — simulate the options segment of a json struct tag.
	cases := []struct {
		opts string
		want bool
	}{
		{"", false},
		{"omitempty", true},
		{"string,omitempty", true},
		{"omitempty,string", true},
		{"someomitemptyish", false},
		{"prefixomitemptysuffix", false},
		{"omitemptyextra", false},
		{"foo,omitempty,bar", true},
	}

	for _, tc := range cases {
		// Act
		got := hasOption(tc.opts, "omitempty")

		// Assert
		assert.That(t, "opts="+tc.opts, got, tc.want)
	}
}
