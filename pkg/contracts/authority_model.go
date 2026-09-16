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
	AuthorityModelID                = "praxis.authority-model"
	AuthorityModelVersion           = "v1"
	AuthorityModelSuccessorVersion  = "v2"
	AuthorityModelDeploymentVersion = "v3"
	GovernedWorkPlanAccept          = "workplan.accept"
	GovernedPackagePublish          = "package.publish"
	DelegationProfileWorkPlanAccept = "WORKPLAN_ACCEPT"
	DelegationProfilePackagePublish = "PACKAGE_PUBLISH"
	GovernedPackageDeploy           = "package.deploy"
	DelegationProfilePackageDeploy  = "PACKAGE_DEPLOY"
	PackageManagerPrincipalID       = "package-manager:praxis"
	PackageManagerPrincipalKind     = "package-manager"
)

func AuthorityModelDigest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelVersion, AuthorityDelegateCapability, GovernedWorkPlanAccept})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AuthorityModelSuccessorDigest identifies the closed v2 profile registry;
// v1 remains immutable and continues to validate with AuthorityModelDigest.
func AuthorityModelSuccessorDigest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelSuccessorVersion, AuthorityDelegateCapability, GovernedWorkPlanAccept, GovernedPackagePublish, DelegationProfileWorkPlanAccept, DelegationProfilePackagePublish})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// AuthorityModelDeploymentDigest identifies the immutable v3 profile set.
// v1 and v2 remain independently valid historical model identities.
func AuthorityModelDeploymentDigest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelDeploymentVersion, AuthorityDelegateCapability, GovernedWorkPlanAccept, GovernedPackagePublish, GovernedPackageDeploy, DelegationProfileWorkPlanAccept, DelegationProfilePackagePublish, DelegationProfilePackageDeploy})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidateAuthorityModel(id, version, digest string) error {
	if id != AuthorityModelID || (version != AuthorityModelVersion && version != AuthorityModelSuccessorVersion && version != AuthorityModelDeploymentVersion) || (version == AuthorityModelVersion && digest != AuthorityModelDigest()) || (version == AuthorityModelSuccessorVersion && digest != AuthorityModelSuccessorDigest()) || (version == AuthorityModelDeploymentVersion && digest != AuthorityModelDeploymentDigest()) {
		return errors.New("authority model identity or digest is not a supported immutable model")
	}
	return nil
}

// PackageManagerPrincipal is the stable logical identity of the local package
// lifecycle subsystem. Installation binding is carried by its delegated
// generation's exact parent/root lineage, not by a global principal name.
func PackageManagerPrincipal() PrincipalRef {
	return PrincipalRef{ID: PackageManagerPrincipalID, Kind: PackageManagerPrincipalKind}
}

func PackageDeploymentScope(installationGenerationDigest string) (string, error) {
	if !isSHA256Digest(installationGenerationDigest) {
		return "", errors.New("package deployment scope requires an exact installation generation sha256 digest")
	}
	return "package-deployment:installation:" + installationGenerationDigest, nil
}

