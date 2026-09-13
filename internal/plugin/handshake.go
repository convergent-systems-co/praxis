package plugin

import (
	"errors"
	"fmt"
)

type Handshake struct {
	Manifest              Manifest
	Identity              InstanceIdentity
	Isolation             IsolationProfile
	AdvertisedCapabilities []string
	Protocol              ProtocolRange
}

type HandshakeResult struct {
	NegotiatedProtocol string
	Provider           Provider
}

func ValidateHandshake(core ProtocolRange, h Handshake) (HandshakeResult, error) {
	if err := h.Manifest.Validate(); err != nil { return HandshakeResult{}, fmt.Errorf("manifest: %w", err) }
	if err := h.Identity.ValidateAgainst(h.Manifest); err != nil { return HandshakeResult{}, fmt.Errorf("identity: %w", err) }
	if err := h.Isolation.Satisfies(h.Manifest.RequiredIsolation); err != nil { return HandshakeResult{}, fmt.Errorf("isolation: %w", err) }
	v, err := NegotiateProtocol(core, h.Protocol)
	if err != nil { return HandshakeResult{}, err }
	if len(h.AdvertisedCapabilities) == 0 && len(h.Manifest.Capabilities) != 0 {
		return HandshakeResult{}, errors.New("plugin advertised no capabilities declared by manifest")
	}
	declared := make(map[string]struct{}, len(h.Manifest.Capabilities))
	for _, c := range h.Manifest.Capabilities { declared[c] = struct{}{} }
	seen := map[string]struct{}{}
	for _, c := range h.AdvertisedCapabilities {
		if c == "" { return HandshakeResult{}, errors.New("empty advertised capability") }
		if _, ok := declared[c]; !ok { return HandshakeResult{}, fmt.Errorf("undeclared advertised capability %q", c) }
		if _, dup := seen[c]; dup { return HandshakeResult{}, fmt.Errorf("duplicate advertised capability %q", c) }
		seen[c] = struct{}{}
	}
	for _, c := range h.Manifest.Capabilities {
		if _, ok := seen[c]; !ok { return HandshakeResult{}, fmt.Errorf("manifest capability %q was not advertised by runtime", c) }
	}
	return HandshakeResult{NegotiatedProtocol:v, Provider:Provider{Manifest:h.Manifest, Identity:h.Identity, State:StateStarting, Isolation:h.Isolation}}, nil
}
