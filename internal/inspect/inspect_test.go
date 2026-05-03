package inspect_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/inspect"
	"github.com/andygeiss/mcp/internal/prompts"
	"github.com/andygeiss/mcp/internal/protocol"
	"github.com/andygeiss/mcp/internal/resources"
	"github.com/andygeiss/mcp/internal/tools"
)

type sampleIn struct {
	Greeting string `json:"greeting" description:"the greeting to echo"`
}

type sampleOut struct {
	Echoed string `json:"echoed" description:"the echoed greeting"`
}

type samplePromptIn struct {
	Topic string `json:"topic" description:"topic for the prompt"`
}

func newFixtureRegistries(t *testing.T) (*tools.Registry, *resources.Registry, *prompts.Registry) {
	t.Helper()

	tr := tools.NewRegistry()
	if err := tools.Register[sampleIn, sampleOut](tr, "sample", "echoes greeting",
		func(_ context.Context, in sampleIn) (sampleOut, tools.Result) {
			return sampleOut{Echoed: in.Greeting}, tools.Result{}
		},
	); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	rr := resources.NewRegistry()
	if err := resources.Register(rr, "file:///fixture", "fixture", "static fixture",
		func(_ context.Context, _ string) (resources.Result, error) {
			return resources.Result{}, nil
		},
	); err != nil {
		t.Fatalf("register resource: %v", err)
	}

	pr := prompts.NewRegistry()
	if err := prompts.Register[samplePromptIn](pr, "sample-prompt", "fixture prompt",
		func(_ context.Context, _ samplePromptIn) prompts.Result {
			return prompts.Result{}
		},
	); err != nil {
		t.Fatalf("register prompt: %v", err)
	}
	return tr, rr, pr
}

func Test_Inspect_With_PopulatedRegistries_Should_EmitDeterministicShape(t *testing.T) {
	t.Parallel()

	// Arrange
	tr, rr, pr := newFixtureRegistries(t)

	// Act — render twice to confirm idempotency.
	var first, second bytes.Buffer
	assert.That(t, "first render error", inspect.Inspect("mcp", "test", tr, rr, pr, &first), nil)
	assert.That(t, "second render error", inspect.Inspect("mcp", "test", tr, rr, pr, &second), nil)

	// Assert — byte-for-byte identical across runs.
	assert.That(t, "deterministic across runs", first.String(), second.String())

	// Assert — output is valid JSON with the documented shape.
	var got inspect.Output
	if err := json.Unmarshal(first.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	assert.That(t, "server name", got.Server.Name, "mcp")
	assert.That(t, "server version", got.Server.Version, "test")
	assert.That(t, "protocol version", got.ProtocolVersion, protocol.MCPVersion)
	assert.That(t, "tools count", len(got.Tools), 1)
	assert.That(t, "tool name", got.Tools[0].Name, "sample")
	assert.That(t, "tool input schema type", got.Tools[0].InputSchema.Type, "object")
	assert.That(t, "resources count", len(got.Resources), 1)
	assert.That(t, "resource URI", got.Resources[0].URI, "file:///fixture")
	assert.That(t, "prompts count", len(got.Prompts), 1)
	assert.That(t, "prompt name", got.Prompts[0].Name, "sample-prompt")
}

type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, errExpectedFailure
}

var errExpectedFailure = &writeError{}

type writeError struct{}

func (*writeError) Error() string { return "boom" }

