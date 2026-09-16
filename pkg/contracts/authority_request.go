package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

type AuthorityRequestStatus string

const (
	AuthorityRequestPending     AuthorityRequestStatus = "pending"
	AuthorityRequestResolved    AuthorityRequestStatus = "resolved"
	AuthorityRequestInvalidated AuthorityRequestStatus = "invalidated"
)

type AuthorityGenerationState string

const (
	AuthorityGenerationActive AuthorityGenerationState = "active"
)

type AuthorityGeneration struct {
	Ref                   string                   `json:"ref"`
	Version               string                   `json:"version"`
	Digest                string                   `json:"digest"`
	Principal             PrincipalRef             `json:"principal"`
	Scope                 string                   `json:"scope"`
	Capabilities          []string                 `json:"capabilities,omitempty"`
	ProvenanceRef         string                   `json:"provenance_ref"`
	ProvenanceDigest      string                   `json:"provenance_digest"`
	State                 AuthorityGenerationState `json:"state"`
	EffectiveAt           time.Time                `json:"effective_at"`
	ParentRef             string                   `json:"parent_ref,omitempty"`
	ParentVersion         string                   `json:"parent_version,omitempty"`
	ParentDigest          string                   `json:"parent_digest,omitempty"`
	DelegatedBy           PrincipalRef             `json:"delegated_by,omitempty"`
	DelegationRef         string                   `json:"delegation_ref,omitempty"`
	DelegationDigest      string                   `json:"delegation_digest,omitempty"`
	PolicyRef             string                   `json:"policy_ref,omitempty"`
	PolicyVersion         string                   `json:"policy_version,omitempty"`
	PolicyDigest          string                   `json:"policy_digest,omitempty"`
	ExpiresAt             *time.Time               `json:"expires_at,omitempty"`
	AuthorityModel        string                   `json:"authority_model,omitempty"`
	AuthorityModelVersion string                   `json:"authority_model_version,omitempty"`
	AuthorityModelDigest  string                   `json:"authority_model_digest,omitempty"`
	Authorities           []string                 `json:"authorities,omitempty"`
	DelegationProfile     string                   `json:"delegation_profile,omitempty"`
	SubjectKind           string                   `json:"subject_kind,omitempty"`
	SubjectID             string                   `json:"subject_id,omitempty"`
	SubjectVersion        string                   `json:"subject_version,omitempty"`
	SubjectDigest         string                   `json:"subject_digest,omitempty"`
	SubjectKeyDigest      string                   `json:"subject_key_digest,omitempty"`
}

type PackagePublishAuthorization struct {
	Generation AuthorityGeneration
	Request    AuthorityRequest
	Decision   AuthorityDecision
}

const InstallationGovernanceScopePrefix = "installation-governance:"

// InstallationGovernanceScope derives the sole governance-root scope from
// the protected bootstrap record identity. It is deliberately independent of
// any work, package, provider, repository, or invocation.
func InstallationGovernanceScope(bootstrapDigest string) (string, error) {
	if !isSHA256Digest(bootstrapDigest) {
		return "", errors.New("bootstrap digest must be a canonical sha256 digest")
	}
	return InstallationGovernanceScopePrefix + bootstrapDigest, nil
}

// InstallationOwnerPrincipal derives the installation-bound human principal
// from the same protected bootstrap identity used for the governance scope.
func InstallationOwnerPrincipal(bootstrapDigest string) (PrincipalRef, error) {
	if !isSHA256Digest(bootstrapDigest) {
		return PrincipalRef{}, errors.New("bootstrap digest must be a canonical sha256 digest")
	}
	return PrincipalRef{ID: "installation-owner:" + bootstrapDigest, Kind: "human"}, nil
}

