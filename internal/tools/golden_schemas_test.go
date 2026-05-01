package tools_test

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/tools"
)

// updateGoldens, when set via -update, rewrites every golden file in
// testdata/golden_schemas/ to match the live derived schema. Reviewers should
// inspect the diff before committing — that conscious diff is the entire
// point of the freeze (Q60). Never run -update on CI.
var updateGoldens = flag.Bool("update", false, "rewrite golden schema files to match live tool schemas")

// goldenSchemaCase pins each registered tool's input AND output schema to
// checked-in JSON files. Adding a tool to this table requires checking in
// goldens; changing a tool's input or output struct requires running -update
// and reviewing both diffs. The freeze enforces conscious schema evolution
// per Q60 — extended in Story 2.2 to cover the new outputSchema surface.
type goldenSchemaCase struct {
	goldenInput  string // file path under testdata/golden_schemas/ for InputSchema
	goldenOutput string // file path under testdata/golden_schemas/ for OutputSchema
	name         string // tool name registered with Register
	register     func(r *tools.Registry) error
}

// nestedOutputInput / nestedOutput exercise a struct-within-struct OutputSchema.
type nestedOutputInput struct {
	Query string `json:"query" description:"Search query"`
}

type nestedOutputAddress struct {
	City string `json:"city" description:"City name"`
	Zip  string `json:"zip" description:"Postal code"`
}

type nestedOutput struct {
	Address nestedOutputAddress `json:"address" description:"Mailing address"`
	Name    string              `json:"name" description:"Person name"`
}

func nestedOutputHandler(_ context.Context, _ nestedOutputInput) (nestedOutput, tools.Result) {
	return nestedOutput{}, tools.TextResult("ok")
}

// optionalOutputInput / optionalOutput exercise pointer-type optional fields
// in the OutputSchema (per the project rule: pointer = optional).
type optionalOutputInput struct {
	ID string `json:"id" description:"Record id"`
}

type optionalOutput struct {
	Comment *string `json:"comment,omitempty" description:"Optional comment"`
	Score   *int    `json:"score,omitempty" description:"Optional score 0-100"`
	Title   string  `json:"title" description:"Required title"`
}

func optionalOutputHandler(_ context.Context, _ optionalOutputInput) (optionalOutput, tools.Result) {
	return optionalOutput{}, tools.TextResult("ok")
}

// arrayOutputInput / arrayOutput exercise array-of-struct in OutputSchema.
type arrayOutputInput struct {
	Limit int `json:"limit" description:"Maximum results"`
}

type arrayOutputItem struct {
	Label string `json:"label" description:"Item label"`
	Value int    `json:"value" description:"Item value"`
}

type arrayOutput struct {
	Items []arrayOutputItem `json:"items" description:"Result items"`
}

func arrayOutputHandler(_ context.Context, _ arrayOutputInput) (arrayOutput, tools.Result) {
	return arrayOutput{}, tools.TextResult("ok")
}

func goldenSchemaCases() []goldenSchemaCase {
	return []goldenSchemaCase{
		{
			goldenInput:  "echo.input.json",
			goldenOutput: "echo.output.json",
			name:         "echo",
			register: func(r *tools.Registry) error {
				return tools.Register[tools.EchoInput, tools.EchoOutput](r, "echo", "echo back the supplied message", tools.Echo)
			},
		},
		{
			goldenInput:  "nested-output.input.json",
			goldenOutput: "nested-output.output.json",
			name:         "nested-output",
			register: func(r *tools.Registry) error {
				return tools.Register[nestedOutputInput, nestedOutput](r, "nested-output", "nested struct in output", nestedOutputHandler)
			},
		},
		{
			goldenInput:  "optional-output.input.json",
			goldenOutput: "optional-output.output.json",
			name:         "optional-output",
			register: func(r *tools.Registry) error {
				return tools.Register[optionalOutputInput, optionalOutput](r, "optional-output", "pointer-typed optional fields in output", optionalOutputHandler)
			},
		},
		{
			goldenInput:  "array-output.input.json",
			goldenOutput: "array-output.output.json",
			name:         "array-output",
			register: func(r *tools.Registry) error {
				return tools.Register[arrayOutputInput, arrayOutput](r, "array-output", "array-of-struct in output", arrayOutputHandler)
			},
		},
	}
}

func Test_GoldenSchema_Should_MatchPinnedBytes(t *testing.T) {
	t.Parallel()

	for _, tc := range goldenSchemaCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Arrange — build a fresh registry containing only this tool, so
			// the goldens capture its schemas in isolation.
			r := tools.NewRegistry()
			if err := tc.register(r); err != nil {
				t.Fatalf("register: %v", err)
			}
			tool, ok := r.Lookup(tc.name)
			if !ok {
				t.Fatalf("tool %q not in registry after Register", tc.name)
			}

			// Act — marshal both schemas exactly as the wire would see them on
			// tools/list. Indented for human-readable goldens.
			gotInput, err := json.MarshalIndent(tool.InputSchema, "", "  ")
			if err != nil {
				t.Fatalf("marshal input schema: %v", err)
			}
			if tool.OutputSchema == nil {
				t.Fatalf("tool %q has nil OutputSchema after Register; expected reflection-derived schema", tc.name)
			}
			gotOutput, err := json.MarshalIndent(tool.OutputSchema, "", "  ")
			if err != nil {
				t.Fatalf("marshal output schema: %v", err)
			}

			compareGolden(t, tc.name, "input", tc.goldenInput, gotInput)
			compareGolden(t, tc.name, "output", tc.goldenOutput, gotOutput)
		})
	}
}

func compareGolden(t *testing.T, toolName, side, file string, got []byte) {
	t.Helper()
	// Append a final newline so golden files match POSIX text-file convention
	// and don't trip editor auto-newline-on-save into spurious diffs. The
	// renderer itself does not emit one (json.MarshalIndent never appends
	// \n) — the goldens carry it explicitly.
	if len(got) == 0 || got[len(got)-1] != '\n' {
		got = append(got, '\n')
	}

	path := filepath.Join("testdata", "golden_schemas", file)
	if *updateGoldens {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		t.Logf("updated golden %s", path)
		return
	}
	//nolint:gosec // path is constructed from a hard-coded test table.
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -args -update to create it)", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s schema for %q drifted from golden %s\n--- want ---\n%s\n--- got ---\n%s",
			side, toolName, path, string(want), string(got))
	}
}

// silence the test-only context import; used by registered handlers above.
var _ = context.Background
