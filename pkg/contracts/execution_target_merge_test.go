package contracts

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func contribution(authority TargetAuthority, target ExecutionTarget) ExecutionTargetContribution {
	return ExecutionTargetContribution{Authority: authority, SourceRef: "source:" + string(authority), SourceDigest: "sha256:" + string(authority), Target: target}
}

func TestMergeExecutionTargetsStrongerEmptyFallbackCannotBeWidened(t *testing.T) {
	strong := validExecutionTarget()
	strong.SourceAuthority = "platform:security"
	strong.AllowedFallbackProfiles = nil
	weak := validExecutionTarget()
	weak.SourceAuthority = "node:planner"
	weak.AllowedFallbackProfiles = []string{"balanced", "economy"}

	effective, err := MergeExecutionTargets([]ExecutionTargetContribution{
		contribution(AuthorityNode, weak),
		contribution(AuthorityPlatformSecurity, strong),
	})
	if err != nil {
		t.Fatalf("MergeExecutionTargets() error = %v", err)
	}
	if len(effective.Target.AllowedFallbackProfiles) != 0 {
		t.Fatalf("allowed fallbacks = %v, want none", effective.Target.AllowedFallbackProfiles)
	}
	wantAuthorities := []TargetAuthority{AuthorityPlatformSecurity, AuthorityNode}
	if !reflect.DeepEqual(effective.Authorities, wantAuthorities) {
		t.Fatalf("authorities = %v, want %v", effective.Authorities, wantAuthorities)
	}
	if len(effective.Contributions) != 2 {
		t.Fatalf("contributions length = %d, want 2", len(effective.Contributions))
	}
	if got := effective.Contributions[0]; got.Authority != AuthorityPlatformSecurity || got.SourceRef != "source:platform_security" || got.SourceDigest != "sha256:platform_security" || got.Target.SourceAuthority != "platform:security" {
		t.Fatalf("strong contribution provenance not preserved: %#v", got)
	}
}

func TestMergeExecutionTargetsIntersectsFallbackAndPreservesProhibitions(t *testing.T) {
	strong := validExecutionTarget()
	strong.SourceAuthority = "platform:security"
	strong.RequiredCapabilities = []string{"reasoning", "trusted-runtime"}
	strong.AllowedFallbackProfiles = []string{"local", "balanced", "metered"}
	strong.ProhibitedProfiles = []string{"metered"}
	strong.TransportPolicy = []TransportClass{TransportSubscriptionCLI, TransportLocal}
	strong.APIPolicy = APIPolicyForbid

	weak := validExecutionTarget()
	weak.SourceAuthority = "node:planner"
	weak.RequiredCapabilities = []string{"reasoning", "tool-use"}
	weak.AllowedFallbackProfiles = []string{"balanced", "metered", "economy"}
	weak.ProhibitedProfiles = []string{"economy"}
	weak.TransportPolicy = []TransportClass{TransportMeteredAPI, TransportLocal, TransportSubscriptionCLI}
	weak.APIPolicy = APIPolicyAllow

	effective, err := MergeExecutionTargets([]ExecutionTargetContribution{
		contribution(AuthorityNode, weak),
		contribution(AuthorityPlatformSecurity, strong),
	})
	if err != nil {
		t.Fatalf("MergeExecutionTargets() error = %v", err)
	}
	if got, want := effective.Target.AllowedFallbackProfiles, []string{"balanced"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("allowed fallbacks = %v, want %v", got, want)
	}
	if got, want := effective.Target.RequiredCapabilities, []string{"reasoning", "trusted-runtime", "tool-use"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("required capabilities = %v, want %v", got, want)
	}
	if got, want := effective.Target.ProhibitedProfiles, []string{"metered", "economy"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prohibited profiles = %v, want %v", got, want)
	}
	if got, want := effective.Target.TransportPolicy, []TransportClass{TransportSubscriptionCLI, TransportLocal}; !reflect.DeepEqual(got, want) {
		t.Fatalf("transport policy = %v, want %v", got, want)
	}
	if effective.Target.APIPolicy != APIPolicyForbid {
		t.Fatalf("API policy = %q, want %q", effective.Target.APIPolicy, APIPolicyForbid)
	}
}

