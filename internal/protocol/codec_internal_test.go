package protocol

import (
	"errors"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

// White-box tests for checkLimits. These cover defensive guards that cannot
// be reached via Decode(), because json.Decoder rejects malformed top-level
// JSON before raw bytes ever reach checkLimits. Without these tests the
// pop()/push() guards would be unreachable from black-box paths.

func Test_checkLimits_With_TopLevelCloseBracket_Should_ReturnUnbalanced(t *testing.T) {
	t.Parallel()

	// Act — feed bytes directly to checkLimits, bypassing json.Decoder.
	// Each input must hit pop() at depth==0 (after at most matching push/pop
	// pairs) and trip the underflow guard. Mismatched-type closures like
	// `[}` are NOT in this list — pop() does not validate bracket type, so
	// those pass checkLimits and are caught later by json.Unmarshal.
	for _, input := range []string{`]`, `}`, `]]]]`, `}}}}`, `[]]`, `{}}`} {
		err := checkLimits([]byte(input))

		if err == nil {
			t.Fatalf("expected error for %q", input)
		}
		if !errors.Is(err, errUnbalanced) {
			t.Fatalf("input %q: expected errUnbalanced, got %v", input, err)
		}
	}
}

func Test_checkLimits_With_DepthAtMaxPlusOne_Should_ReturnDepthExceeded(t *testing.T) {
	t.Parallel()

	// Arrange — exactly MaxJSONDepth+1 opening brackets. The guard inside
	// push() must fire BEFORE writing the slice index, so even at the
	// boundary there is no out-of-bounds access.
	input := strings.Repeat("[", MaxJSONDepth+1)

	// Act
	err := checkLimits([]byte(input))

	// Assert
	if !errors.Is(err, ErrJSONDepthExceeded) {
		t.Fatalf("expected ErrJSONDepthExceeded, got %v", err)
	}
}

func Test_checkLimits_With_EmptyInput_Should_Succeed(t *testing.T) {
	t.Parallel()

	// Act
	err := checkLimits([]byte(""))

	// Assert
	assert.That(t, "error", err, nil)
}
