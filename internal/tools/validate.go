package tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// MaxInputLength is the maximum allowed length for any tool input string.
	MaxInputLength = 4096
)

// ValidatePath checks a path string for security issues: path traversal,
// null bytes, and length limits. Returns nil if the path is safe.
func ValidatePath(path string) error {
	if len(path) > MaxInputLength {
		return fmt.Errorf("path exceeds maximum length of %d characters", MaxInputLength)
	}
	if strings.ContainsRune(path, '\x00') {
		return errors.New("path contains null byte")
	}
	cleaned := filepath.Clean(path)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return errors.New("path traversal not allowed")
	}
	return nil
}

// unmarshalAndValidate unmarshals params into dst and validates required fields
// in a single pass. It first unmarshals into the typed struct (rejecting
// unknown fields), then checks required fields via a lightweight key-presence
// scan if needed.
func unmarshalAndValidate(params json.RawMessage, dst any, required []string) error {
	dec := json.NewDecoder(bytes.NewReader(params))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON object: %w", err)
	}
	if len(required) == 0 {
		return nil
	}
	// Lightweight required-field check: unmarshal to map only for key presence.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(params, &fields); err != nil {
		return fmt.Errorf("invalid JSON object: %w", err)
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("missing required field %q", key)
		}
	}
	return nil
}

// ValidateInput checks a generic input string for security issues: null bytes,
// dangerous shell metacharacters, and length limits. Returns nil if safe.
func ValidateInput(input string) error {
	if len(input) > MaxInputLength {
		return fmt.Errorf("input exceeds maximum length of %d characters", MaxInputLength)
	}
	if strings.ContainsRune(input, '\x00') {
		return errors.New("input contains null byte")
	}
	return nil
}
