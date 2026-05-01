package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Template-only paths the scaffold removes from a consumer's project. Hoisted
// as package constants so the goconst linter does not flag the same literals
// across the scaffold and its tests.
const (
	bmadDirName       = "_bmad"
	bmadOutputDirName = "_bmad-output"
	claudeDirName     = ".claude"
	claudeMdName      = "CLAUDE.md"
)

const (
	templateBinaryName = "mcp"
	templateModulePath = "github.com/andygeiss/mcp"
	templateRepoPath   = "andygeiss/mcp"
)

// repoShortForm returns the "owner/repo" portion of a module path — the two
// segments after the host. Returns "" when the path has fewer than three
// segments. A trailing `/vN` major-version suffix is naturally ignored
// because only the first two post-host segments are kept.
//
//	github.com/myorg/mytool        -> "myorg/mytool"
//	github.com/myorg/mytool/v2     -> "myorg/mytool"
//	gitlab.com/group/project       -> "group/project"
func repoShortForm(modulePath string) string {
	parts := strings.Split(strings.TrimRight(modulePath, "/"), "/")
	if len(parts) < 3 {
		return ""
	}
	return parts[1] + "/" + parts[2]
}

// rewriteProject performs all rewrite operations on the project rooted at dir.
// When force is false and dir is a git working tree with uncommitted changes,
// init refuses to run — the final resetGitHistory step would otherwise blow
// away the user's in-progress edits irrecoverably.
func rewriteProject(dir, modulePath string, force bool) error {
	if modulePath != templateModulePath && strings.HasPrefix(modulePath, templateModulePath) {
		return fmt.Errorf("module path %q must not extend template path %q", modulePath, templateModulePath)
	}
	if err := checkCleanWorkingTree(dir, force); err != nil {
		return err
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"rewrite go.mod", func() error { return rewriteGoMod(dir, modulePath) }},
		{"rewrite go files", func() error { return rewriteGoFiles(dir, modulePath) }},
		{"rewrite text files", func() error { return rewriteTextFiles(dir, modulePath) }},
		{"go mod tidy", func() error { return runGoModTidy(dir) }},
		{"self-cleanup", func() error { return selfCleanup(dir) }},
		{"remove build artifacts", func() error { return removeBuildArtifacts(dir) }},
		{"remove template-only content", func() error { return removeTemplateOnlyContent(dir) }},
		{"verify zero fingerprint", func() error { return verifyZeroFingerprint(dir) }},
		{"reset git history", func() error { return resetGitHistory(dir) }},
	}
	for _, step := range steps {
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Clean(path))
}

func writeFile(path string, data []byte) error {
	f, err := os.OpenFile(filepath.Clean(path), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func rewriteGoMod(dir, modulePath string) error {
	path := filepath.Join(dir, "go.mod")
	data, err := readFile(path)
	if err != nil {
		return err
	}
	replaced := bytes.Replace(data, []byte("module "+templateModulePath), []byte("module "+modulePath), 1)
	return writeFile(path, replaced)
}

func rewriteGoFiles(dir, modulePath string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if shouldSkip(dir, path, info) {
			return filepath.SkipDir
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		return rewriteImportsInFile(path, modulePath)
	})
}

// rewriteImportsInFile replaces template module path in import statements.
// bytes.ReplaceAll is used instead of go/parser+go/printer for simplicity —
// it handles grouped, individual, and aliased imports correctly. The init tool
// runs once and deletes itself, so the edge case of string literals containing
// the module path has near-zero blast radius.
func rewriteImportsInFile(path, modulePath string) error {
	data, err := readFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	replaced := bytes.ReplaceAll(data,
		[]byte(`"`+templateModulePath+`/`),
		[]byte(`"`+modulePath+`/`),
	)
	if bytes.Equal(data, replaced) {
		return nil
	}
	return writeFile(path, replaced)
}

func rewriteTextFiles(dir, modulePath string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if shouldSkip(dir, path, info) {
			return filepath.SkipDir
		}
		if info.IsDir() || info.Size() == 0 || strings.HasSuffix(path, ".go") {
			return nil
		}
		if !isTextFile(path) {
			return nil
		}
		return rewriteTextFile(path, modulePath)
	})
}

