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

// goldenSchemaCases pins each registered tool's input schema to a checked-in
// JSON file. Adding a tool to this table requires checking in a corresponding
// golden file; changing a tool's input struct requires running `-update` and
// reviewing the diff. The freeze enforces conscious schema evolution per Q60.
type goldenSchemaCase struct {
	golden   string // file path under testdata/golden_schemas/
	name     string // tool name registered with Register
	register func(r *tools.Registry) error
}

func goldenSchemaCases() []goldenSchemaCase {
	return []goldenSchemaCase{
		{
			golden: "echo.input.json",
			name:   "echo",
			register: func(r *tools.Registry) error {
				return tools.Register(r, "echo", "echo back the supplied message", tools.Echo)
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
			// the golden captures its schema in isolation.
			r := tools.NewRegistry()
			if err := tc.register(r); err != nil {
				t.Fatalf("register: %v", err)
			}
			tool, ok := r.Lookup(tc.name)
			if !ok {
				t.Fatalf("tool %q not in registry after Register", tc.name)
			}

			// Act — marshal the InputSchema field exactly as the wire would
			// see it on tools/list. Indented for human-readable goldens.
			got, err := json.MarshalIndent(tool.InputSchema, "", "  ")
			if err != nil {
				t.Fatalf("marshal schema: %v", err)
			}

			// Assert (or update on -update flag).
			path := filepath.Join("testdata", "golden_schemas", tc.golden)
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
				t.Fatalf("schema for %q drifted from golden %s\n--- want ---\n%s\n--- got ---\n%s",
					tc.name, path, string(want), string(got))
			}
		})
	}
}

// silence the test-only context import; used by registered handlers above.
var _ = context.Background
