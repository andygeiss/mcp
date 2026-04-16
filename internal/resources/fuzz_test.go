package resources_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/resources"
)

// Fuzz_LookupTemplate_With_ArbitraryInputs asserts the RFC 6570 Level 1
// matching state machine never panics on any pattern/URI pair. Templates come
// from server authors, but URIs come from clients — malformed patterns plus
// adversarial URIs must terminate cleanly with a bool result.
//
//	go test -fuzz Fuzz_LookupTemplate_With_ArbitraryInputs ./internal/resources -fuzztime=${FUZZ_TIME:-30s}
func Fuzz_LookupTemplate_With_ArbitraryInputs(f *testing.F) {
	// Seed corpus — patterns and URIs exercising known edge cases.
	f.Add("file://{path}", "file://readme.md")
	f.Add("{scheme}{path}", "ab")
	f.Add("{a}{b}{c}", "xyz")
	f.Add("api/{v}/docs", "api//docs")
	f.Add("file://{path", "file://broken")
	f.Add("", "")
	f.Add("{x}", "")
	f.Add("a/{x}/b", "a//b")
	f.Add("a{x}{y}", "a")
	f.Add("{host}:8080", "localhost")
	f.Add("static://exact", "static://exact")
	f.Add("{{", "{")
	f.Add("}", "}")
	f.Add("{}", "x")

	f.Fuzz(func(_ *testing.T, pattern, uri string) {
		r := resources.NewRegistry()
		_ = resources.RegisterTemplate(r, pattern, "T", "fuzz",
			func(_ context.Context, u string) (resources.Result, error) {
				return resources.TextResult(u, ""), nil
			},
		)
		// Invariant: must not panic. Match outcome is not asserted — fuzzing
		// is about termination safety, not correctness of every pattern.
		_, _ = r.LookupTemplate(uri)
	})
}
