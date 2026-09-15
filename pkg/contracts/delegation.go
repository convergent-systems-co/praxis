package contracts

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

const AuthorityDelegateCapability = "authority.delegate"

// DelegationRequest is the generic, exact authority payload carried by an
// AuthorityRequest. It describes a possible child; it is not authority.
type DelegationRequest struct {
	ParentRef             string       `json:"parent_ref"`
	ParentVersion         string       `json:"parent_version"`
	ParentDigest          string       `json:"parent_digest"`
	DelegatedPrincipal    PrincipalRef `json:"delegated_principal"`
	TargetKind            string       `json:"target_kind"`
	TargetIdentity        string       `json:"target_identity"`
	TargetConstraints     []string     `json:"target_constraints,omitempty"`
	RequestedCapabilities []string     `json:"requested_capabilities"`
	RequestedOperations   []string     `json:"requested_operations"`
	RequestedAuthority    string       `json:"requested_authority"`
	RequestedOperation    string       `json:"requested_operation"`
	RequestedScope        string       `json:"requested_scope"`
	TargetVersion         string       `json:"target_version"`
	TargetDigest          string       `json:"target_digest"`
	ProposalVersion       string       `json:"proposal_version"`
	ProposalDigest        string       `json:"proposal_digest"`
	ReviewVersion         string       `json:"review_version"`
	ReviewDigest          string       `json:"review_digest"`
	ExpiresAt             time.Time    `json:"expires_at"`
	Reason                string       `json:"reason"`
	PolicyRef             string       `json:"policy_ref"`
	PolicyVersion         string       `json:"policy_version"`
	PolicyDigest          string       `json:"policy_digest"`
}

func (d DelegationRequest) Validate(now time.Time) error {
	if d.ParentRef == "" || d.ParentVersion == "" || d.ParentDigest == "" || d.TargetKind == "" || d.TargetIdentity == "" || d.RequestedScope == "" || d.Reason == "" || d.PolicyRef == "" || d.PolicyVersion == "" || d.PolicyDigest == "" || d.ExpiresAt.IsZero() || !d.ExpiresAt.After(now) || d.TargetVersion == "" || d.TargetDigest == "" || d.ProposalVersion == "" || d.ProposalDigest == "" || d.ReviewVersion == "" || d.ReviewDigest == "" {
		return errors.New("delegation request requires exact parent, target, scope, policy, reason, and future expiry")
	}
	if err := d.DelegatedPrincipal.Validate(); err != nil {
		return fmt.Errorf("delegated principal: %w", err)
	}
	if d.RequestedAuthority == "" || d.RequestedOperation == "" {
		return errors.New("delegation request requires governed authority and operation")
	}
	if d.DelegatedPrincipal.ID == "" {
		return errors.New("delegated principal is required")
	}
	return nil
}

// DelegationContainmentPolicy is implemented by the authoritative policy
// registry. It intentionally has no lexical or set-intersection fallback.
type DelegationContainmentPolicy interface {
	ContainDelegation(parent AuthorityGeneration, request DelegationRequest, now time.Time) error
}

func CanonicalStringSet(values []string) []string {
	copy := append([]string(nil), values...)
	sort.Strings(copy)
	return copy
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
