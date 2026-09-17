package contracts

import (
	"errors"
	"testing"
)

func contribution(authority TargetAuthority, target ExecutionTarget) ExecutionTargetContribution {
	return ExecutionTargetContribution{Authority: authority, SourceRef: "docs/SPEC/039-execution-targets-affinity-and-budget-routing.md", SourceDigest: "sha256:target-source", Target: target}
}

func TestMergeTargetsPreservesStrongerProhibitionAndDeterministicOrder(t *testing.T) {
	platform := validExecutionTarget()
	platform.ProhibitedProfiles = []string{"metered"}
	platform.AllowedFallbackProfiles = []string{"local", "metered"}
	platform.TransportPolicy = []TransportClass{TransportLocal, TransportSubscriptionCLI}
	platform.SourceAuthority = "platform"
	node := validExecutionTarget()
	node.APIPolicy = APIPolicyPreferNot
	node.TransportPolicy = []TransportClass{TransportLocal, TransportSubscriptionCLI}
	node.SourceAuthority = "node"
	merged, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityNode, node), contribution(AuthorityPlatformSecurity, platform)})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Target.APIPolicy != APIPolicyForbid || len(merged.Target.AllowedFallbackProfiles) != 2 || merged.Target.AllowedFallbackProfiles[0] == "metered" || merged.Target.AllowedFallbackProfiles[1] == "metered" {
		t.Fatalf("lower authority relaxed stronger policy: %+v", merged)
	}
	if merged.Authorities[0] != AuthorityPlatformSecurity || merged.Authorities[1] != AuthorityNode {
		t.Fatalf("authority order was not deterministic: %v", merged.Authorities)
	}
}

func TestMergeTargetsRejectsConflictingHardPoliciesAndUnknownAuthority(t *testing.T) {
	forbid := validExecutionTarget()
	forbid.SourceAuthority = "platform"
	required := validExecutionTarget()
	required.SourceAuthority = "operator"
	required.APIPolicy = APIPolicyRequired
	required.TransportPolicy = []TransportClass{TransportMeteredAPI}
	if _, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityPlatformSecurity, forbid), contribution(AuthorityOperator, required)}); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("conflicting API policies were accepted: %v", err)
	}
	if _, err := MergeExecutionTargets([]ExecutionTargetContribution{{Authority: "model", SourceRef: "model", SourceDigest: "sha256:model", Target: validExecutionTarget()}}); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("unknown authority was accepted: %v", err)
	}
}
