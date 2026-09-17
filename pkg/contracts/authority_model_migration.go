package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const AuthorityModelV2MigrationID = "praxis.authority-model.v1-to-v2-root"

type AuthorityModelMigration struct {
	ID                      string       `json:"id"`
	Version                 string       `json:"version"`
	ProposalDigest          string       `json:"proposal_digest"`
	SourceRef               string       `json:"source_ref"`
	SourceVersion           string       `json:"source_version"`
	SourceDigest            string       `json:"source_digest"`
	TargetRef               string       `json:"target_ref"`
	TargetVersion           string       `json:"target_version"`
	TargetDigest            string       `json:"target_digest"`
	BootstrapDigest         string       `json:"bootstrap_digest"`
	PolicyRef               string       `json:"policy_ref"`
	PolicyVersion           string       `json:"policy_version"`
	PolicyDigest            string       `json:"policy_digest"`
	TransformID             string       `json:"transform_id"`
	AuthorizedBy            PrincipalRef `json:"authorized_by"`
	ApprovalRequestID       string       `json:"approval_request_id"`
	ApprovalRequestVersion  string       `json:"approval_request_version"`
	ApprovalRequestDigest   string       `json:"approval_request_digest"`
	ApprovalDecisionRef     string       `json:"approval_decision_ref"`
	ApprovalDecisionVersion string       `json:"approval_decision_version"`
	ApprovalDecisionDigest  string       `json:"approval_decision_digest"`
	EffectiveAt             time.Time    `json:"effective_at"`
}

func FreezeAuthorityModelMigration(source AuthorityGeneration, bootstrapDigest string, at time.Time) (AuthorityModelMigration, AuthorityGeneration, error) {
	rootScope, err := InstallationGovernanceScope(bootstrapDigest)
	if err != nil || source.ParentRef != "" || source.Ref != rootScope || source.Version != "1" || source.Scope != rootScope || source.Principal.Kind != "human" || source.Principal.ID != "installation-owner:"+bootstrapDigest || source.AuthorityModelVersion != AuthorityModelVersion || source.AuthorityModelDigest != AuthorityModelDigest() || source.VerifyDigest() != nil || !containsString(source.Capabilities, AuthorityDelegateCapability) || at.IsZero() {
		return AuthorityModelMigration{}, AuthorityGeneration{}, errors.New("authority-model migration requires the exact active v1 installation root")
	}
	target := AuthorityGeneration{Ref: rootScope, Version: "2", Principal: source.Principal, Scope: source.Scope, Capabilities: []string{AuthorityDelegateCapability}, ProvenanceRef: "authority-model-migration:" + source.Digest, ProvenanceDigest: source.Digest, State: AuthorityGenerationActive, EffectiveAt: at.UTC(), AuthorityModel: AuthorityModelID, AuthorityModelVersion: AuthorityModelV2Version, AuthorityModelDigest: AuthorityModelV2Digest()}
	target.Digest, err = target.ComputeDigest()
	if err != nil {
		return AuthorityModelMigration{}, AuthorityGeneration{}, err
	}
	m := AuthorityModelMigration{Version: "2", SourceRef: source.Ref, SourceVersion: source.Version, SourceDigest: source.Digest, TargetRef: target.Ref, TargetVersion: target.Version, TargetDigest: target.Digest, BootstrapDigest: bootstrapDigest, PolicyRef: AuthorityModelID, PolicyVersion: AuthorityModelV2Version, PolicyDigest: AuthorityModelV2Digest(), TransformID: AuthorityModelV2MigrationID, AuthorizedBy: source.Principal, EffectiveAt: at.UTC()}
	payload, _ := json.Marshal(m)
	sum := sha256.Sum256(payload)
	m.ProposalDigest = "sha256:" + hex.EncodeToString(sum[:])
	return m, target, nil
}

func VerifyAuthorityModelMigration(m AuthorityModelMigration, source, target AuthorityGeneration) error {
	if err := verifyAuthorityModelMigrationProposal(m, source, target); err != nil {
		return err
	}
	if m.ID == "" || m.ApprovalRequestID == "" || m.ApprovalRequestVersion == "" || !isSHA256Digest(m.ApprovalRequestDigest) || m.ApprovalDecisionRef == "" || m.ApprovalDecisionVersion == "" || !isSHA256Digest(m.ApprovalDecisionDigest) {
		return errors.New("authority-model migration requires exact approval identity")
	}
	candidate := m
	candidate.ID = ""
	payload, _ := json.Marshal(candidate)
	sum := sha256.Sum256(payload)
	if m.ID != "sha256:"+hex.EncodeToString(sum[:]) {
		return errors.New("authority-model migration content identity mismatch")
	}
	return nil
}

