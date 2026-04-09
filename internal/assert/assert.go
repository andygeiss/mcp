// Package assert provides lightweight test assertion helpers.
package assert

import (
	"reflect"
	"testing"
)

// That marks the test as failed when got and want are not deeply equal.
func That[T any](tb testing.TB, desc string, got, want T) {
	tb.Helper()
	if !reflect.DeepEqual(got, want) {
		tb.Errorf("%s: got %v, want %v", desc, got, want)
	}
}