// ValidateBuiltinPackageDeployDelegation is the closed v3 containment rule.
// It grants only governed package deployment; activation and runtime leases
// remain separate effects/capabilities.
func ValidateBuiltinPackageDeployDelegation(parent AuthorityGeneration, request DelegationRequest, now time.Time) error {
	if request.Profile != DelegationProfilePackageDeploy || request.ParentRef != parent.Ref || request.ParentVersion != parent.Version || request.ParentDigest != parent.Digest || request.RequestedAuthority != GovernedPackageDeploy || request.RequestedOperation != "deploy" || len(request.RequestedCapabilities) != 0 || len(request.RequestedOperations) != 0 {
		return errors.New("delegation is not the closed package-deploy profile")
	}
	if request.DelegatedPrincipal != PackageManagerPrincipal() {
		return errors.New("package-deploy delegation requires the canonical package-manager principal")
	}
	if request.TargetKind != PackageManagerPrincipalKind || request.TargetIdentity != PackageManagerPrincipalID || request.TargetVersion == "" || !isSHA256Digest(request.TargetDigest) || request.TargetDigest != parent.Digest || request.SubjectKind != PackageManagerPrincipalKind || request.SubjectID != PackageManagerPrincipalID || request.SubjectVersion == "" || request.SubjectDigest != parent.Digest {
		return errors.New("package-deploy delegation does not bind the exact package-manager generation")
	}
	if request.TargetConstraints == nil || len(request.TargetConstraints) != 1 || request.TargetConstraints[0] != request.TargetDigest {
		return errors.New("package-deploy delegation requires the exact deployment constraint")
	}
	if request.PolicyRef != AuthorityModelID || request.PolicyVersion != AuthorityModelDeploymentVersion || request.PolicyDigest != AuthorityModelDeploymentDigest() {
		return errors.New("package-deploy delegation must use authority-model v3")
	}
	installationScope, scopeErr := InstallationGovernanceScope(parent.ProvenanceDigest)
	if scopeErr != nil || parent.ParentRef != "" || parent.Principal.Kind != "human" || parent.Principal.ID != "installation-owner:"+parent.ProvenanceDigest || parent.Scope != installationScope || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("package-deploy parent is not the installation governance root")
	}
	scope, err := PackageDeploymentScope(request.TargetDigest)
	if err != nil || request.RequestedScope != scope || request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return errors.New("package-deploy scope or expiry is invalid")
	}
	return nil
}

func PackagePublishScope(namespace string) (string, error) {
	if namespace == "" || strings.ContainsAny(namespace, "*/\\") || strings.Contains(namespace, "..") || strings.ContainsAny(namespace, " \t\r\n") {
		return "", errors.New("package namespace is malformed")
	}
	return "package-namespace:" + namespace, nil
}

func PackageIDInNamespace(packageID, namespace string) bool {
	if packageID == "" || namespace == "" {
		return false
	}
	return packageID == namespace || strings.HasPrefix(packageID, namespace+".")
}

func ValidateBuiltinPackagePublishDelegation(parent AuthorityGeneration, request DelegationRequest, now time.Time) error {
	if request.Profile != DelegationProfilePackagePublish || request.RequestedAuthority != GovernedPackagePublish || request.RequestedOperation != "sign" || len(request.RequestedCapabilities) != 0 || len(request.RequestedOperations) != 0 {
		return errors.New("delegation is not the closed package-publish profile")
	}
	if request.TargetKind != "publisher-generation" || request.SubjectKind != "publisher" || request.SubjectID != request.DelegatedPrincipal.ID || request.SubjectVersion == "" || request.SubjectDigest == "" || request.SubjectKeyDigest == "" || request.TargetIdentity != request.SubjectID || request.TargetDigest != request.SubjectDigest {
		return errors.New("package-publish delegation does not bind the exact PublisherGeneration")
	}
	if request.DelegatedPrincipal.Kind != "publisher" || request.DelegatedPrincipal.ID != "publisher:praxis-first-party" {
		return errors.New("package-publish delegation requires the first-party publisher principal")
	}
	scope, err := PackagePublishScope(request.TargetVersion)
	if err != nil || request.RequestedScope != scope || request.TargetConstraints == nil || len(request.TargetConstraints) != 1 || request.TargetConstraints[0] != request.TargetVersion {
		return errors.New("package-publish delegation scope is not the exact package namespace")
	}
	if request.PolicyVersion != AuthorityModelSuccessorVersion || request.PolicyRef != AuthorityModelID || request.PolicyDigest != AuthorityModelSuccessorDigest() {
		return errors.New("package-publish delegation must use the authority-model successor")
	}
	if parent.ParentRef != "" || parent.Principal.Kind != "human" || !strings.HasPrefix(parent.Principal.ID, "installation-owner:") || !strings.HasPrefix(parent.Scope, InstallationGovernanceScopePrefix) || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("package-publish parent is not the installation governance root")
	}
	if request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return errors.New("package-publish delegation expiry is missing or expired")
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
