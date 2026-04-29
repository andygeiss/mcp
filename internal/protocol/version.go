package protocol

import "slices"

// SupportedVersions is the ordered list of MCP protocol revisions this server
// can speak, newest first. The server advertises the first (newest) entry as
// its preferred protocol; on initialize, if the client requests any entry in
// this list, the server echoes that entry back. Otherwise the server
// counter-proposes the newest supported and the client decides whether to
// proceed (per MCP 2025-11-25 §Version Negotiation).
//
// To add a new revision: prepend it here, update MCPVersion to match the
// newest entry, and exercise the matrix via for-range tests. Removing an
// older revision is a wire breaking change — only do it when no current
// client of record requires it.
var SupportedVersions = []string{
	"2025-11-25",
}

// IsVersionSupported reports whether v exactly matches one of the entries in
// SupportedVersions. Exact match only — there is no semver-style range
// matching in MCP; revisions are date-stamped and either match or do not.
func IsVersionSupported(v string) bool {
	return slices.Contains(SupportedVersions, v)
}

// NegotiateVersion returns the protocol version to echo back in the
// initialize response. If the client's requested version is in
// SupportedVersions, that exact string is returned. Otherwise the newest
// supported version is returned, signalling a counter-proposal that the
// client may accept or reject.
func NegotiateVersion(clientRequested string) string {
	if IsVersionSupported(clientRequested) {
		return clientRequested
	}
	return MCPVersion
}
