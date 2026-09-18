package contracts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAuthorityModelV6IsExplicitlyConstructedFromV3 proves that v6 is not
// inherited by version number: its digest binds the exact v3 identity by
// value, binds neither installation-scoped Goals model, and enumerates every
// addition, so no authority appears in v6 merely because 6 > 5.
func TestAuthorityModelV6IsExplicitlyConstructedFromV3(t *testing.T) {
	chain := []string{AuthorityModelDigest(), AuthorityModelSuccessorDigest(), AuthorityModelDeploymentDigest(), AuthorityModelGoalsPublicationDigest(), AuthorityModelGoalsRecoveryDigest(), AuthorityModelRoutingDigest()}
	for i := range chain {
		for j := range chain {
			if i != j && chain[i] == chain[j] {
				t.Fatalf("authority model digests v%d and v%d collide", i+1, j+1)
			}
		}
	}
	var members []string
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelRoutingVersion, AuthorityModelDeploymentDigest(), AuthorityRoutingTargetContributionIssue, AuthorityRoutingSurfaceEligibilityIssue, DelegationProfileRoutingTargetContribution, DelegationProfileRoutingSurfaceEligibility, RoutingIssuanceOperation})
	_ = json.Unmarshal(payload, &members)
	if len(members) != 8 || members[2] != AuthorityModelDeploymentDigest() {
		t.Fatalf("v6 must retain the exact v3 identity by value and add exactly the routing rule set: %v", members)
	}
	// Pinned identities: v4 and v5 are unchanged by the topology decision and
	// v6 binds v3 deterministically. Any drift here is a durable-identity change.
	pinned := map[string]string{
		"v4": "sha256:3f835c18b65cf1eeea06f58ac8c066698a067eb370c969e394de72f2bd61988d",
		"v5": "sha256:fb63093de7b4f271d8b8ffb7229572d7887d5f6af7dd89191ef5283eb90fb53c",
	}
	if AuthorityModelGoalsPublicationDigest() != pinned["v4"] || AuthorityModelGoalsRecoveryDigest() != pinned["v5"] {
		t.Fatal("installation-scoped Goals model digests must remain unchanged")
	}
	if AuthorityModelRoutingDigest() == "sha256:e3a8bb27da1247921dae7c8f887e91741b06d923d5aff059d9fe0d33e1a67ed3" {
		t.Fatal("v6 must no longer bind the v5 identity")
	}
	for _, forbidden := range []string{AuthorityModelGoalsPublicationDigest(), AuthorityModelGoalsRecoveryDigest(), GoalsPublicationProfile, GoalsPublicationRecoveryProfile, GoalsPublicationOperation, GovernedInstallationRepairStorageSchema, GovernedInstallationRepairRuntimeState} {
		if strings.Contains(string(payload), forbidden) {
			t.Fatalf("unrelated authority %q appears in the v6 rule set", forbidden)
		}
	}
	if err := ValidateAuthorityModel(AuthorityModelID, AuthorityModelRoutingVersion, AuthorityModelRoutingDigest()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAuthorityModel(AuthorityModelID, AuthorityModelRoutingVersion, AuthorityModelGoalsRecoveryDigest()); err == nil {
		t.Fatal("v6 must not validate with the v5 digest")
	}
	if err := ValidateAuthorityModel(AuthorityModelID, AuthorityModelGoalsRecoveryVersion, AuthorityModelRoutingDigest()); err == nil {
		t.Fatal("v5 must not validate with the v6 digest")
	}
	// v6 adds exactly two authorities and two profiles; every other
	// governed authority stays bound to its own model and profile.
	if RoutingAuthorityForProfile(DelegationProfileRoutingTargetContribution) != AuthorityRoutingTargetContributionIssue || RoutingAuthorityForProfile(DelegationProfileRoutingSurfaceEligibility) != AuthorityRoutingSurfaceEligibilityIssue {
		t.Fatal("routing profiles must map to their single authority")
	}
	for _, profile := range []string{DelegationProfileWorkPlanAccept, DelegationProfilePackagePublish, DelegationProfilePackageDeploy, GoalsPublicationProfile, GoalsPublicationRecoveryProfile, ""} {
		if RoutingAuthorityForProfile(profile) != "" {
			t.Fatalf("profile %q must not acquire routing authority", profile)
		}
	}
}

func routingDelegationFixture(t *testing.T) (AuthorityGeneration, DelegationRequest, time.Time) {
	t.Helper()
	parent, request, now := validBuiltinDelegation(t)
	target := "sha256:" + strings.Repeat("7", 64)
	request.Profile = DelegationProfileRoutingTargetContribution
	request.PolicyRef, request.PolicyVersion, request.PolicyDigest = AuthorityModelID, AuthorityModelRoutingVersion, AuthorityModelRoutingDigest()
	request.RequestedAuthority, request.RequestedOperation = AuthorityRoutingTargetContributionIssue, RoutingIssuanceOperation
	request.TargetKind, request.TargetIdentity, request.TargetVersion, request.TargetDigest = "routing.target-contribution", "target:weather", "1", target
	request.TargetConstraints = nil
	request.RequestedScope, _ = RoutingTargetContributionScope("target:weather", "1", target)
	return parent, request, now
}

func TestRoutingDelegationIsClosedToV6PolicyAndRoutingAuthorityOnly(t *testing.T) {
	parent, request, now := routingDelegationFixture(t)
	if err := ValidateBuiltinRoutingDelegation(parent, request, now); err != nil {
		t.Fatal(err)
	}
	drift := request
	drift.PolicyVersion, drift.PolicyDigest = AuthorityModelGoalsRecoveryVersion, AuthorityModelGoalsRecoveryDigest()
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("routing delegation accepted a v5 policy")
	}
	drift = request
	drift.RequestedAuthority = AuthorityDelegateCapability
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("routing child received authority.delegate")
	}
	drift = request
	drift.RequestedAuthority = GovernedPackageDeploy
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("routing profile carried a package authority")
	}
	drift = request
	drift.Profile = DelegationProfileRoutingSurfaceEligibility
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("profile and authority must form one closed pair")
	}
	drift = request
	drift.RequestedCapabilities = []string{"runtime.exec"}
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("routing delegation accepted runtime capabilities")
	}
	drift = request
	drift.RequestedScope = "routing-target/other/1/" + strings.Repeat("7", 64)
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("routing delegation accepted a non-canonical scope")
	}
	drift = request
	drift.ParentDigest = "sha256:" + strings.Repeat("0", 64)
	if err := ValidateBuiltinRoutingDelegation(parent, drift, now); err == nil {
		t.Fatal("routing delegation accepted a substituted parent")
	}
	// The v1 WorkPlan rule never admits routing authority.
	if err := ValidateBuiltinDelegation(parent, request, now); err == nil {
		t.Fatal("v1 delegation admitted a routing authority")
	}
}