func TestMergeExecutionTargetsRetainsTierAndBudgetAndRejectsConflicts(t *testing.T) {
	strong := validExecutionTarget()
	strong.ReasoningTier = "D2"
	strong.BudgetProfile = "premium"
	weak := validExecutionTarget()
	weak.ReasoningTier = "D2"
	weak.BudgetProfile = "premium"

	effective, err := MergeExecutionTargets([]ExecutionTargetContribution{
		contribution(AuthorityNode, weak),
		contribution(AuthorityPlatformSecurity, strong),
	})
	if err != nil {
		t.Fatalf("MergeExecutionTargets() error = %v", err)
	}
	if effective.Target.ReasoningTier != "D2" || effective.Target.BudgetProfile != "premium" {
		t.Fatalf("tier/budget = %q/%q, want D2/premium", effective.Target.ReasoningTier, effective.Target.BudgetProfile)
	}

	weak.ReasoningTier = "D1"
	if _, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityPlatformSecurity, strong), contribution(AuthorityNode, weak)}); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("reasoning tier conflict error = %v, want ErrTargetMergeConflict", err)
	}

	weak.ReasoningTier = "D2"
	weak.BudgetProfile = "economy"
	if _, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityPlatformSecurity, strong), contribution(AuthorityNode, weak)}); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("budget profile conflict error = %v, want ErrTargetMergeConflict", err)
	}
}

func TestMergeExecutionTargetsLowerTierAndBudgetFillOnlySilence(t *testing.T) {
	strong := validExecutionTarget()
	strong.ReasoningTier = ""
	strong.BudgetProfile = ""
	weak := validExecutionTarget()
	weak.ReasoningTier = "D1"
	weak.BudgetProfile = "balanced"

	effective, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityPlatformSecurity, strong), contribution(AuthorityNode, weak)})
	if err != nil {
		t.Fatalf("MergeExecutionTargets() error = %v", err)
	}
	if effective.Target.ReasoningTier != "D1" || effective.Target.BudgetProfile != "balanced" {
		t.Fatalf("tier/budget = %q/%q, want D1/balanced", effective.Target.ReasoningTier, effective.Target.BudgetProfile)
	}
}

func TestMergeExecutionTargetsPreferenceOrderAndInputOrderDeterminism(t *testing.T) {
	platform := validExecutionTarget()
	platform.PreferredProfiles = []string{"quality", "balanced"}
	operator := validExecutionTarget()
	operator.PreferredProfiles = []string{"interactive", "balanced"}
	node := validExecutionTarget()
	node.PreferredProfiles = []string{"fast", "interactive"}

	first, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityNode, node), contribution(AuthorityPlatformSecurity, platform), contribution(AuthorityOperator, operator)})
	if err != nil {
		t.Fatalf("first MergeExecutionTargets() error = %v", err)
	}
	second, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityOperator, operator), contribution(AuthorityNode, node), contribution(AuthorityPlatformSecurity, platform)})
	if err != nil {
		t.Fatalf("second MergeExecutionTargets() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("merge depends on input order:\nfirst:  %#v\nsecond: %#v", first, second)
	}
	if got, want := first.Target.PreferredProfiles, []string{"quality", "balanced", "interactive", "fast"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("preferred profiles = %v, want %v", got, want)
	}
	wantAuthorities := []TargetAuthority{AuthorityPlatformSecurity, AuthorityOperator, AuthorityNode}
	if !reflect.DeepEqual(first.Authorities, wantAuthorities) {
		t.Fatalf("authorities = %v, want %v", first.Authorities, wantAuthorities)
	}
}

func TestMergeExecutionTargetsRejectsConflictingPoliciesAndUnknownAuthority(t *testing.T) {
	forbidden := validExecutionTarget()
	forbidden.APIPolicy = APIPolicyForbid
	required := validExecutionTarget()
	required.APIPolicy = APIPolicyRequired
	required.TransportPolicy = []TransportClass{TransportMeteredAPI}

	if _, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityPlatformSecurity, forbidden), contribution(AuthorityNode, required)}); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("API policy conflict error = %v, want ErrTargetMergeConflict", err)
	}

	unknown := contribution(TargetAuthority("unknown"), validExecutionTarget())
	if _, err := MergeExecutionTargets([]ExecutionTargetContribution{unknown}); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("unknown authority error = %v, want ErrTargetMergeConflict", err)
	}
}

