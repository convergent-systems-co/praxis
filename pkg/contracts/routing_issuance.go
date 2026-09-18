package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

type RoutingIssuanceKind string

const (
	RoutingTargetContribution RoutingIssuanceKind = "target_contribution"
	RoutingSurfaceEligibility RoutingIssuanceKind = "surface_eligibility"
)

type RoutingIssuanceRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type RoutingIssuance struct {
	ID                string              `json:"id"`
	Version           string              `json:"version"`
	Kind              RoutingIssuanceKind `json:"kind"`
	Authority         string              `json:"authority"`
	Scope             string              `json:"scope"`
	RequestID         string              `json:"request_id"`
	RequestVersion    string              `json:"request_version"`
	DecisionRef       string              `json:"decision_ref"`
	DecisionVersion   string              `json:"decision_version"`
	GenerationRef     string              `json:"generation_ref"`
	GenerationVersion string              `json:"generation_version"`
	GenerationDigest  string              `json:"generation_digest"`
	IssuedBy          PrincipalRef        `json:"issued_by"`
	Payload           json.RawMessage     `json:"payload"`
	PayloadDigest     string              `json:"payload_digest"`
	EffectiveAt       time.Time           `json:"effective_at"`
	ExpiresAt         *time.Time          `json:"expires_at,omitempty"`
}

func FreezeRoutingIssuance(value RoutingIssuance) (RoutingIssuance, error) {
	value.ID = ""
	if value.Version != "1" || (value.Kind != RoutingTargetContribution && value.Kind != RoutingSurfaceEligibility) || value.Authority == "" || value.Scope == "" || value.RequestID == "" || value.RequestVersion == "" || value.DecisionRef == "" || value.DecisionVersion == "" || value.GenerationRef == "" || value.GenerationVersion == "" || !isSHA256Digest(value.GenerationDigest) || len(value.Payload) == 0 || value.EffectiveAt.IsZero() || value.IssuedBy.Validate() != nil {
		return RoutingIssuance{}, errors.New("routing issuance identity, authority, lineage, payload, and time are required")
	}
	sum := sha256.Sum256(value.Payload)
	if value.PayloadDigest != "sha256:"+hex.EncodeToString(sum[:]) {
		return RoutingIssuance{}, errors.New("routing issuance payload digest mismatch")
	}
	if value.ExpiresAt != nil && !value.ExpiresAt.After(value.EffectiveAt) {
		return RoutingIssuance{}, errors.New("routing issuance expiry must follow effective time")
	}
	encoded, _ := json.Marshal(value)
	identity := sha256.Sum256(encoded)
	value.ID = "sha256:" + hex.EncodeToString(identity[:])
	return value, nil
}

func VerifyRoutingIssuance(value RoutingIssuance) error {
	frozen, err := FreezeRoutingIssuance(value)
	if err != nil {
		return err
	}
	if frozen.ID != value.ID {
		return errors.New("routing issuance digest mismatch")
	}
	return nil
}
