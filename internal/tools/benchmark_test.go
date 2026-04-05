package tools_test

import (
	"context"
	"testing"

	"github.com/andygeiss/mcp/internal/tools"
)

type simpleBenchInput struct {
	Message string `json:"message" description:"A message"`
}

type complexBenchInput struct {
	Count   int     `json:"count" description:"Item count"`
	Enabled bool    `json:"enabled" description:"Feature flag"`
	Name    string  `json:"name" description:"The name"`
	Score   float64 `json:"score" description:"Score value"`
	Tag     string  `json:"tag" description:"A tag"`
	Verbose bool    `json:"verbose,omitempty" description:"Verbose output"`
}

func Benchmark_SchemaDerivation_With_SimpleStruct(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r := tools.NewRegistry()
		if err := tools.Register(r, "bench", "bench tool", func(_ context.Context, _ simpleBenchInput) tools.Result {
			return tools.TextResult("ok")
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_SchemaDerivation_With_ComplexStruct(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r := tools.NewRegistry()
		if err := tools.Register(r, "bench", "bench tool", func(_ context.Context, _ complexBenchInput) tools.Result {
			return tools.TextResult("ok")
		}); err != nil {
			b.Fatal(err)
		}
	}
}
