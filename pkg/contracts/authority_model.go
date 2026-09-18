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

	// GovernedInstallationRepairStorageSchema and
	// GovernedInstallationRepairRuntimeState are the two ADR-088-governed
	// installation-repair lifecycle authorities (the storage_schema and
	// runtime_state transitions respectively). ADR-088 §10 requires both
	// to be root-owner-only and non-delegable: no production path may
	// persist a child/delegated AuthorityGeneration carrying either
	// authority. Neither is a closed delegation profile — no
	// ValidateBuiltin*Delegation function exists for them, and none
	// should be added — so they are rejected by the delegation-profile
	// dispatch's default case exactly as any other unrecognized authority
	// string is, and independently rejected by the plaintext
	// non-delegability check in internal/state.Store's sole typed
	// AuthorityGeneration persistence boundary (the actual ADR-088 §10(b)
	// closure), which validates the exact plaintext before it can be sealed.
	GovernedInstallationRepairStorageSchema = "installation.repair.storage_schema"
	GovernedInstallationRepairRuntimeState  = "installation.repair.runtime_state"

	// Authority model v6 (ADR-092, ADR-094) is the global exact-dispatch and
	// executor-surface routing issuance successor of v3. Authority models
	// form a succession graph, not a linear chain: v1 -> v2 -> v3 are the
	// global installation models, v4 -> v5 are the installation-scoped Goals
	// publication branch off v3, and v6 is the global branch off v3. v6 is
	// adopted only through the canonical adoption ceremony from an active v3
	// state and adds exactly the two routing issuance authorities and their
	// closed delegation profiles. Nothing is inherited by version number:
	// AuthorityModelRoutingDigest binds the immutable v3 digest by value and
	// enumerates every addition, and the v6 tests prove the set.
	AuthorityModelRoutingVersion               = "v6"
	AuthorityRoutingTargetContributionIssue    = "routing.target-contribution.issue"
	AuthorityRoutingSurfaceEligibilityIssue    = "routing.surface-eligibility.issue"
	DelegationProfileRoutingTargetContribution = "ROUTING_TARGET_CONTRIBUTION"
	DelegationProfileRoutingSurfaceEligibility = "ROUTING_SURFACE_ELIGIBILITY"
	RoutingIssuanceOperation                   = "issue"
)

// AuthorityModelRoutingDigest identifies the immutable v6 rule set: the exact
// v3 identity retained by value plus the two routing issuance authorities,
// their delegation profiles, and the single "issue" operation. It binds no
// installation-scoped model (v4, v5) by design.
func AuthorityModelRoutingDigest() string {
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelRoutingVersion, AuthorityModelDeploymentDigest(), AuthorityRoutingTargetContributionIssue, AuthorityRoutingSurfaceEligibilityIssue, DelegationProfileRoutingTargetContribution, DelegationProfileRoutingSurfaceEligibility, RoutingIssuanceOperation})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// RoutingAuthorityForProfile maps a closed routing delegation profile to the
// single authority it may carry. Unknown profiles map to nothing.
func RoutingAuthorityForProfile(profile string) string {
	switch profile {
	case DelegationProfileRoutingTargetContribution:
		return AuthorityRoutingTargetContributionIssue
	case DelegationProfileRoutingSurfaceEligibility:
		return AuthorityRoutingSurfaceEligibilityIssue
	}
	return ""
}

// ValidateBuiltinRoutingDelegation is the closed v6 containment rule for a
// routing issuance child of the installation root. The parent must be the
// v1-labelled installation root (the active model is adopted state, not a
// root label), the policy must be exactly v6, the profile must name the one
// authority requested, the operation is "issue", no runtime capability may be
// requested, and the scope is the canonical exact target scope.
func ValidateBuiltinRoutingDelegation(parent AuthorityGeneration, request DelegationRequest, now time.Time) error {
	if request.PolicyRef != AuthorityModelID || request.PolicyVersion != AuthorityModelRoutingVersion || request.PolicyDigest != AuthorityModelRoutingDigest() {
		return errors.New("routing delegation requires authority model v6 policy")
	}
	if err := ValidateAuthorityModel(parent.AuthorityModel, parent.AuthorityModelVersion, parent.AuthorityModelDigest); err != nil {
		return fmt.Errorf("parent authority model: %w", err)
	}
	if parent.ParentRef != "" || parent.Principal.Kind != "human" || !strings.HasPrefix(parent.Principal.ID, "installation-owner:") || !strings.HasPrefix(parent.Scope, InstallationGovernanceScopePrefix) || !containsString(parent.Capabilities, AuthorityDelegateCapability) {
		return errors.New("routing delegation parent is not the installation governance root")
	}
	if request.ParentRef != parent.Ref || request.ParentVersion != parent.Version || request.ParentDigest != parent.Digest {
		return errors.New("routing delegation does not bind the exact parent generation")
	}
	if request.DelegatedPrincipal.Kind != "controller" || !strings.HasPrefix(request.DelegatedPrincipal.ID, "controller:") || request.DelegatedPrincipal == parent.Principal {
		return errors.New("routing delegation requires a distinct controller principal")
	}
	authority := RoutingAuthorityForProfile(request.Profile)
	if authority == "" || request.RequestedAuthority != authority || request.RequestedOperation != RoutingIssuanceOperation || len(request.RequestedCapabilities) != 0 || len(request.RequestedOperations) != 0 {
		return errors.New("routing delegation profile, authority, and operation must form one closed pair with no runtime capabilities")
	}
	wantKind, wantScope := "", ""
	switch authority {
	case AuthorityRoutingTargetContributionIssue:
		wantKind = "routing.target-contribution"
		wantScope, _ = RoutingTargetContributionScope(request.TargetIdentity, request.TargetVersion, request.TargetDigest)
	case AuthorityRoutingSurfaceEligibilityIssue:
		wantKind = "routing.surface-eligibility"
		if len(request.TargetConstraints) != 1 {
			return errors.New("surface eligibility delegation requires one canonical surface digest")
		}
		wantScope, _ = RoutingSurfaceEligibilityScope(request.TargetIdentity, request.TargetVersion, request.TargetDigest, request.TargetConstraints[0])
	}
	if wantScope == "" || request.TargetKind != wantKind || request.TargetIdentity == "" || request.TargetVersion == "" || request.RequestedScope != wantScope || !isSHA256Digest(request.TargetDigest) || !isSHA256Digest(request.ProposalDigest) || !isSHA256Digest(request.ReviewDigest) || request.ExpiresAt.IsZero() || !request.ExpiresAt.After(now) {
		return errors.New("routing delegation requires exact target, canonical scope, protected proposal and review digests, and future expiry")
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
	if id == AuthorityModelID && ((version == AuthorityModelGoalsPublicationVersion && digest == AuthorityModelGoalsPublicationDigest()) || (version == AuthorityModelGoalsRecoveryVersion && digest == AuthorityModelGoalsRecoveryDigest()) || (version == AuthorityModelRoutingVersion && digest == AuthorityModelRoutingDigest())) {
		return nil
	}
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