func verifyAuthorityModelMigrationProposal(m AuthorityModelMigration, source, target AuthorityGeneration) error {
	want, wantTarget, err := FreezeAuthorityModelMigration(source, m.BootstrapDigest, m.EffectiveAt)
	if err != nil {
		return err
	}
	candidate := m
	candidate.ID = ""
	candidate.ApprovalRequestID = ""
	candidate.ApprovalRequestVersion = ""
	candidate.ApprovalRequestDigest = ""
	candidate.ApprovalDecisionRef = ""
	candidate.ApprovalDecisionVersion = ""
	candidate.ApprovalDecisionDigest = ""
	wantJSON, _ := json.Marshal(want)
	gotJSON, _ := json.Marshal(candidate)
	wantTargetJSON, _ := json.Marshal(wantTarget)
	gotTargetJSON, _ := json.Marshal(target)
	if string(wantJSON) != string(gotJSON) || string(wantTargetJSON) != string(gotTargetJSON) {
		return errors.New("authority-model migration or successor mismatch")
	}
	return nil
}

func BindAuthorityModelMigrationApproval(m AuthorityModelMigration, source, target AuthorityGeneration, request AuthorityRequest, decision AuthorityDecision, now time.Time) (AuthorityModelMigration, error) {
	if err := verifyAuthorityModelMigrationProposal(m, source, target); err != nil {
		return AuthorityModelMigration{}, err
	}
	if m.ID == "" && (m.ApprovalRequestID != "" || m.ApprovalRequestVersion != "" || m.ApprovalRequestDigest != "" || m.ApprovalDecisionRef != "" || m.ApprovalDecisionVersion != "" || m.ApprovalDecisionDigest != "") {
		return AuthorityModelMigration{}, errors.New("authority-model migration contains partial approval binding")
	}
	if err := validateAuthorityModelMigrationApproval(m, source, target, request, decision, now); err != nil {
		return AuthorityModelMigration{}, err
	}
	requestDigest, err := request.Digest()
	if err != nil {
		return AuthorityModelMigration{}, err
	}
	decisionDigest, err := decision.Digest()
	if err != nil {
		return AuthorityModelMigration{}, err
	}
	bound := m
	bound.ID = ""
	bound.ApprovalRequestID, bound.ApprovalRequestVersion, bound.ApprovalRequestDigest = request.ID, request.Version, requestDigest
	bound.ApprovalDecisionRef, bound.ApprovalDecisionVersion, bound.ApprovalDecisionDigest = decision.DecisionRef, decision.DecisionVersion, decisionDigest
	payload, _ := json.Marshal(bound)
	sum := sha256.Sum256(payload)
	bound.ID = "sha256:" + hex.EncodeToString(sum[:])
	if m.ID != "" {
		left, _ := json.Marshal(m)
		right, _ := json.Marshal(bound)
		if string(left) != string(right) {
			return AuthorityModelMigration{}, errors.New("authority-model migration approval binding mismatch")
		}
	}
	return bound, nil
}

func ValidateAuthorityModelMigrationApproval(m AuthorityModelMigration, source, target AuthorityGeneration, request AuthorityRequest, decision AuthorityDecision, now time.Time) error {
	if err := VerifyAuthorityModelMigration(m, source, target); err != nil {
		return err
	}
	if err := validateAuthorityModelMigrationApproval(m, source, target, request, decision, now); err != nil {
		return err
	}
	requestDigest, _ := request.Digest()
	decisionDigest, _ := decision.Digest()
	if m.ApprovalRequestID != request.ID || m.ApprovalRequestVersion != request.Version || m.ApprovalRequestDigest != requestDigest || m.ApprovalDecisionRef != decision.DecisionRef || m.ApprovalDecisionVersion != decision.DecisionVersion || m.ApprovalDecisionDigest != decisionDigest {
		return errors.New("authority-model migration does not bind exact approval records")
	}
	return nil
}

func validateAuthorityModelMigrationApproval(m AuthorityModelMigration, source, target AuthorityGeneration, request AuthorityRequest, decision AuthorityDecision, now time.Time) error {
	if request.RequestedAuthority != AuthorityDelegateCapability || request.RequestedScope != source.Scope || request.Delegation == nil || decision.Outcome != AuthorityApprove {
		return errors.New("authority-model migration requires approved owner transition request")
	}
	if err := decision.Validate(request, now); err != nil {
		return err
	}
	d := request.Delegation
	if d.ParentRef != source.Ref || d.ParentVersion != source.Version || d.ParentDigest != source.Digest || d.DelegatedPrincipal != source.Principal || d.TargetKind != "authority.model.root" || d.TargetIdentity != target.Ref || d.TargetVersion != target.Version || d.TargetDigest != target.Digest || d.ProposalVersion != m.Version || d.ProposalDigest != m.ProposalDigest || d.ReviewVersion != AuthorityModelV2Version || d.ReviewDigest != AuthorityModelV2Digest() || d.RequestedAuthority != AuthorityModelMigrate || d.RequestedOperation != "migrate" || d.RequestedScope != source.Scope || len(d.RequestedCapabilities) != 0 || len(d.RequestedOperations) != 0 || d.PolicyRef != AuthorityModelID || d.PolicyVersion != AuthorityModelV2Version || d.PolicyDigest != AuthorityModelV2Digest() {
		return errors.New("authority-model migration approval does not bind exact transition")
	}
	if decision.AuthorityRef != source.Ref || decision.AuthorityVersion != source.Version || decision.AuthorityGenerationDigest != source.Digest || decision.DecidedBy != source.Principal || decision.GrantedScope != source.Scope {
		return errors.New("authority-model migration decision is not issued by exact installation owner root")
	}
	return nil
}
