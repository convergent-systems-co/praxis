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
	AuthorityModelID                        = "praxis.authority-model"
	AuthorityModelVersion                   = "v1"
	AuthorityModelV2Version                 = "v2"
	GovernedWorkPlanAccept                  = "workplan.accept"
	AuthorityRoutingTargetContributionIssue = "routing.target-contribution.issue"
	AuthorityRoutingSurfaceEligibilityIssue = "routing.surface-eligibility.issue"
	AuthorityModelMigrate                   = "authority.model.migrate"
)

func AuthorityModelDigest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelVersion, AuthorityDelegateCapability, GovernedWorkPlanAccept})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func AuthorityModelV2Digest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelV2Version, AuthorityDelegateCapability, GovernedWorkPlanAccept, AuthorityRoutingTargetContributionIssue, AuthorityRoutingSurfaceEligibilityIssue})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateAuthorityModel(id, version, digest string) error {
	if id != AuthorityModelID {
		return errors.New("authority model identity is unsupported")
	}
	switch version {
	case AuthorityModelVersion:
		if digest == AuthorityModelDigest() {
			return nil
		}
	case AuthorityModelV2Version:
		if digest == AuthorityModelV2Digest() {
			return nil
		}
	}
	return errors.New("authority model version or digest is unsupported")
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
	if parent.AuthorityModelVersion == AuthorityModelV2Version {
		return validateV2Delegation(request, now)
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

func validateV2Delegation(request DelegationRequest, now time.Time) error {
	if err := ValidateAuthorityModel(request.PolicyRef, request.PolicyVersion, request.PolicyDigest); err != nil || request.PolicyVersion != AuthorityModelV2Version {
		return errors.New("routing delegation requires authority model v2 policy")
	}
	wantKind, wantOperation, wantScope := "", "issue", ""
	switch request.RequestedAuthority {
	case AuthorityRoutingTargetContributionIssue:
		wantKind = "routing.target-contribution"
		wantScope, _ = RoutingTargetContributionScope(request.TargetIdentity, request.TargetVersion, request.TargetDigest)
	case AuthorityRoutingSurfaceEligibilityIssue:
		wantKind = "routing.surface-eligibility"
		if len(request.TargetConstraints) != 1 {
			return errors.New("surface eligibility delegation requires one canonical surface digest")
		}
		wantScope, _ = RoutingSurfaceEligibilityScope(request.TargetIdentity, request.TargetVersion, request.TargetDigest, request.TargetConstraints[0])
	default:
		return errors.New("v2 delegation authority is not in the closed routing table")
	}
	if request.TargetKind != wantKind || request.TargetIdentity == "" || request.TargetVersion == "" || request.RequestedScope != wantScope || request.RequestedOperation != wantOperation || len(request.RequestedCapabilities) != 0 || len(request.RequestedOperations) != 0 || !isSHA256Digest(request.TargetDigest) || !isSHA256Digest(request.ProposalDigest) || !isSHA256Digest(request.ReviewDigest) || request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return errors.New("v2 routing delegation requires exact target, issue operation, no capabilities, and future expiry")
	}
	return nil
}

func RoutingTargetContributionScope(identity, version, digest string) (string, error) {
	if identity == "" || version == "" || !isSHA256Digest(digest) {
		return "", errors.New("target contribution scope requires exact identity, version, and digest")
	}
	return fmt.Sprintf("routing-target/%s/%s/%s", identity, version, digest), nil
}

func RoutingSurfaceEligibilityScope(requestID, requestVersion, requestDigest, surfaceDigest string) (string, error) {
	if requestID == "" || requestVersion == "" || !isSHA256Digest(requestDigest) || !isSHA256Digest(surfaceDigest) {
		return "", errors.New("surface eligibility scope requires exact request and surface digests")
	}
	return fmt.Sprintf("routing-eligibility/%s/%s/%s/%s", requestID, requestVersion, requestDigest, surfaceDigest), nil
}