// rewriteTextFile replaces module path and bare "owner/repo" references in a
// text file. The full module path is substituted first so any embedded
// occurrence of the short form (inside the full form) is consumed before the
// short-form pass runs — that second pass then only touches the bare slug
// used by badge URLs (shields.io, codecov, etc.).
func rewriteTextFile(path, modulePath string) error {
	data, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File removed by prior step — idempotent.
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	replaced := bytes.ReplaceAll(data, []byte(templateModulePath), []byte(modulePath))
	if short := repoShortForm(modulePath); short != "" {
		replaced = bytes.ReplaceAll(replaced, []byte(templateRepoPath), []byte(short))
	}
	if bytes.Equal(data, replaced) {
		return nil
	}
	return writeFile(path, replaced)
}

// checkCleanWorkingTree refuses to proceed when dir is a git repo with
// uncommitted changes. init ends with resetGitHistory, which wipes .git and
// captures only the on-disk state — any unstaged work would be lost with no
// recovery path. When dir is not a git repo (fresh template extract, zip
// download), the check is a no-op.
func checkCleanWorkingTree(dir string, force bool) error {
	if force {
		return nil
	}
	info, statErr := os.Stat(filepath.Join(dir, ".git"))
	if os.IsNotExist(statErr) || (statErr == nil && !info.IsDir()) {
		return nil
	}
	if statErr != nil {
		return fmt.Errorf("stat .git: %w", statErr)
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if len(bytes.TrimSpace(out)) > 0 {
		return fmt.Errorf("working tree has uncommitted changes:\n%s\ncommit or stash them, or pass --force to proceed (destructive: init will reset git history)", out)
	}
	return nil
}

// resetGitHistory wipes any inherited .git and creates a single fresh commit on
// branch main. The consumer starts from a clean history under their own git
// identity — the template's PRs, tags, and authorship do not carry over.
// Runs last, so the initial commit captures the verified-clean project state.
func resetGitHistory(dir string) error {
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		return fmt.Errorf("remove .git: %w", err)
	}

	initCmd := exec.Command("git", "init", "-b", "main")
	initCmd.Dir = dir
	initCmd.Stdout = os.Stderr
	initCmd.Stderr = os.Stderr
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	addCmd.Stdout = os.Stderr
	addCmd.Stderr = os.Stderr
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "feat: initial version")
	commitCmd.Dir = dir
	commitCmd.Stdout = os.Stderr
	commitCmd.Stderr = os.Stderr
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

func runGoModTidy(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func removeBuildArtifacts(dir string) error {
	for _, name := range []string{templateBinaryName, "scaffold"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if rmErr := os.Remove(path); rmErr != nil {
				return rmErr
			}
		}
	}
	return nil
}

func isTextFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".go", ".md", ".mod", ".sum", ".yml", ".yaml", ".json", ".toml", ".txt", ".cfg":
		return true
	}
	return false
}

func verifyZeroFingerprint(dir string) error {
	var remaining []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if shouldSkip(dir, path, info) {
			return filepath.SkipDir
		}
		if info.IsDir() || info.Size() == 0 || !isTextFile(path) {
			return nil
		}
		data, readErr := readFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(data, []byte(templateModulePath)) || bytes.Contains(data, []byte(templateRepoPath)) {
			rel, _ := filepath.Rel(dir, path)
			remaining = append(remaining, rel)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("fingerprint check: %w", err)
	}
	if len(remaining) > 0 {
		return fmt.Errorf("template fingerprint remains in: %s", strings.Join(remaining, ", "))
	}
	return nil
}

func selfCleanup(dir string) error {
	scaffoldDir := filepath.Join(dir, "cmd", "scaffold")
	if _, err := os.Stat(scaffoldDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(scaffoldDir)
}

// removeTemplateOnlyContent deletes files and directories that exist to support
// the template itself but have no place in a consumer's project: Andy's
// engineering-context document (CLAUDE.md), the BMad workflow directories
// (_bmad, _bmad-output), and Claude Code's local config (.claude). os.RemoveAll
// is idempotent — missing paths are a no-op.
func removeTemplateOnlyContent(dir string) error {
	for _, name := range []string{claudeDirName, claudeMdName, bmadDirName, bmadOutputDirName} {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("remove %s: %w", name, err)
		}
	}
	return nil
}

func shouldSkip(root, path string, info os.FileInfo) bool {
	if !info.IsDir() {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	base := filepath.Base(rel)
	switch base {
	case ".git", bmadOutputDirName, bmadDirName, claudeDirName:
		return true
	}
	return false
}