func isSHA256Digest(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func (g AuthorityGeneration) Validate() error {
	if g.Ref == "" || g.Version == "" || g.Digest == "" || g.Scope == "" || g.ProvenanceRef == "" || g.ProvenanceDigest == "" || g.EffectiveAt.IsZero() {
		return errors.New("authority generation identity, scope, provenance, and effective time are required")
	}
	if err := g.Principal.Validate(); err != nil {
		return err
	}
	if g.State != AuthorityGenerationActive {
		return fmt.Errorf("authority generation is not active: %q", g.State)
	}
	if (g.ParentRef == "") != (g.ParentVersion == "") || (g.ParentRef == "") != (g.ParentDigest == "") {
		return errors.New("authority generation parent lineage is incomplete")
	}
	if g.ParentRef != "" {
		if err := g.DelegatedBy.Validate(); err != nil {
			return err
		}
		if g.DelegationRef == "" || g.DelegationDigest == "" || g.PolicyRef == "" || g.PolicyVersion == "" || g.PolicyDigest == "" {
			return errors.New("delegated generation provenance is incomplete")
		}
	}
	if g.ExpiresAt != nil && !g.ExpiresAt.After(g.EffectiveAt) {
		return errors.New("authority generation expiry must be after effective time")
	}
	if g.AuthorityModel != "" || g.AuthorityModelVersion != "" || g.AuthorityModelDigest != "" {
		if err := ValidateAuthorityModel(g.AuthorityModel, g.AuthorityModelVersion, g.AuthorityModelDigest); err != nil {
			return err
		}
	}
	return nil
}

// ComputeDigest derives the immutable identity of a generation from its
// non-digest fields. Callers must persist the returned value as Digest.
// Generation digests are evidence bindings, not substitutes for the durable
// generation record or its effective-state validation.
func (g AuthorityGeneration) ComputeDigest() (string, error) {
	validated := g
	if validated.Digest == "" {
		validated.Digest = "sha256:" + strings.Repeat("0", 64)
	}
	if err := validated.Validate(); err != nil {
		return "", err
	}
	copy := g
	copy.Digest = ""
	payload, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (g AuthorityGeneration) VerifyDigest() error {
	computed, err := g.ComputeDigest()
	if err != nil {
		return err
	}
	if computed != g.Digest {
		return errors.New("authority generation digest mismatch")
	}
	return nil
}

type AuthorityGenerationInvalidation struct {
	Ref                 string       `json:"ref"`
	Version             string       `json:"version"`
	GenerationDigest    string       `json:"generation_digest"`
	InvalidationRef     string       `json:"invalidation_ref"`
	InvalidationVersion string       `json:"invalidation_version"`
	Kind                string       `json:"kind"`
	SupersededBy        string       `json:"superseded_by,omitempty"`
	InvalidatedBy       PrincipalRef `json:"invalidated_by"`
	EffectiveAt         time.Time    `json:"effective_at"`
	Reason              string       `json:"reason"`
}

func (i AuthorityGenerationInvalidation) Validate(generation AuthorityGeneration) error {
	if i.Ref != generation.Ref || i.Version != generation.Version || i.GenerationDigest != generation.Digest || i.InvalidationRef == "" || i.InvalidationVersion == "" || i.Kind == "" || i.EffectiveAt.IsZero() || i.Reason == "" {
		return errors.New("authority generation invalidation does not bind the exact generation")
	}
	if i.Kind != "revoked" && i.Kind != "superseded" {
		return fmt.Errorf("unknown authority generation invalidation %q", i.Kind)
	}
	return i.InvalidatedBy.Validate()
}

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
	Delegation            *DelegationRequest     `json:"delegation,omitempty"`
	// Package deployment requests bind the exact verified action separately
	// from the long-lived PACKAGE_DEPLOY delegation.
	IntentDigest               string        `json:"intent_digest,omitempty"`
	InstallationDigest         string        `json:"installation_digest,omitempty"`
	ClosureDigest              string        `json:"closure_digest,omitempty"`
	VerificationEvidenceDigest string        `json:"verification_evidence_digest,omitempty"`
	Intent                     *ActionIntent `json:"intent,omitempty"`
}

func (r AuthorityRequest) Validate() error {
	if r.ID == "" || r.Version == "" || r.RequestedAuthority == "" || r.RequestedScope == "" || r.Reason == "" {
		return errors.New("authority request identity and decision scope are required")
	}
	if r.RequestedAuthority != AuthorityDelegateCapability && r.RequestedAuthority != GovernedPackagePublish && r.RequestedAuthority != GovernedPackageDeploy && (r.BaselineID == "" || r.BaselineVersion == "" || r.BaselineDigest == "" || r.ProposalID == "" || r.ProposalVersion == "" || r.ProposalDigest == "" || r.ReviewRef == "" || r.ReviewVersion == "" || r.ReviewDigest == "") {
		return errors.New("authority request requires exact evidence and decision scope")
	}
	if r.Status != AuthorityRequestPending && r.Status != AuthorityRequestResolved && r.Status != AuthorityRequestInvalidated {
		return fmt.Errorf("unknown authority request status %q", r.Status)
	}
	if r.RequestedAuthority == AuthorityDelegateCapability || r.RequestedAuthority == GovernedPackagePublish || (r.RequestedAuthority == GovernedPackageDeploy && r.Delegation != nil) {
		if r.Delegation == nil {
			return errors.New("delegation authority request requires a delegation payload")
		}
		if err := r.Delegation.Validate(time.Now().UTC()); err != nil {
			return err
		}
	}
	if r.RequestedAuthority == GovernedPackageDeploy && r.Delegation == nil && (r.IntentDigest == "" || r.InstallationDigest == "" || r.ClosureDigest == "" || r.VerificationEvidenceDigest == "") {
		return errors.New("exact package-deploy request requires intent, installation, closure, and verification evidence digests")
	}
	if r.RequestedAuthority == GovernedPackageDeploy && r.Delegation == nil {
		if r.Intent == nil {
			return errors.New("exact package-deploy request requires the canonical action intent")
		}
		d, err := r.Intent.Digest()
		if err != nil || d != r.IntentDigest || r.Intent.Operation != GovernedPackageDeploy {
			return errors.New("package-deploy request intent mismatch")
		}
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
	RequestID                            string                   `json:"request_id"`
	RequestVersion                       string                   `json:"request_version"`
	RequestDigest                        string                   `json:"request_digest"`
	DecisionRef                          string                   `json:"decision_ref"`
	DecisionVersion                      string                   `json:"decision_version"`
	DecidedBy                            PrincipalRef             `json:"decided_by"`
	AuthorityRef                         string                   `json:"authority_ref"`
	AuthorityVersion                     string                   `json:"authority_version"`
	AuthorityGenerationDigest            string                   `json:"authority_generation_digest"`
	OperationalAuthorityRef              string                   `json:"operational_authority_ref,omitempty"`
	OperationalAuthorityVersion          string                   `json:"operational_authority_version,omitempty"`
	OperationalAuthorityGenerationDigest string                   `json:"operational_authority_generation_digest,omitempty"`
	GrantedScope                         string                   `json:"granted_scope"`
	Outcome                              AuthorityDecisionOutcome `json:"outcome"`
	AuthorityDigest                      string                   `json:"authority_digest"`
	IssuedAt                             time.Time                `json:"issued_at"`
	ExpiresAt                            *time.Time               `json:"expires_at,omitempty"`
	Delegation                           *DelegationRequest       `json:"delegation,omitempty"`
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
	validScope := d.GrantedScope == request.RequestedScope
	if request.RequestedAuthority == AuthorityDelegateCapability && d.Delegation != nil {
		validScope = d.GrantedScope != "" && d.Delegation.RequestedScope == request.RequestedScope
	}
	if d.RequestID != request.ID || d.RequestVersion != request.Version || d.RequestDigest != digest || d.DecisionRef == "" || d.DecisionVersion == "" || !validScope || d.AuthorityDigest == "" {
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
	if request.RequestedAuthority == GovernedPackageDeploy && request.Delegation == nil {
		if d.AuthorityRef == "" || d.AuthorityVersion == "" || d.AuthorityGenerationDigest == "" || d.OperationalAuthorityRef == "" || d.OperationalAuthorityVersion == "" || d.OperationalAuthorityGenerationDigest == "" {
			return errors.New("exact package-deploy decision requires decision and operational authority lineage")
		}
	}
	if request.RequestedAuthority == AuthorityDelegateCapability || request.RequestedAuthority == GovernedPackagePublish || (request.RequestedAuthority == GovernedPackageDeploy && request.Delegation != nil) {
		if d.Delegation == nil || !reflect.DeepEqual(*d.Delegation, *request.Delegation) {
			return errors.New("delegation decision does not match exact request")
		}
	}
	return nil
}
