package contracts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestAuthorityModelV6IsExplicitlyConstructedFromV5 proves that v6 is not
// inherited by version order: its digest binds the exact v5 identity by
// value and enumerates every addition, and no earlier authority appears in
// v6 delegation merely because v6 follows v5.
func TestAuthorityModelV6IsExplicitlyConstructedFromV5(t *testing.T) {
	chain := []string{AuthorityModelDigest(), AuthorityModelSuccessorDigest(), AuthorityModelDeploymentDigest(), AuthorityModelGoalsPublicationDigest(), AuthorityModelGoalsRecoveryDigest(), AuthorityModelRoutingDigest()}
	for i := range chain {
		for j := range chain {
			if i != j && chain[i] == chain[j] {
				t.Fatalf("authority model digests v%d and v%d collide", i+1, j+1)
			}
		}
	}
	var members []string
	payload, _ := json.Marshal([]string{AuthorityModelID, AuthorityModelRoutingVersion, AuthorityModelGoalsRecoveryDigest(), AuthorityRoutingTargetContributionIssue, AuthorityRoutingSurfaceEligibilityIssue, DelegationProfileRoutingTargetContribution, DelegationProfileRoutingSurfaceEligibility, RoutingIssuanceOperation})
	_ = json.Unmarshal(payload, &members)
	if len(members) != 8 || members[2] != AuthorityModelGoalsRecoveryDigest() {
		t.Fatalf("v6 must retain the exact v5 identity by value and add exactly the routing rule set: %v", members)
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
