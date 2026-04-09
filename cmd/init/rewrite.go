package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	templateBinaryName = "mcp"
	templateModulePath = "github.com/andygeiss/mcp"
)

// deriveProjectName returns the last segment of a module path.
func deriveProjectName(modulePath string) string {
	modulePath = strings.TrimRight(modulePath, "/")
	i := strings.LastIndex(modulePath, "/")
	if i < 0 {
		return modulePath
	}
	return modulePath[i+1:]
}

// rewriteProject performs all rewrite operations on the project rooted at dir.
func rewriteProject(dir, modulePath string) error {
	if modulePath != templateModulePath && strings.HasPrefix(modulePath, templateModulePath) {
		return fmt.Errorf("module path %q must not extend template path %q", modulePath, templateModulePath)
	}
	projectName := deriveProjectName(modulePath)

	if err := rewriteGoMod(dir, modulePath); err != nil {
		return fmt.Errorf("rewrite go.mod: %w", err)
	}

	if err := rewriteGoFiles(dir, modulePath); err != nil {
		return fmt.Errorf("rewrite go files: %w", err)
	}

	if err := rewriteTextFiles(dir, modulePath, projectName); err != nil {
		return fmt.Errorf("rewrite text files: %w", err)
	}

	if err := renameBinaryDir(dir, projectName); err != nil {
		return fmt.Errorf("rename binary dir: %w", err)
	}

	if err := runGoModTidy(dir); err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	if err := selfCleanup(dir); err != nil {
		return fmt.Errorf("self-cleanup: %w", err)
	}

	if err := removeBuildArtifacts(dir); err != nil {
		return fmt.Errorf("remove build artifacts: %w", err)
	}

	if err := verifyZeroFingerprint(dir); err != nil {
		return fmt.Errorf("verify zero fingerprint: %w", err)
	}

	return nil
}

// readFile reads a file at the given path, sanitizing it first.
func readFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Clean(path))
}

// writeFile writes data to the given path, sanitizing it first.
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

// rewriteGoMod replaces the module path in go.mod.
func rewriteGoMod(dir, modulePath string) error {
	path := filepath.Join(dir, "go.mod")
	data, err := readFile(path)
	if err != nil {
		return err
	}
	replaced := bytes.Replace(data, []byte("module "+templateModulePath), []byte("module "+modulePath), 1)
	return writeFile(path, replaced)
}

// rewriteGoFiles walks all .go files and replaces template import paths.
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

// rewriteTextFiles walks all non-.go text files and rewrites template references.
func rewriteTextFiles(dir, modulePath, projectName string) error {
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
		return rewriteTextFile(path, modulePath, projectName)
	})
}

// rewriteTextFile replaces module path and binary name references in a text file.
func rewriteTextFile(path, modulePath, projectName string) error {
	data, err := readFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File removed by prior step — idempotent.
		}
		return fmt.Errorf("read %s: %w", path, err)
	}
	replaced := bytes.ReplaceAll(data, []byte(templateModulePath), []byte(modulePath))
	replaced = bytes.ReplaceAll(replaced, []byte("cmd/"+templateBinaryName+"/"), []byte("cmd/"+projectName+"/"))
	replaced = bytes.ReplaceAll(replaced, []byte("cmd/"+templateBinaryName+" "), []byte("cmd/"+projectName+" "))
	if bytes.Equal(data, replaced) {
		return nil
	}
	return writeFile(path, replaced)
}

// renameBinaryDir renames cmd/mcp/ to cmd/<projectName>/.
func renameBinaryDir(dir, projectName string) error {
	oldDir := filepath.Clean(filepath.Join(dir, "cmd", templateBinaryName))
	newDir := filepath.Clean(filepath.Join(dir, "cmd", projectName))

	if oldDir == newDir {
		return nil
	}

	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		// Already renamed — idempotent.
		if _, statErr := os.Stat(newDir); statErr == nil {
			return nil
		}
		return fmt.Errorf("neither %s nor %s exists", oldDir, newDir)
	}

	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("target directory already exists: %s", newDir)
	}

	return os.Rename(oldDir, newDir)
}

// runGoModTidy executes go mod tidy in the project directory.
func runGoModTidy(dir string) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// removeBuildArtifacts removes any compiled binaries from the project root.
func removeBuildArtifacts(dir string) error {
	for _, name := range []string{templateBinaryName, "init"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			if rmErr := os.Remove(path); rmErr != nil {
				return rmErr
			}
		}
	}
	return nil
}

// isTextFile returns true if the file is likely a text file based on extension.
func isTextFile(path string) bool {
	ext := filepath.Ext(path)
	switch ext {
	case ".go", ".md", ".mod", ".sum", ".yml", ".yaml", ".json", ".toml", ".txt", ".cfg":
		return true
	}
	return false
}

// verifyZeroFingerprint checks that no template references remain.
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
		if bytes.Contains(data, []byte(templateModulePath)) {
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

// selfCleanup removes the cmd/init/ directory after successful init.
func selfCleanup(dir string) error {
	initDir := filepath.Join(dir, "cmd", "init")
	if _, err := os.Stat(initDir); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(initDir)
}

// shouldSkip returns true for directories that should not be processed.
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
	case ".git", "_bmad-output", "_bmad", ".claude":
		return true
	}
	return false
}
