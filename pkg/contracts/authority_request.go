package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type AuthorityRequestStatus string

const (
	AuthorityRequestPending     AuthorityRequestStatus = "pending"
	AuthorityRequestResolved    AuthorityRequestStatus = "resolved"
	AuthorityRequestInvalidated AuthorityRequestStatus = "invalidated"
)

type AuthorityDecisionOutcome string

const (
	AuthorityApprove      AuthorityDecisionOutcome = "approve"
	AuthorityReject       AuthorityDecisionOutcome = "reject"
	AuthorityRevise       AuthorityDecisionOutcome = "revise"
	AuthorityDefer        AuthorityDecisionOutcome = "defer"
	AuthorityInsufficient AuthorityDecisionOutcome = "insufficient_authority"
)

// AuthorityRequest is generic pending governance state. It describes the
// smallest decision boundary and evidence context without granting authority.
type AuthorityRequest struct {
	ID                    string                 `json:"id"`
	Version               string                 `json:"version"`
	BaselineID            string                 `json:"baseline_id"`
	BaselineVersion       string                 `json:"baseline_version"`
	BaselineDigest        string                 `json:"baseline_digest"`
	ProposalID            string                 `json:"proposal_id"`
	ProposalVersion       string                 `json:"proposal_version"`
	ProposalDigest        string                 `json:"proposal_digest"`
	ReviewRef             string                 `json:"review_ref"`
	ReviewVersion         string                 `json:"review_version"`
	ReviewDigest          string                 `json:"review_digest"`
	RequestedAuthority    string                 `json:"requested_authority"`
	RequestedScope        string                 `json:"requested_scope"`
	Reason                string                 `json:"reason"`
	AffectedWork          []string               `json:"affected_work,omitempty"`
	TransitivelyBlocked   []string               `json:"transitively_blocked,omitempty"`
	UnrelatedRunnableWork []string               `json:"unrelated_runnable_work,omitempty"`
	Recommendation        string                 `json:"recommendation,omitempty"`
	Alternatives          []string               `json:"alternatives,omitempty"`
	Status                AuthorityRequestStatus `json:"status"`
}

func (r AuthorityRequest) Validate() error {
	if r.ID == "" || r.Version == "" || r.BaselineID == "" || r.BaselineVersion == "" || r.BaselineDigest == "" || r.ProposalID == "" || r.ProposalVersion == "" || r.ProposalDigest == "" || r.ReviewRef == "" || r.ReviewVersion == "" || r.ReviewDigest == "" || r.RequestedAuthority == "" || r.RequestedScope == "" || r.Reason == "" {
		return errors.New("authority request requires exact evidence and decision scope")
	}
	if r.Status != AuthorityRequestPending && r.Status != AuthorityRequestResolved && r.Status != AuthorityRequestInvalidated {
		return fmt.Errorf("unknown authority request status %q", r.Status)
	}
	return nil
}

func (r AuthorityRequest) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type AuthorityDecision struct {
	RequestID        string                   `json:"request_id"`
	RequestVersion   string                   `json:"request_version"`
	RequestDigest    string                   `json:"request_digest"`
	DecisionRef      string                   `json:"decision_ref"`
	DecisionVersion  string                   `json:"decision_version"`
	DecidedBy        PrincipalRef             `json:"decided_by"`
	AuthorityRef     string                   `json:"authority_ref"`
	AuthorityVersion string                   `json:"authority_version"`
	GrantedScope     string                   `json:"granted_scope"`
	Outcome          AuthorityDecisionOutcome `json:"outcome"`
	AuthorityDigest  string                   `json:"authority_digest"`
	IssuedAt         time.Time                `json:"issued_at"`
	ExpiresAt        *time.Time               `json:"expires_at,omitempty"`
}

func (d AuthorityDecision) Digest() (string, error) {
	if d.RequestID == "" || d.RequestVersion == "" || d.RequestDigest == "" || d.DecisionRef == "" || d.DecisionVersion == "" {
		return "", errors.New("authority decision identity is required")
	}
	payload, err := json.Marshal(d)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

type AuthorityRevocation struct {
	RequestID         string       `json:"request_id"`
	RequestVersion    string       `json:"request_version"`
	DecisionRef       string       `json:"decision_ref"`
	DecisionVersion   string       `json:"decision_version"`
	DecisionDigest    string       `json:"decision_digest"`
	RevocationRef     string       `json:"revocation_ref"`
	RevocationVersion string       `json:"revocation_version"`
	RevokedBy         PrincipalRef `json:"revoked_by"`
	AuthorityDigest   string       `json:"authority_digest"`
	EffectiveAt       time.Time    `json:"effective_at"`
	Reason            string       `json:"reason"`
}

func (r AuthorityRevocation) Validate(decision AuthorityDecision) error {
	digest, err := decision.Digest()
	if err != nil {
		return err
	}
	if r.RequestID != decision.RequestID || r.RequestVersion != decision.RequestVersion || r.DecisionRef != decision.DecisionRef || r.DecisionVersion != decision.DecisionVersion || r.DecisionDigest != digest || r.RevocationRef == "" || r.RevocationVersion == "" || r.AuthorityDigest == "" || r.Reason == "" || r.EffectiveAt.IsZero() {
		return errors.New("authority revocation does not bind the exact decision")
	}
	if err := r.RevokedBy.Validate(); err != nil {
		return err
	}
	if r.RevokedBy.Kind != "human" && r.RevokedBy.Kind != "policy" && r.RevokedBy.Kind != "controller" {
		return errors.New("authority revocation principal is not a governance authority")
	}
	return nil
}

func (d AuthorityDecision) Validate(request AuthorityRequest, now time.Time) error {
	digest, err := request.Digest()
	if err != nil {
		return err
	}
	if d.RequestID != request.ID || d.RequestVersion != request.Version || d.RequestDigest != digest || d.DecisionRef == "" || d.DecisionVersion == "" || d.GrantedScope != request.RequestedScope || d.AuthorityDigest == "" {
		return errors.New("authority decision does not bind exact request and scope")
	}
	if err := d.DecidedBy.Validate(); err != nil {
		return err
	}
	if d.DecidedBy.Kind != "human" && d.DecidedBy.Kind != "policy" && d.DecidedBy.Kind != "controller" {
		return errors.New("authority decision principal is not a governance authority")
	}
	switch d.Outcome {
	case AuthorityApprove, AuthorityReject, AuthorityRevise, AuthorityDefer, AuthorityInsufficient:
	default:
		return fmt.Errorf("unknown authority decision outcome %q", d.Outcome)
	}
	if d.IssuedAt.IsZero() || (d.ExpiresAt != nil && !now.Before(*d.ExpiresAt)) {
		return errors.New("authority decision is missing or expired")
	}
	return nil
}
