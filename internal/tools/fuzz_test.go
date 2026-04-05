package tools_test

import (
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/tools"
)

func Fuzz_ValidatePath_With_ArbitraryInput(f *testing.F) {
	f.Add("/valid/path")
	f.Add(".")
	f.Add("../../etc/passwd")
	f.Add(string([]byte{0x00}))
	f.Add(strings.Repeat("a", 5000))
	f.Add("日本語/パス")
	f.Add("/tmp/../../../etc/shadow")
	f.Add("foo/../../etc/passwd")
	f.Add("..")
	f.Add("")

	f.Fuzz(func(_ *testing.T, path string) {
		_ = tools.ValidatePath(path)
	})
}

func Fuzz_ValidateInput_With_ArbitraryInput(f *testing.F) {
	f.Add("hello world")
	f.Add("")
	f.Add(string([]byte{0x00}))
	f.Add(strings.Repeat("a", 5000))
	f.Add("日本語テスト")
	f.Add("foo\nbar\tbaz")

	f.Fuzz(func(_ *testing.T, input string) {
		_ = tools.ValidateInput(input)
	})
}
