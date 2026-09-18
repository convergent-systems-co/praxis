package plugin

import (
	"errors"
	"fmt"

	praxisv1 "github.com/convergent-systems-co/praxis/gen/praxis/v1"
)

// ValidateGRPCHandshake validates plugin-originated wire evidence against the
// verified package and runtime-bound launch identity. It derives protocol
// selection through core authority; the response cannot assert acceptance,
// grants, or the negotiated protocol.
func ValidateGRPCHandshake(core ProtocolRange, manifest Manifest, expected InstanceIdentity, isolation IsolationProfile, response *praxisv1.HandshakeResponse) (HandshakeResult, error) {
	if response == nil {
		return HandshakeResult{}, errors.New("plugin handshake response is required")
	}
	if response.Identity == nil {
		return HandshakeResult{}, errors.New("plugin handshake identity is required")
	}
	identity := InstanceIdentity{
		PluginID:       response.Identity.PluginId,
		PluginVersion:  response.Identity.PluginVersion,
		InstanceID:     response.Identity.InstanceId,
		ArtifactDigest: response.Identity.ArtifactDigest,
		RuntimeSession: response.Identity.RuntimeSession,
	}
	if identity != expected {
		return HandshakeResult{}, errors.New("plugin response identity does not match runtime-bound launch identity")
	}
	if !response.Ready {
		return HandshakeResult{}, fmt.Errorf("plugin did not report readiness: %v", response.Reasons)
	}
	capabilities := make([]string, 0, len(response.AdvertisedCapabilities))
	for _, advertisement := range response.AdvertisedCapabilities {
		if advertisement == nil {
			return HandshakeResult{}, errors.New("nil capability advertisement")
		}
		capabilities = append(capabilities, advertisement.Capability)
	}
	return ValidateHandshake(core, Handshake{
		Manifest:               manifest,
		Identity:               identity,
		Isolation:              isolation,
		AdvertisedCapabilities: capabilities,
		Protocol:               ProtocolRange{Min: response.PluginProtocolMin, Max: response.PluginProtocolMax},
	})
}
