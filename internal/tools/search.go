package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	binaryCheckSize   = 512
	defaultMaxResults = 100
	maxFileSize       = 1 << 20 // 1 MB
	maxSearchDepth    = 20
)

// SearchInput defines the parameters for the search tool.
// Copy this pattern when adding tools with multiple input types.
type SearchInput struct {
	CaseSensitive bool     `json:"caseSensitive,omitempty" description:"Whether the search is case-sensitive"`
	Extensions    []string `json:"extensions,omitempty"    description:"File extensions to include (e.g. .go, .md)"`
	MaxResults    int      `json:"maxResults,omitempty"    description:"Maximum number of results to return"`
	Path          string   `json:"path"                   description:"Root directory to search in"`
	Pattern       string   `json:"pattern"                description:"The search pattern (regex supported)"`
}

// Search searches files under Path for lines matching Pattern.
func Search(ctx context.Context, input SearchInput) Result {
	if err := ValidatePath(input.Path); err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}
	if err := ValidateInput(input.Pattern); err != nil {
		return ErrorResult(fmt.Sprintf("invalid pattern: %v", err))
	}

	re, err := compilePattern(input.Pattern, input.CaseSensitive)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid pattern: %v", err))
	}

	root, err := validateSearchPath(input.Path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	limit := input.MaxResults
	if limit <= 0 {
		limit = defaultMaxResults
	}

	extSet := buildExtSet(input.Extensions)

	matches, err := walkAndMatch(ctx, root, re, extSet, limit)
	if err != nil {
		return ErrorResult(fmt.Sprintf("search failed: %v", err))
	}

	if len(matches) == 0 {
		return TextResult("no matches found")
	}
	return TextResult(strings.Join(matches, "\n"))
}

func compilePattern(pattern string, caseSensitive bool) (*regexp.Regexp, error) {
	expr := pattern
	if !caseSensitive {
		expr = "(?i)" + expr
	}
	return regexp.Compile(expr)
}

func buildExtSet(extensions []string) map[string]bool {
	set := make(map[string]bool, len(extensions))
	for _, ext := range extensions {
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		set[ext] = true
	}
	return set
}

func validateSearchPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve symlinks: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	wd, err = filepath.EvalSymlinks(wd)
	if err != nil {
		return "", fmt.Errorf("resolve working directory symlinks: %w", err)
	}
	if abs != wd && !strings.HasPrefix(abs, wd+string(filepath.Separator)) {
		return "", errors.New("path must be within working directory")
	}
	return abs, nil
}

func walkAndMatch(ctx context.Context, root string, re *regexp.Regexp, extSet map[string]bool, limit int) ([]string, error) {
	cleanRoot := filepath.Clean(root)
	var matches []string
	err := filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Enforce maximum search depth.
		rel, _ := filepath.Rel(cleanRoot, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if d.IsDir() && depth >= maxSearchDepth {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if len(extSet) > 0 && !extSet[filepath.Ext(path)] {
			return nil
		}

		found := matchFile(path, cleanRoot, re, limit-len(matches))
		matches = append(matches, found...)
		if len(matches) >= limit {
			return fs.SkipAll
		}
		return nil
	})
	return matches, err
}

func isBinary(data []byte) bool {
	check := data
	if len(check) > binaryCheckSize {
		check = check[:binaryCheckSize]
	}
	return bytes.Contains(check, []byte{0})
}

func matchFile(path, root string, re *regexp.Regexp, remaining int) []string {
	cleanPath := filepath.Clean(path)
	f, err := os.OpenFile(cleanPath, os.O_RDONLY|openNoFollowFlag, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil || info.IsDir() || info.Size() > maxFileSize {
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil
	}
	if int64(len(data)) > maxFileSize {
		return nil
	}
	if isBinary(data) {
		return nil
	}

	rel, _ := filepath.Rel(root, cleanPath)
	var matches []string
	for i, line := range strings.Split(string(data), "\n") {
		if re.MatchString(line) {
			matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, i+1, line))
			if len(matches) >= remaining {
				break
			}
		}
	}
	return matches
}
