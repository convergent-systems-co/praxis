package plugin

import (
	"errors"
	"fmt"
)

type Handshake struct {
	Manifest               Manifest
	Identity               InstanceIdentity
	Isolation              IsolationProfile
	AdvertisedCapabilities []string
	Protocol               ProtocolRange
}

type HandshakeResult struct {
	NegotiatedProtocol string
	Provider           Provider
}

func ValidateHandshake(core ProtocolRange, h Handshake) (HandshakeResult, error) {
	if err := h.Manifest.Validate(); err != nil {
		return HandshakeResult{}, fmt.Errorf("manifest: %w", err)
	}
	if err := h.Identity.ValidateAgainst(h.Manifest); err != nil {
		return HandshakeResult{}, fmt.Errorf("identity: %w", err)
	}
	if err := h.Isolation.Satisfies(h.Manifest.RequiredIsolation); err != nil {
		return HandshakeResult{}, fmt.Errorf("isolation: %w", err)
	}
	v, err := NegotiateProtocol(core, h.Protocol)
	if err != nil {
		return HandshakeResult{}, err
	}
	if err := validateCapabilityAdvertisement(h.Manifest.Capabilities, h.AdvertisedCapabilities); err != nil {
		return HandshakeResult{}, err
	}
	return HandshakeResult{NegotiatedProtocol: v, Provider: Provider{Manifest: h.Manifest, Identity: h.Identity, State: StateStarting, Isolation: h.Isolation, advertisedCapabilities: append([]string(nil), h.AdvertisedCapabilities...), advertisementValidated: true}}, nil
}

// validateCapabilityAdvertisement enforces the signed manifest as an upper
// bound. Missing permitted capabilities describe current instance availability;
// they are not silently restored from package metadata.
func validateCapabilityAdvertisement(permitted, advertised []string) error {
	declared := make(map[string]struct{}, len(permitted))
	for _, c := range permitted {
		declared[c] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, c := range advertised {
		if c == "" {
			return errors.New("empty advertised capability")
		}
		if _, ok := declared[c]; !ok {
			return fmt.Errorf("undeclared advertised capability %q", c)
		}
		if _, dup := seen[c]; dup {
			return fmt.Errorf("duplicate advertised capability %q", c)
		}
		seen[c] = struct{}{}
	}
	return nil
}