func TestFreezeEffectiveExecutionTargetRecomputesExactOrderedProductAcrossReplay(t *testing.T) {
	platform := validExecutionTarget()
	platform.SourceAuthority = "platform:security"
	platform.PreferredProfiles = []string{"z-quality", "a-balanced"}
	platform.AllowedFallbackProfiles = []string{"z-local", "a-economy"}
	node := validExecutionTarget()
	node.SourceAuthority = "node:deliver"
	node.PreferredProfiles = []string{"b-interactive"}
	node.AllowedFallbackProfiles = []string{"a-economy", "z-local"}

	effective, err := MergeExecutionTargets([]ExecutionTargetContribution{
		contribution(AuthorityNode, node),
		contribution(AuthorityPlatformSecurity, platform),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(effective)
	if err != nil {
		t.Fatal(err)
	}
	var replayed EffectiveExecutionTarget
	if err := json.Unmarshal(payload, &replayed); err != nil {
		t.Fatal(err)
	}
	frozen, err := FreezeEffectiveExecutionTarget(replayed)
	if err != nil {
		t.Fatalf("FreezeEffectiveExecutionTarget() error = %v", err)
	}
	if !reflect.DeepEqual(frozen, effective) {
		t.Fatalf("recomputed target changed across replay:\ngot:  %#v\nwant: %#v", frozen, effective)
	}
	if got, want := frozen.Target.PreferredProfiles, []string{"z-quality", "a-balanced", "b-interactive"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("preferred profiles = %v, want semantic order %v", got, want)
	}
	if got, want := frozen.Target.AllowedFallbackProfiles, []string{"z-local", "a-economy"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fallback profiles = %v, want semantic order %v", got, want)
	}
}

func TestVerifyEffectiveExecutionTargetRejectsAssemblyMutationAndReordering(t *testing.T) {
	newEffective := func(t *testing.T) EffectiveExecutionTarget {
		t.Helper()
		strong := validExecutionTarget()
		strong.PreferredProfiles = []string{"z-quality", "a-balanced"}
		weak := validExecutionTarget()
		weak.PreferredProfiles = []string{"b-interactive"}
		effective, err := MergeExecutionTargets([]ExecutionTargetContribution{
			contribution(AuthorityNode, weak),
			contribution(AuthorityPlatformSecurity, strong),
		})
		if err != nil {
			t.Fatal(err)
		}
		return effective
	}

	manual := newEffective(t)
	manual.Contributions = nil
	if err := VerifyEffectiveExecutionTarget(manual); !errors.Is(err, ErrTargetMergeConflict) {
		t.Fatalf("manual target error = %v, want ErrTargetMergeConflict", err)
	}

	tests := []struct {
		name   string
		mutate func(*EffectiveExecutionTarget)
	}{
		{name: "merged profile order", mutate: func(e *EffectiveExecutionTarget) {
			e.Target.PreferredProfiles[0], e.Target.PreferredProfiles[1] = e.Target.PreferredProfiles[1], e.Target.PreferredProfiles[0]
		}},
		{name: "authority order", mutate: func(e *EffectiveExecutionTarget) {
			e.Authorities[0], e.Authorities[1] = e.Authorities[1], e.Authorities[0]
		}},
		{name: "contribution order", mutate: func(e *EffectiveExecutionTarget) {
			e.Contributions[0], e.Contributions[1] = e.Contributions[1], e.Contributions[0]
		}},
		{name: "contribution provenance", mutate: func(e *EffectiveExecutionTarget) {
			e.Contributions[0].SourceDigest = "sha256:forged"
		}},
		{name: "merged constraint", mutate: func(e *EffectiveExecutionTarget) {
			e.Target.RequiredCapabilities = append(e.Target.RequiredCapabilities, "forged")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			effective := newEffective(t)
			test.mutate(&effective)
			if err := VerifyEffectiveExecutionTarget(effective); !errors.Is(err, ErrTargetMergeConflict) {
				t.Fatalf("VerifyEffectiveExecutionTarget() error = %v, want ErrTargetMergeConflict", err)
			}
		})
	}
}

func TestFreezeEffectiveExecutionTargetReturnsDetachedCanonicalCopy(t *testing.T) {
	target := validExecutionTarget()
	target.PreferredProfiles = []string{"quality"}
	effective, err := MergeExecutionTargets([]ExecutionTargetContribution{contribution(AuthorityNode, target)})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := FreezeEffectiveExecutionTarget(effective)
	if err != nil {
		t.Fatal(err)
	}
	effective.Target.PreferredProfiles[0] = "mutated"
	effective.Contributions[0].Target.PreferredProfiles[0] = "mutated"
	if got := frozen.Target.PreferredProfiles[0]; got != "quality" {
		t.Fatalf("frozen merged target aliased caller memory: %q", got)
	}
	if got := frozen.Contributions[0].Target.PreferredProfiles[0]; got != "quality" {
		t.Fatalf("frozen contribution aliased caller memory: %q", got)
	}
}
