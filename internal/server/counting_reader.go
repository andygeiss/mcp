package server

import (
	"errors"
	"io"
)

// maxMessageSize is the per-message safety limit (16 MiB). Sized to preserve
// a 1:4 ratio with MaxJSONStringLen (4 MiB) so a single max-size string plus
// envelope, metadata, and a second medium field fits under the cap. The
// countingReader is reset before each dec.Decode call, but the json.Decoder
// reads ahead into an internal buffer (typically 4–64 KB in the stdlib). The
// counting reader sees these buffered reads, not individual message
// boundaries, so the effective enforcement point is between 16 MiB and
// 16 MiB + one buffer fill. This imprecision is acceptable for a safety
// limit whose goal is preventing abuse, not byte-exact enforcement.
// See docs/adr/ADR-004-decode-limits.md.
const (
	maxMessageSize = 16 * 1024 * 1024 // 16 MiB
	maxResultSize  = 1 * 1024 * 1024  // 1 MiB
)

var errMessageTooLarge = errors.New("message exceeds 16MB size limit")

// countingReader wraps an io.Reader and tracks bytes read since the last reset.
type countingReader struct {
	count  int64
	limit  int64
	reader io.Reader
}

// newCountingReader creates a countingReader with the given limit.
func newCountingReader(r io.Reader, limit int64) *countingReader {
	return &countingReader{
		limit:  limit,
		reader: r,
	}
}

// Exceeded reports whether the byte counter has exceeded the limit.
func (cr *countingReader) Exceeded() bool {
	return cr.count > cr.limit
}

// Read implements io.Reader, tracking total bytes and enforcing the limit.
// When the limit is exceeded after a read, the bytes already consumed are
// reported along with the error, per the io.Reader contract.
func (cr *countingReader) Read(p []byte) (int, error) {
	if cr.count > cr.limit {
		return 0, errMessageTooLarge
	}
	n, err := cr.reader.Read(p)
	cr.count += int64(n)
	if cr.count > cr.limit {
		return n, errMessageTooLarge
	}
	return n, err
}

// Reset resets the byte counter for the next message.
func (cr *countingReader) Reset() {
	cr.count = 0
}
