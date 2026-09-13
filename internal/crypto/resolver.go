package crypto

import (
	"errors"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Capabilities describe suites actually available from a provider/peer.
type Capabilities struct {
	Classical bool
	PQ        bool
	Hybrid    bool
}

type Resolution struct {
	Requested contracts.CryptoProfile
	Selected  contracts.CryptoProfile
	Fallback  bool
}

// Resolve selects a profile without allowing silent downgrade.
func Resolve(requested contracts.CryptoProfile, caps Capabilities, allowPQPreferredFallback bool) (Resolution, error) {
	if err := requested.Validate(); err != nil { return Resolution{}, err }
	switch requested {
	case contracts.CryptoPQRequired:
		if !caps.PQ { return Resolution{}, errors.New("pq-required unavailable") }
		return Resolution{Requested: requested, Selected: contracts.CryptoPQRequired}, nil
	case contracts.CryptoHybridHighAssurance:
		if !caps.Hybrid || !caps.PQ || !caps.Classical { return Resolution{}, errors.New("hybrid-high-assurance unavailable") }
		return Resolution{Requested: requested, Selected: contracts.CryptoHybridHighAssurance}, nil
	case contracts.CryptoPQPreferred:
		if caps.PQ { return Resolution{Requested: requested, Selected: contracts.CryptoPQPreferred}, nil }
		if allowPQPreferredFallback && caps.Classical { return Resolution{Requested: requested, Selected: contracts.CryptoClassicalCompatible, Fallback: true}, nil }
		return Resolution{}, errors.New("pq-preferred unavailable and fallback not authorized")
	case contracts.CryptoClassicalCompatible:
		if !caps.Classical { return Resolution{}, errors.New("classical-compatible unavailable") }
		return Resolution{Requested: requested, Selected: contracts.CryptoClassicalCompatible}, nil
	default:
		return Resolution{}, errors.New("unsupported crypto profile")
	}
}
