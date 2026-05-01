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

// Test_checkLimits_With_AlternatingNesting_Should_CountKeysPerScope verifies
// that parentIsObject[depth] flips correctly across alternating object/array
// shapes and that bumpKey fires at the depth of the containing object — not
// across the whole payload. If keyCount aggregated across object scopes, an
// outer object with 1 key plus an inner object at MaxJSONKeysPerObject keys
// would falsely trip the limit (1 + 10_000 > 10_000); tracked correctly,
// each object scope is counted independently and the payload passes.
func Test_checkLimits_With_AlternatingNesting_Should_CountKeysPerScope(t *testing.T) {
	t.Parallel()

	// Outer object: 1 key. Array element: an object with exactly
	// MaxJSONKeysPerObject keys (at the limit, must not trip).
	innerAtLimit := buildKeyedObject(MaxJSONKeysPerObject)
	innerOverLimit := buildKeyedObject(MaxJSONKeysPerObject + 1)

	cases := []struct {
		name      string
		input     string
		wantLimit string // empty for success
	}{
		{
			name:  "deeply alternating, all valid",
			input: `{"a":[{"b":[{"c":[true,false,null,1]}]}]}`,
		},
		{
			name:  "outer object plus array of object at exactly limit",
			input: `{"a":[` + innerAtLimit + `]}`,
		},
		{
			name:      "outer object plus array of object over limit",
			input:     `{"a":[` + innerOverLimit + `]}`,
			wantLimit: "maxKeysPerObject",
		},
		{
			name:      "deeply alternating with oversized inner string",
			input:     `{"a":[{"b":[{"c":"` + repeatA(MaxJSONStringLen+1) + `"}]}]}`,
			wantLimit: "maxStringLength",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := checkLimits([]byte(tc.input))
			if tc.wantLimit == "" {
				assert.That(t, "error", err, nil)
				return
			}
			var sle *StructuralLimitError
			if !errors.As(err, &sle) {
				t.Fatalf("expected *StructuralLimitError, got %v", err)
			}
			assert.That(t, "limit", sle.Limit, tc.wantLimit)
		})
	}
}

// buildKeyedObject returns a JSON object literal with n distinct integer-keyed
// entries. Used by alternating-nesting tests to construct payloads at and
// over MaxJSONKeysPerObject without inflating test code.
func buildKeyedObject(n int) string {
	var sb strings.Builder
	sb.WriteByte('{')
	for i := range n {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`"k`)
		sb.WriteString(itoa(i))
		sb.WriteString(`":1`)
	}
	sb.WriteByte('}')
	return sb.String()
}

// repeatA returns a string of n 'a' bytes, used to construct over-limit
// JSON string literals without bringing strings.Repeat into the import set
// of this test file (already imported indirectly via strings.Builder).
func repeatA(n int) string {
	return strings.Repeat("a", n)
}

// itoa converts a non-negative int to its decimal string. Inlined to avoid
// pulling strconv into this internal test file's imports.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
