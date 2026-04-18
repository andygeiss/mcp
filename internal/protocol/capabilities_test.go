package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/andygeiss/mcp/internal/assert"
	"github.com/andygeiss/mcp/internal/protocol"
)

func Test_ClientCapabilities_Has_Should_HandleAllPermutations(t *testing.T) {
	t.Parallel()

	// Arrange
	all := &protocol.ClientCapabilities{
		Elicitation: &protocol.ElicitationCapability{},
		Roots:       &protocol.RootsCapability{},
		Sampling:    &protocol.SamplingCapability{},
	}
	onlySampling := &protocol.ClientCapabilities{Sampling: &protocol.SamplingCapability{}}
	onlyElicitation := &protocol.ClientCapabilities{Elicitation: &protocol.ElicitationCapability{}}
	onlyRoots := &protocol.ClientCapabilities{Roots: &protocol.RootsCapability{}}

	cases := []struct {
		name string
		caps *protocol.ClientCapabilities
		cap  protocol.Capability
		want bool
	}{
		{"nil receiver / sampling", nil, protocol.CapSampling, false},
		{"nil receiver / elicitation", nil, protocol.CapElicitation, false},
		{"nil receiver / roots", nil, protocol.CapRoots, false},
		{"all-fields-nil / sampling", &protocol.ClientCapabilities{}, protocol.CapSampling, false},
		{"all-fields-nil / elicitation", &protocol.ClientCapabilities{}, protocol.CapElicitation, false},
		{"all-fields-nil / roots", &protocol.ClientCapabilities{}, protocol.CapRoots, false},
		{"only sampling / sampling", onlySampling, protocol.CapSampling, true},
		{"only sampling / elicitation", onlySampling, protocol.CapElicitation, false},
		{"only sampling / roots", onlySampling, protocol.CapRoots, false},
		{"only elicitation / elicitation", onlyElicitation, protocol.CapElicitation, true},
		{"only elicitation / sampling", onlyElicitation, protocol.CapSampling, false},
		{"only elicitation / roots", onlyElicitation, protocol.CapRoots, false},
		{"only roots / roots", onlyRoots, protocol.CapRoots, true},
		{"only roots / sampling", onlyRoots, protocol.CapSampling, false},
		{"only roots / elicitation", onlyRoots, protocol.CapElicitation, false},
		{"all set / sampling", all, protocol.CapSampling, true},
		{"all set / elicitation", all, protocol.CapElicitation, true},
		{"all set / roots", all, protocol.CapRoots, true},
		{"unknown capability", all, protocol.Capability("invented/unknown"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Act
			got := tc.caps.Has(tc.cap)

			// Assert
			assert.That(t, "Has result", got, tc.want)
		})
	}
}

func Test_ClientCapabilities_JSON_Should_OmitNilSubCapabilities(t *testing.T) {
	t.Parallel()

	// Arrange — only sampling advertised
	caps := &protocol.ClientCapabilities{Sampling: &protocol.SamplingCapability{}}

	// Act
	raw, err := json.Marshal(caps)

	// Assert — elicitation and roots must NOT appear; sampling appears as `{}`
	assert.That(t, "no error", err, error(nil))
	assert.That(t, "exact wire format", string(raw), `{"sampling":{}}`)
}

func Test_ClientCapabilities_JSON_Should_DistinguishAbsentFromPresentEmpty(t *testing.T) {
	t.Parallel()

	// Arrange — round-trip both shapes through JSON
	absentBytes := []byte(`{}`)
	presentEmptyBytes := []byte(`{"sampling":{}}`)

	// Act
	var absent, presentEmpty protocol.ClientCapabilities
	errAbsent := json.Unmarshal(absentBytes, &absent)
	errPresent := json.Unmarshal(presentEmptyBytes, &presentEmpty)

	// Assert
	assert.That(t, "absent decode ok", errAbsent, error(nil))
	assert.That(t, "present-empty decode ok", errPresent, error(nil))
	assert.That(t, "absent.Sampling is nil", absent.Sampling == nil, true)
	assert.That(t, "present-empty.Sampling is non-nil", presentEmpty.Sampling != nil, true)
}
