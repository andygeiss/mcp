package doctor_test

import (
	"strings"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/doctor"
)

func Test_Clients_With_Darwin_Should_ReturnMacOSPaths(t *testing.T) {
	t.Parallel()

	got := doctor.Clients("darwin")
	assert.That(t, "three clients", len(got), 3)

	paths := pathsByClient(got, "/Users/andy/Library/Application Support")
	assertEndsWith(t, paths["Claude Desktop"], "Claude/claude_desktop_config.json")
	assertEndsWith(t, paths["Cursor"], "Cursor/User/settings.json")
	assertEndsWith(t, paths["VS Code"], "Code/User/settings.json")
}

func Test_Clients_With_Linux_Should_ReturnXDGPaths(t *testing.T) {
	t.Parallel()

	got := doctor.Clients("linux")
	assert.That(t, "three clients", len(got), 3)

	paths := pathsByClient(got, "/home/andy/.config")
	assertEndsWith(t, paths["Claude Desktop"], "Claude/claude_desktop_config.json")
	assertEndsWith(t, paths["Cursor"], "Cursor/User/settings.json")
}

func Test_Clients_With_Windows_Should_ReturnAppDataPaths(t *testing.T) {
	t.Parallel()

	got := doctor.Clients("windows")
	assert.That(t, "three clients", len(got), 3)

	paths := pathsByClient(got, `C:\Users\andy\AppData\Roaming`)
	// Windows path joins use backslashes; assert via Contains rather than
	// suffix because filepath.Join uses native separators.
	for _, want := range []string{"Claude", "claude_desktop_config.json"} {
		if !strings.Contains(paths["Claude Desktop"], want) {
			t.Errorf("Claude Desktop path %q missing %q", paths["Claude Desktop"], want)
		}
	}
}

func Test_Clients_With_UnknownGOOS_Should_ReturnEmpty(t *testing.T) {
	t.Parallel()

	got := doctor.Clients("plan9")
	assert.That(t, "no clients", len(got), 0)
}

func Test_CurrentGOOS_Should_ReturnNonEmptyString(t *testing.T) {
	t.Parallel()

	assert.That(t, "non-empty GOOS", doctor.CurrentGOOS() != "", true)
}

func pathsByClient(clients []doctor.ClientConfig, userConfigDir string) map[string]string {
	out := make(map[string]string, len(clients))
	for _, c := range clients {
		out[c.ClientName] = c.Path(userConfigDir)
	}
	return out
}

func assertEndsWith(t *testing.T, got, suffix string) {
	t.Helper()
	if !strings.HasSuffix(got, suffix) {
		t.Errorf("path %q does not end with %q", got, suffix)
	}
}
