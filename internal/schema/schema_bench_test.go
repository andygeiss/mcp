package schema_test

import (
	"testing"
	"time"

	"github.com/andygeiss/mcp/internal/schema"
)

type benchSmallInput struct {
	Message string `json:"message" description:"a short message"`
}

type benchLargeInput struct {
	Field01 string  `json:"f01" description:"d"`
	Field02 string  `json:"f02" description:"d"`
	Field03 string  `json:"f03" description:"d"`
	Field04 string  `json:"f04" description:"d"`
	Field05 string  `json:"f05" description:"d"`
	Field06 string  `json:"f06" description:"d"`
	Field07 string  `json:"f07" description:"d"`
	Field08 string  `json:"f08" description:"d"`
	Field09 string  `json:"f09" description:"d"`
	Field10 string  `json:"f10" description:"d"`
	Field11 int     `json:"f11" description:"d"`
	Field12 int     `json:"f12" description:"d"`
	Field13 int     `json:"f13" description:"d"`
	Field14 float64 `json:"f14" description:"d"`
	Field15 float64 `json:"f15" description:"d"`
	Field16 bool    `json:"f16" description:"d"`
	Field17 bool    `json:"f17" description:"d"`
	Field18 *int    `json:"f18,omitempty" description:"d"`
	Field19 *string `json:"f19,omitempty" description:"d"`
	Field20 *bool   `json:"f20,omitempty" description:"d"`
}

type benchNestedDepth1 struct {
	Inner benchNestedDepth2 `json:"inner" description:"d"`
}
type benchNestedDepth2 struct {
	Inner benchNestedDepth3 `json:"inner" description:"d"`
}
type benchNestedDepth3 struct {
	Inner benchNestedDepth4 `json:"inner" description:"d"`
}
type benchNestedDepth4 struct {
	Leaf string `json:"leaf" description:"d"`
}

type benchSpecialFields struct {
	When    time.Time         `json:"when" description:"timestamp"`
	Payload []byte            `json:"payload,omitempty" description:"raw payload"`
	Items   []benchSmallInput `json:"items,omitempty" description:"list of inputs"`
}

func Benchmark_DeriveInputSchema_With_SmallStruct(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := schema.DeriveInputSchema[benchSmallInput]()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_DeriveInputSchema_With_LargeStruct(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := schema.DeriveInputSchema[benchLargeInput]()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_DeriveInputSchema_With_DeeplyNestedStruct(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := schema.DeriveInputSchema[benchNestedDepth1]()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_DeriveInputSchema_With_SpecialFields(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := schema.DeriveInputSchema[benchSpecialFields]()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func Benchmark_DeriveOutputSchema_With_SmallStruct(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, err := schema.DeriveOutputSchema[benchSmallInput]()
		if err != nil {
			b.Fatal(err)
		}
	}
}
