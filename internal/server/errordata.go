package server

import (
	"encoding/json"

	"github.com/andygeiss/mcp/internal/protocol"
)

// invalidParamData is the structured `error.data` payload returned alongside
// a -32602 invalid-params response when the server can name the offending
// field, the value the client sent, and the values it was willing to accept.
// Operators can read this without source diving.
type invalidParamData struct {
	Expected []string `json:"expected,omitempty"`
	Field    string   `json:"field"`
	Got      string   `json:"got,omitempty"`
}

// invalidParamError builds a CodeError with structured Data for an invalid
// parameter. expected may be nil when the valid set is unbounded (e.g. any
// string). The returned error is safe to pass to errorResponse.
func invalidParamError(message, field, got string, expected []string) *protocol.CodeError {
	ce := protocol.ErrInvalidParams(message)
	if data, err := json.Marshal(invalidParamData{
		Expected: expected,
		Field:    field,
		Got:      got,
	}); err == nil {
		ce.Data = data
	}
	return ce
}
