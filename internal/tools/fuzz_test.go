package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/tools"
)

func Fuzz_Search_With_ArbitraryInputs(f *testing.F) {
	f.Add("pattern", "/tmp", ".go")
	f.Add("", "", "")
	f.Add("[unclosed", "../../etc", "\x00")
	f.Add("a{99999}", "/nonexistent", ".txt")
	f.Add("(?i)hello", ".", ".md")
	f.Add(strings.Repeat("a", 1000), ".", "")
	f.Add("foo\x00bar", "../..", ".rs")
	f.Add("日本語", ".", ".go")

	f.Fuzz(func(_ *testing.T, pattern, path, ext string) {
		input := tools.SearchInput{
			Extensions: []string{ext},
			Path:       path,
			Pattern:    pattern,
		}
		_ = tools.Search(context.Background(), input)
	})
}

func Fuzz_ValidatePath_With_ArbitraryInput(f *testing.F) {
	f.Add("/valid/path")
	f.Add(".")
	f.Add("../../etc/passwd")
	f.Add(string([]byte{0x00}))
	f.Add(strings.Repeat("a", 5000))
	f.Add("日本語/パス")
	f.Add("/tmp/../../../etc/shadow")
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
