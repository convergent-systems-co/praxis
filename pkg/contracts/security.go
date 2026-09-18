package contracts

import (
	"errors"
	"fmt"
	"time"
)

// TrustClass describes how evidence may be used. It is not an authorization level.
type TrustClass string

const (
	TrustUntrustedContent TrustClass = "untrusted_content"
	TrustObserved         TrustClass = "observed"
	TrustDerived          TrustClass = "derived"
	TrustUserConfirmed    TrustClass = "user_confirmed"
	TrustPolicy           TrustClass = "policy"
)

// PrincipalRef identifies an actor without embedding credentials.
type PrincipalRef struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

func (p PrincipalRef) Validate() error {
	if p.ID == "" || p.Kind == "" {
		return errors.New("principal id and kind are required")
	}
	return nil
}

// ProvenanceRef preserves where data/evidence originated.
type ProvenanceRef struct {
	SourcePrincipal *PrincipalRef `json:"source_principal,omitempty"`
	SourceType      string        `json:"source_type"`
	SourceURI       string        `json:"source_uri,omitempty"`
	Digest          string        `json:"digest,omitempty"`
	Trust           TrustClass    `json:"trust"`
	ObservedAt      time.Time     `json:"observed_at"`
}

func (p ProvenanceRef) Validate() error {
	if p.SourceType == "" {
		return errors.New("provenance source_type is required")
	}
	if p.Trust == "" {
		return errors.New("provenance trust class is required")
	}
	return nil
}

// CapabilityLease is deterministic authority delegated to one principal.
type CapabilityLease struct {
	ID                  string       `json:"id"`
	Principal           PrincipalRef `json:"principal"`
	Capability          string       `json:"capability"`
	Operations          []string     `json:"operations"`
	Scope               string       `json:"scope"`
	IssuedAt            time.Time    `json:"issued_at"`
	ExpiresAt           *time.Time   `json:"expires_at,omitempty"`
	RevokedAt           *time.Time   `json:"revoked_at,omitempty"`
	RemainingUses       *uint64      `json:"remaining_uses,omitempty"`
	RequiredEnforcement []string     `json:"required_enforcement,omitempty"`
}

func (l CapabilityLease) Validate(now time.Time) error {
	if l.ID == "" || l.Capability == "" || l.Scope == "" {
		return errors.New("lease id, capability, and scope are required")
	}
	if err := l.Principal.Validate(); err != nil {
		return fmt.Errorf("lease principal: %w", err)
	}
	if l.RevokedAt != nil {
		return errors.New("capability lease is revoked")
	}
	if l.ExpiresAt != nil && !now.Before(*l.ExpiresAt) {
		return errors.New("capability lease is expired")
	}
	if l.RemainingUses != nil && *l.RemainingUses == 0 {
		return errors.New("capability lease has no remaining uses")
	}
	return nil
}

// CryptoProfile expresses required cryptographic properties, not a raw algorithm.
type CryptoProfile string

const (
	CryptoClassicalCompatible CryptoProfile = "classical-compatible"
	CryptoPQPreferred         CryptoProfile = "pq-preferred"
	CryptoPQRequired          CryptoProfile = "pq-required"
	CryptoHybridHighAssurance CryptoProfile = "hybrid-high-assurance"
)

func (p CryptoProfile) Validate() error {
	switch p {
	case CryptoClassicalCompatible, CryptoPQPreferred, CryptoPQRequired, CryptoHybridHighAssurance:
		return nil
	default:
		return fmt.Errorf("unknown crypto profile %q", p)
	}
}

// ActionIntent is the canonical side effect requested before authorization/commit.
type ActionIntent struct {
	Version       string            `json:"version"`
	ID            string            `json:"id"`
	Actor         PrincipalRef      `json:"actor"`
	Operation     string            `json:"operation"`
	Target        string            `json:"target"`
	Parameters    map[string]string `json:"parameters,omitempty"`
	Scope         string            `json:"scope"`
	Preconditions map[string]string `json:"preconditions,omitempty"`
	CryptoProfile CryptoProfile     `json:"crypto_profile,omitempty"`
}

func (a ActionIntent) Validate() error {
	if a.Version == "" || a.ID == "" || a.Operation == "" || a.Target == "" || a.Scope == "" {
		return errors.New("action intent version, id, operation, target, and scope are required")
	}
	if err := a.Actor.Validate(); err != nil {
		return fmt.Errorf("action actor: %w", err)
	}
	if a.CryptoProfile != "" {
		if err := a.CryptoProfile.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ApprovalBinding binds authority to an exact intent digest or an explicitly bounded policy.
type ApprovalBinding struct {
	ID                                   string       `json:"id"`
	Approver                             PrincipalRef `json:"approver"`
	IntentDigest                         string       `json:"intent_digest,omitempty"`
	PolicyRef                            string       `json:"policy_ref,omitempty"`
	IssuedAt                             time.Time    `json:"issued_at"`
	ExpiresAt                            *time.Time   `json:"expires_at,omitempty"`
	RemainingUses                        uint64       `json:"remaining_uses"`
	RevokedAt                            *time.Time   `json:"revoked_at,omitempty"`
	DecisionRef                          string       `json:"decision_ref,omitempty"`
	DecisionVersion                      string       `json:"decision_version,omitempty"`
	DecisionDigest                       string       `json:"decision_digest,omitempty"`
	DecisionAuthorityRef                 string       `json:"decision_authority_ref,omitempty"`
	DecisionAuthorityVersion             string       `json:"decision_authority_version,omitempty"`
	DecisionAuthorityGenerationDigest    string       `json:"decision_authority_generation_digest,omitempty"`
	OperationalAuthorityRef              string       `json:"operational_authority_ref,omitempty"`
	OperationalAuthorityVersion          string       `json:"operational_authority_version,omitempty"`
	OperationalAuthorityGenerationDigest string       `json:"operational_authority_generation_digest,omitempty"`
	VerificationEvidenceDigest           string       `json:"verification_evidence_digest,omitempty"`
	InstallationDigest                   string       `json:"installation_digest,omitempty"`
}

func (a ApprovalBinding) Validate(now time.Time) error {
	if a.ID == "" {
		return errors.New("approval id is required")
	}
	if err := a.Approver.Validate(); err != nil {
		return fmt.Errorf("approver: %w", err)
	}
	if (a.IntentDigest == "") == (a.PolicyRef == "") {
		return errors.New("approval must bind exactly one of intent_digest or policy_ref")
	}
	if a.RevokedAt != nil {
		return errors.New("approval is revoked")
	}
	if a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return errors.New("approval is expired")
	}
	if a.RemainingUses == 0 {
		return errors.New("approval has no remaining uses")
	}
	return nil
}
