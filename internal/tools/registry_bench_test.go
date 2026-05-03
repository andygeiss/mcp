package tools_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/tools"
)

type listBenchInput struct {
	Message string `json:"message" description:"the message to echo"`
}

type listBenchOutput struct {
	Echoed string `json:"echoed" description:"the echoed message"`
}

func newListBenchRegistry(b *testing.B, n int) *tools.Registry {
	b.Helper()
	r := tools.NewRegistry()
	for i := range n {
		name := []byte("tool-")
		// Avoid fmt.Sprintf in the hot path; uppercase index char is fine
		// for benchmark identifiers (a, b, c, ...) and stays alloc-cheap.
		name = append(name, byte('a'+(i%26)), byte('a'+((i/26)%26)))
		if err := tools.Register[listBenchInput, listBenchOutput](r, string(name), "fixture",
			func(_ context.Context, in listBenchInput) (listBenchOutput, tools.Result) {
				return listBenchOutput{Echoed: in.Message}, tools.Result{}
			},
		); err != nil {
			b.Fatal(err)
		}
	}
	return r
}

// Benchmark_ToolsList_AllocsPerOp_With_OneTool is the headline number for
// procurement-readable perf claims: how cheap is the inventory accessor on
// a one-tool registry (the production wiring as of 2026-05-03).
func Benchmark_ToolsList_AllocsPerOp_With_OneTool(b *testing.B) {
	r := newListBenchRegistry(b, 1)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.Tools()
	}
}

// Benchmark_ToolsList_AllocsPerOp_With_TenTools projects the same metric to
// a small ensemble — relevant once downstream servers register multiple
// tools in production.
func Benchmark_ToolsList_AllocsPerOp_With_TenTools(b *testing.B) {
	r := newListBenchRegistry(b, 10)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.Tools()
	}
}

// Benchmark_ToolsList_AllocsPerOp_With_FiftyTools stresses the upper end of
// what we expect to see in any single MCP server. A linear regression
// across the three sizes feeds future S3 / schema-cache evaluations.
func Benchmark_ToolsList_AllocsPerOp_With_FiftyTools(b *testing.B) {
	r := newListBenchRegistry(b, 50)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.Tools()
	}
}
