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

// Spec normative levels per RFC 2119. Use these constants instead of string
// literals when constructing Clause entries so that misspellings become
// compile-time errors and the goconst linter does not flag the same literal
// across many test files.
const (
	LevelMAY    = "MAY"
	LevelMUST   = "MUST"
	LevelSHOULD = "SHOULD"
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

// clauseSites maps a registered Clause.ID to the "file:line" string of the
// site that called Register. Captured via runtime.Caller(1) so the panic on
// duplicate registration can name BOTH offending sites — the original and
// the duplicate — without forcing the operator to grep. Kept parallel to
// Clauses (rather than embedded in the Clause value) so the exported map's
// read shape stays stable for any future external iterator.
var clauseSites = map[string]string{}

// Register adds c to the registry. Panics on duplicate ID, missing fields,
// invalid Level, or nil entries in Tests — registration time is the right
// moment to fail loudly. On duplicate ID the panic names BOTH the first
// registration site and the duplicate site so the conflict is mechanical
// to resolve.
func Register(c Clause) {
	site := callerSite()
	if c.ID == "" {
		panic("protocol.Register: empty Clause.ID")
	}
	if existingSite, ok := clauseSites[c.ID]; ok {
		panic(fmt.Sprintf(
			"protocol.Register: duplicate clause ID %q (first registered at %s, duplicate at %s)",
			c.ID, existingSite, site,
		))
	}
	switch c.Level {
	case LevelMAY, LevelMUST, LevelSHOULD:
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
	clauseSites[c.ID] = site
}

// callerSite returns the "file:line" of the caller of Register, or "unknown"
// when the runtime cannot resolve the frame. Skip level 2: callerSite +
// Register + caller-of-Register.
func callerSite() string {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%s:%d", file, line)
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
