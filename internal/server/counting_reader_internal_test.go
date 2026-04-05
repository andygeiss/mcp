package server

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/andygeiss/mcp/internal/pkg/assert"
)

func Test_countingReader_With_ExactLimit_Should_Accept(t *testing.T) {
	t.Parallel()

	// Arrange
	data := make([]byte, maxMessageSize)
	cr := newCountingReader(bytes.NewReader(data), maxMessageSize)

	// Act
	got, err := io.ReadAll(cr)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "bytes read", int64(len(got)), maxMessageSize)
	assert.That(t, "exceeded", cr.Exceeded(), false)
}

func Test_countingReader_With_OneOverLimit_Should_ReturnErrMessageTooLarge(t *testing.T) {
	t.Parallel()

	// Arrange
	data := make([]byte, maxMessageSize+1)
	cr := newCountingReader(bytes.NewReader(data), maxMessageSize)

	// Act
	_, err := io.ReadAll(cr)

	// Assert
	assert.That(t, "is errMessageTooLarge", errors.Is(err, errMessageTooLarge), true)
	assert.That(t, "exceeded", cr.Exceeded(), true)
}

func Test_countingReader_With_OneUnderLimit_Should_Accept(t *testing.T) {
	t.Parallel()

	// Arrange
	data := make([]byte, maxMessageSize-1)
	cr := newCountingReader(bytes.NewReader(data), maxMessageSize)

	// Act
	got, err := io.ReadAll(cr)

	// Assert
	assert.That(t, "error", err, nil)
	assert.That(t, "bytes read", int64(len(got)), maxMessageSize-1)
	assert.That(t, "exceeded", cr.Exceeded(), false)
}

func Test_countingReader_With_ResetBetweenMessages_Should_RestartCounter(t *testing.T) {
	t.Parallel()

	// Arrange — two 10-byte chunks with limit 10
	data := make([]byte, 20)
	cr := newCountingReader(bytes.NewReader(data), 10)

	// Act — read first chunk
	buf := make([]byte, 10)
	n1, err1 := io.ReadFull(cr, buf)

	// Reset between messages
	cr.Reset()

	// Read second chunk
	n2, err2 := io.ReadFull(cr, buf)

	// Assert
	assert.That(t, "first read bytes", n1, 10)
	assert.That(t, "first read error", err1, nil)
	assert.That(t, "second read bytes", n2, 10)
	assert.That(t, "second read error", err2, nil)
	assert.That(t, "exceeded", cr.Exceeded(), false)
}

func Test_countingReader_With_PreExceededState_Should_ReturnImmediately(t *testing.T) {
	t.Parallel()

	// Arrange — use a reader that panics on Read to prove it's never consulted
	pr := &panicReader{}
	cr := newCountingReader(pr, 5)
	cr.count = 6 // simulate pre-exceeded state

	// Act
	buf := make([]byte, 10)
	n, err := cr.Read(buf)

	// Assert
	assert.That(t, "bytes read", n, 0)
	assert.That(t, "is errMessageTooLarge", errors.Is(err, errMessageTooLarge), true)
}

// panicReader panics if Read is called, proving the underlying reader is not consulted.
type panicReader struct{}

func (pr *panicReader) Read(_ []byte) (int, error) {
	panic("underlying reader should not be called")
}
