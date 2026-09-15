package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	AuthorityModelID       = "praxis.authority-model"
	AuthorityModelVersion  = "v1"
	GovernedWorkPlanAccept = "workplan.accept"
)

func AuthorityModelDigest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelVersion, AuthorityDelegateCapability, GovernedWorkPlanAccept})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateAuthorityModel(id, version, digest string) error {
	if id != AuthorityModelID || version != AuthorityModelVersion || digest != AuthorityModelDigest() {
		return errors.New("authority model identity or digest is not the supported v1 model")
	}
	return nil
}

func WorkPlanAuthorityScope(goalID, baselineVersion, baselineDigest, proposalVersion, proposalDigest, reviewVersion, reviewDigest string) (string, error) {
	if goalID == "" || baselineVersion == "" || proposalVersion == "" || reviewVersion == "" || !isSHA256Digest(baselineDigest) || !isSHA256Digest(proposalDigest) || !isSHA256Digest(reviewDigest) {
		return "", errors.New("WorkPlan scope requires exact Goal, baseline, proposal, and review identities")
	}
	return fmt.Sprintf("goals-workplan/%s/%s/%s/%s/%s/%s/%s", goalID, baselineVersion, baselineDigest, proposalVersion, proposalDigest, reviewVersion, reviewDigest), nil
}

// ValidateBuiltinDelegation is the closed v1 containment rule for the only
// currently supported governance edge. It intentionally has no fallback.
func ValidateBuiltinDelegation(parent AuthorityGeneration, request DelegationRequest, now time.Time) error {
	if err := ValidateAuthorityModel(request.PolicyRef, request.PolicyVersion, request.PolicyDigest); err != nil {
		return fmt.Errorf("delegation policy is not built-in authority model v1: %w", err)
	}
	if err := ValidateAuthorityModel(parent.AuthorityModel, parent.AuthorityModelVersion, parent.AuthorityModelDigest); err != nil {
		return fmt.Errorf("parent authority model: %w", err)
	}
	if parent.ParentRef != "" || parent.Principal.Kind != "human" || !strings.HasPrefix(parent.Principal.ID, "installation-owner:") || !strings.HasPrefix(parent.Scope, InstallationGovernanceScopePrefix) || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("delegation parent is not the installation governance root")
	}
	if request.DelegatedPrincipal.Kind != "controller" || !strings.HasPrefix(request.DelegatedPrincipal.ID, "controller:") || request.DelegatedPrincipal == parent.Principal {
		return errors.New("v1 delegation requires a distinct controller principal")
	}
	if request.RequestedAuthority != GovernedWorkPlanAccept || request.RequestedOperation != "accept" || len(request.RequestedCapabilities) != 0 {
		return errors.New("v1 delegation permits only workplan.accept and no runtime capabilities")
	}
	if request.TargetKind != "goals.workplan" || request.TargetIdentity == "" || request.TargetVersion == "" || !isSHA256Digest(request.TargetDigest) || request.ProposalVersion == "" || !isSHA256Digest(request.ProposalDigest) || request.ReviewVersion == "" || !isSHA256Digest(request.ReviewDigest) {
		return errors.New("v1 delegation requires an exact protected WorkPlan target")
	}
	scope, err := WorkPlanAuthorityScope(request.TargetIdentity, request.TargetVersion, request.TargetDigest, request.ProposalVersion, request.ProposalDigest, request.ReviewVersion, request.ReviewDigest)
	if err != nil || request.RequestedScope != scope {
		return errors.New("v1 delegation scope is not the canonical exact WorkPlan scope")
	}
	if request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return errors.New("v1 delegation expiry is missing or expired")
	}
	return nil
}
