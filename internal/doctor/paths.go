// Package doctor implements the `mcp doctor` subcommand: detect MCP client
// configurations on the host, validate referenced binary paths, and surface
// drift between the running mcp binary and any configured copies (FR10).
//
// The package is read-only by design: doctor never modifies a client config.
// Auto-repair is an explicit non-goal (see ADR-004) — surfacing the issue
// preserves user intent and avoids racing the client process.
package doctor

import (
	"path/filepath"
	"runtime"
)

// ClientConfig names a known MCP client and the location of its config file
// for a given user-config root. Path is a closure so OS-specific joins stay
// testable: callers inject the userConfigDir, and unit tests pin the
// produced path string per OS without touching the host filesystem.
type ClientConfig struct {
	ClientName string
	Path       func(userConfigDir string) string
}

// Clients returns the per-OS list of known MCP-client configuration
// locations. The returned slice is empty for unrecognized GOOS values so
// callers can iterate without nil-checking.
//
// The path conventions reflect publicly-documented client install layouts.
// Operators on machines with relocated config trees can still use doctor
// against a user-supplied path via a future flag — out of Q2 scope.
func Clients(goos string) []ClientConfig {
	switch goos {
	case "darwin":
		return []ClientConfig{
			{
				ClientName: "Claude Desktop",
				Path: func(u string) string {
					return filepath.Join(u, "Claude", "claude_desktop_config.json")
				},
			},
			{
				ClientName: "Cursor",
				Path: func(u string) string {
					return filepath.Join(u, "Cursor", "User", "settings.json")
				},
			},
			{
				ClientName: "VS Code",
				Path: func(u string) string {
					return filepath.Join(u, "Code", "User", "settings.json")
				},
			},
		}
	case "linux":
		return []ClientConfig{
			{
				ClientName: "Claude Desktop",
				Path: func(u string) string {
					return filepath.Join(u, "Claude", "claude_desktop_config.json")
				},
			},
			{
				ClientName: "Cursor",
				Path: func(u string) string {
					return filepath.Join(u, "Cursor", "User", "settings.json")
				},
			},
			{
				ClientName: "VS Code",
				Path: func(u string) string {
					return filepath.Join(u, "Code", "User", "settings.json")
				},
			},
		}
	case "windows":
		return []ClientConfig{
			{
				ClientName: "Claude Desktop",
				Path: func(u string) string {
					return filepath.Join(u, "Claude", "claude_desktop_config.json")
				},
			},
			{
				ClientName: "Cursor",
				Path: func(u string) string {
					return filepath.Join(u, "Cursor", "User", "settings.json")
				},
			},
			{
				ClientName: "VS Code",
				Path: func(u string) string {
					return filepath.Join(u, "Code", "User", "settings.json")
				},
			},
		}
	}
	return nil
}

// CurrentGOOS returns runtime.GOOS — wrapped so tests can inject alternatives
// via the parameter-passing form `Clients(goos)` rather than reaching for
// build tags.
func CurrentGOOS() string { return runtime.GOOS }