func Test_Inspect_With_MultipleEntriesPerRegistry_Should_SortByIdentityKey(t *testing.T) {
	t.Parallel()

	// Arrange — register out-of-alphabetical-order entries to exercise the
	// defensive sort comparators in collectTools / collectResources /
	// collectPrompts.
	tr := tools.NewRegistry()
	for _, name := range []string{"zeta", "alpha", "mu"} {
		if err := tools.Register[sampleIn, sampleOut](tr, name, "x",
			func(_ context.Context, in sampleIn) (sampleOut, tools.Result) {
				return sampleOut{Echoed: in.Greeting}, tools.Result{}
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	rr := resources.NewRegistry()
	for _, uri := range []string{"file:///z", "file:///a", "file:///m"} {
		if err := resources.Register(rr, uri, "n", "d",
			func(_ context.Context, _ string) (resources.Result, error) {
				return resources.Result{}, nil
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	pr := prompts.NewRegistry()
	for _, name := range []string{"zeta", "alpha", "mu"} {
		if err := prompts.Register[samplePromptIn](pr, name, "d",
			func(_ context.Context, _ samplePromptIn) prompts.Result {
				return prompts.Result{}
			},
		); err != nil {
			t.Fatal(err)
		}
	}

	// Act
	var buf bytes.Buffer
	if err := inspect.Inspect("mcp", "test", tr, rr, pr, &buf); err != nil {
		t.Fatalf("inspect: %v", err)
	}

	// Assert — sort comparators ran (identity key ordering).
	var got inspect.Output
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	assert.That(t, "tools sorted", []string{got.Tools[0].Name, got.Tools[1].Name, got.Tools[2].Name}, []string{"alpha", "mu", "zeta"})
	assert.That(t, "resources sorted", []string{got.Resources[0].URI, got.Resources[1].URI, got.Resources[2].URI}, []string{"file:///a", "file:///m", "file:///z"})
	assert.That(t, "prompts sorted", []string{got.Prompts[0].Name, got.Prompts[1].Name, got.Prompts[2].Name}, []string{"alpha", "mu", "zeta"})
}

func Test_Inspect_With_FailingWriter_Should_PropagateEncodeError(t *testing.T) {
	t.Parallel()

	// Arrange — minimal registries plus a writer that always errors.
	tr, rr, pr := newFixtureRegistries(t)

	// Act
	err := inspect.Inspect("mcp", "test", tr, rr, pr, errWriter{})

	// Assert — error wraps the encoder failure with context.
	assert.That(t, "error returned", err != nil, true)
}

func Test_Inspect_With_NilRegistries_Should_EmitEmptyArrays(t *testing.T) {
	t.Parallel()

	// Act
	var buf bytes.Buffer
	err := inspect.Inspect("mcp", "test", nil, nil, nil, &buf)

	// Assert
	assert.That(t, "render error", err, nil)
	var got inspect.Output
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Empty (not nil) — slices marshal as [] not null, so consumers can
	// always range over them. The test pins the shape contract.
	assert.That(t, "tools is non-nil empty", got.Tools != nil && len(got.Tools) == 0, true)
	assert.That(t, "resources is non-nil empty", got.Resources != nil && len(got.Resources) == 0, true)
	assert.That(t, "prompts is non-nil empty", got.Prompts != nil && len(got.Prompts) == 0, true)
}

// Test_Inspect_With_FixtureRegistries_Should_MatchGolden pins the wire-shape
// of the inspection output byte-for-byte against a committed golden file. The
// test regenerates the golden on drift (matching the spec-coverage discipline)
// so a developer who intentionally changes the shape gets a ready-to-commit
// diff rather than a cryptic byte-comparison failure.
//
//nolint:paralleltest // mutates testdata file on drift; cannot share with parallel siblings
func Test_Inspect_With_FixtureRegistries_Should_MatchGolden(t *testing.T) {
	tr, rr, pr := newFixtureRegistries(t)

	var buf bytes.Buffer
	if err := inspect.Inspect("mcp", "test", tr, rr, pr, &buf); err != nil {
		t.Fatalf("inspect: %v", err)
	}

	goldenPath := filepath.Join("testdata", "inspect.golden.json")
	existing, readErr := os.ReadFile(goldenPath) //nolint:gosec // known repo-relative path
	if readErr == nil && bytes.Equal(existing, buf.Bytes()) {
		return
	}
	if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	if err := os.WriteFile(goldenPath, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	t.Fatalf("internal/inspect/testdata/inspect.golden.json drifted — file regenerated, please review and commit it")
}
