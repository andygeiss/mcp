package assert_test

import (
	"fmt"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
)

type fakeTB struct {
	testing.TB
	failed bool
	msg    string
}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.failed = true
	f.msg = fmt.Sprintf(format, args...)
}

func (f *fakeTB) Helper() {}

func Test_That_With_DifferentValues_Should_IncludeDescription(t *testing.T) {
	t.Parallel()

	// Arrange
	tb := &fakeTB{}

	// Act
	assert.That(tb, "value check", 1, 2)

	// Assert
	expected := "value check: got 1, want 2"
	if tb.msg != expected {
		t.Errorf("message: got %q, want %q", tb.msg, expected)
	}
}

func Test_That_With_DifferentValues_Should_ReportFailure(t *testing.T) {
	t.Parallel()

	// Arrange
	tb := &fakeTB{}

	// Act
	assert.That(tb, "mismatch", "got", "want")

	// Assert
	if !tb.failed {
		t.Error("expected failure")
	}
}

func Test_That_With_EqualValues_Should_NotFail(t *testing.T) {
	t.Parallel()

	// Arrange
	tb := &fakeTB{}

	// Act
	assert.That(tb, "strings", "hello", "hello")
	assert.That(tb, "ints", 42, 42)
	assert.That(tb, "bools", true, true)
	assert.That(tb, "slices", []byte{1, 2, 3}, []byte{1, 2, 3})

	// Assert
	if tb.failed {
		t.Errorf("expected no failure, got: %s", tb.msg)
	}
}
