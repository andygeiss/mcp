package protocol_test

import (
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

func init() {
	protocol.Register(protocol.Clause{
		ID:      "MCP-2025-11-25/initialize/MUST-version-negotiation",
		Level:   "MUST",
		Section: "R5 initialize protocol-version negotiation",
		Summary: "When the client requests an unsupported protocol version, the server returns its newest supported version as a counter-proposal rather than failing.",
		Tests: []func(*testing.T){
			Test_NegotiateVersion_With_UnsupportedVersion_Should_ReturnNewest,
		},
	})
}

func Test_SupportedVersions_Should_ContainCurrentVersion(t *testing.T) {
	t.Parallel()

	// Arrange
	want := protocol.MCPVersion

	// Act + Assert — the constant the server advertises as default must be a
	// member of the supported set; otherwise a client requesting "the same
	// version the server claims to speak" would get a counter-proposal back.
	assert.That(t, "supported", protocol.IsVersionSupported(want), true)
}

func Test_SupportedVersions_Should_ListNewestFirst(t *testing.T) {
	t.Parallel()

	// Act + Assert — MCPVersion is the canonical "newest" advertised version
	// and must equal the first element of SupportedVersions so a counter-
	// proposal always offers the newest.
	if len(protocol.SupportedVersions) == 0 {
		t.Fatal("SupportedVersions must not be empty")
	}
	assert.That(t, "newest first", protocol.SupportedVersions[0], protocol.MCPVersion)
}

func Test_IsVersionSupported_With_KnownVersion_Should_ReturnTrue(t *testing.T) {
	t.Parallel()

	// Cover the entire supported matrix so adding/removing a revision auto-
	// extends/shrinks coverage without test churn (Q1 plan: "for v := range
	// supported { test(v) }").
	for _, v := range protocol.SupportedVersions {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "supported "+v, protocol.IsVersionSupported(v), true)
		})
	}
}

func Test_IsVersionSupported_With_UnknownVersion_Should_ReturnFalse(t *testing.T) {
	t.Parallel()

	// Arrange — pick versions that pre-date and post-date the current one
	// plus an obviously bogus string, to cover the typical "client too old"
	// and "client too new" branches.
	for _, v := range []string{"", "2024-01-01", "2099-12-31", "draft", "1"} {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "supported "+v, protocol.IsVersionSupported(v), false)
		})
	}
}

func Test_NegotiateVersion_With_MatchedVersion_Should_EchoIt(t *testing.T) {
	t.Parallel()

	for _, v := range protocol.SupportedVersions {
		t.Run(v, func(t *testing.T) {
			t.Parallel()
			assert.That(t, "echo "+v, protocol.NegotiateVersion(v), v)
		})
	}
}

func Test_NegotiateVersion_With_UnsupportedVersion_Should_ReturnNewest(t *testing.T) {
	t.Parallel()

	// Arrange + Act + Assert
	got := protocol.NegotiateVersion("2099-12-31")

	// Assert — counter-proposal returns the canonical newest.
	assert.That(t, "counter-proposal", got, protocol.MCPVersion)
}

func Test_NegotiateVersion_With_EmptyClientVersion_Should_ReturnNewest(t *testing.T) {
	t.Parallel()

	// Arrange — client sends initialize without protocolVersion: empty string.
	// Act
	got := protocol.NegotiateVersion("")

	// Assert
	assert.That(t, "counter-proposal", got, protocol.MCPVersion)
}
