package protocol

import (
	"fmt"
	"io"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// Clause maps a single MUST/SHOULD/MAY-bearing requirement of the MCP
// specification to the test functions that prove the server complies. Tests
// are stored as function pointers — never strings — so that renaming or
// removing a covered test is a compile-time error rather than a silent
// runtime drift.
type Clause struct {
	ID      string
	Level   string
	Section string
	Summary string
	Tests   []func(*testing.T)
}

// Clauses is the package-global registry. A map keyed by ID prevents the
// duplicate-ID footgun at registration: Register panics if an ID is
// re-used, and the renderer iterates after sorting for deterministic output.
//
// Bootstrap entries are registered from init() blocks in _test.go files
// adjacent to the tests they reference. Consequently, the registry is empty
// in production binaries and only populates under `go test`.
var Clauses = map[string]Clause{}

// Register adds c to the registry. Panics on duplicate ID, missing fields,
// invalid Level, or nil entries in Tests — registration time is the right
// moment to fail loudly.
func Register(c Clause) {
	if c.ID == "" {
		panic("protocol.Register: empty Clause.ID")
	}
	if _, ok := Clauses[c.ID]; ok {
		panic("protocol.Register: duplicate clause ID: " + c.ID)
	}
	switch c.Level {
	case "MAY", "MUST", "SHOULD":
	default:
		panic(fmt.Sprintf("protocol.Register: clause %s has invalid Level %q (want MUST|SHOULD|MAY)", c.ID, c.Level))
	}
	if c.Section == "" {
		panic("protocol.Register: clause " + c.ID + " has empty Section")
	}
	if c.Summary == "" {
		panic("protocol.Register: clause " + c.ID + " has empty Summary")
	}
	if len(c.Tests) == 0 {
		panic("protocol.Register: clause " + c.ID + " has empty Tests slice")
	}
	for i, fn := range c.Tests {
		if fn == nil {
			panic(fmt.Sprintf("protocol.Register: clause %s has nil Tests[%d]", c.ID, i))
		}
	}
	Clauses[c.ID] = c
}

// Render writes a tab-separated coverage report (header + one row per
// clause) to w. Rows are sorted ascending by Clause.ID for deterministic
// output. Test function names are resolved via runtime.FuncForPC.
func Render(w io.Writer) error {
	if _, err := io.WriteString(w, "ID\tLevel\tSection\tTests\n"); err != nil {
		return fmt.Errorf("write header: %w", err)
	}

	ids := make([]string, 0, len(Clauses))
	for id := range Clauses {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		c := Clauses[id]
		names := make([]string, len(c.Tests))
		for i, fn := range c.Tests {
			ptr := reflect.ValueOf(fn).Pointer()
			info := runtime.FuncForPC(ptr)
			if info == nil || info.Name() == "" {
				return fmt.Errorf("clause %s: cannot resolve test[%d] name via runtime.FuncForPC", c.ID, i)
			}
			names[i] = info.Name()
		}
		row := strings.Join([]string{c.ID, c.Level, c.Section, strings.Join(names, ", ")}, "\t") + "\n"
		if _, err := io.WriteString(w, row); err != nil {
			return fmt.Errorf("write row %s: %w", c.ID, err)
		}
	}
	return nil
}
