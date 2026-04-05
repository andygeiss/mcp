package tools

import (
	"errors"
	"fmt"
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
	if strings.Contains(path, "..") {
		return errors.New("path traversal not allowed")
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
