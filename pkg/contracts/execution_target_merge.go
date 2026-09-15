package contracts

import (
	"fmt"
	"sort"
)

type TargetAuthority string

const (
	AuthorityPlatformSecurity TargetAuthority = "platform_security"
	AuthorityOrganization     TargetAuthority = "organization"
	AuthorityOperator         TargetAuthority = "operator"
	AuthorityPackage          TargetAuthority = "package"
	AuthorityAgent            TargetAuthority = "agent"
	AuthorityGraph            TargetAuthority = "graph"
	AuthorityNode             TargetAuthority = "node"
	AuthorityLearned          TargetAuthority = "learned"
)

type ExecutionTargetContribution struct {
	Authority    TargetAuthority `json:"authority"`
	SourceRef    string          `json:"source_ref"`
	SourceDigest string          `json:"source_digest"`
	Target       ExecutionTarget `json:"target"`
}

type EffectiveExecutionTarget struct {
	Target      ExecutionTarget   `json:"target"`
	Authorities []TargetAuthority `json:"authorities"`
}

var ErrTargetMergeConflict = fmt.Errorf("execution target contributions have no safe deterministic merge")

// MergeExecutionTargets applies a fixed authority-independent safety merge:
// restrictions intersect/union conservatively, while preferences never relax
// a prohibition. Input order is not semantic authority.
func MergeExecutionTargets(contributions []ExecutionTargetContribution) (EffectiveExecutionTarget, error) {
	if len(contributions) == 0 {
		return EffectiveExecutionTarget{}, fmt.Errorf("%w: no contributions", ErrTargetMergeConflict)
	}
	ordered := append([]ExecutionTargetContribution(nil), contributions...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return targetAuthorityRank(ordered[i].Authority) < targetAuthorityRank(ordered[j].Authority)
	})
	merged := ExecutionTarget{Version: "1", APIPolicy: APIPolicyAllow, SourceAuthority: "merged", Scope: ordered[0].Target.Scope}
	seenAuthorities := map[TargetAuthority]bool{}
	var transportSet []TransportClass
	transportConstrained := false
	apiForbid, apiRequired, apiPreferNot := false, false, false
	for _, contribution := range ordered {
		if targetAuthorityRank(contribution.Authority) < 0 || contribution.SourceRef == "" || contribution.SourceDigest == "" || contribution.Target.Scope != merged.Scope {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: contribution provenance, authority, or scope is invalid", ErrTargetMergeConflict)
		}
		if err := contribution.Target.Validate(); err != nil {
			return EffectiveExecutionTarget{}, err
		}
		if seenAuthorities[contribution.Authority] {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: duplicate authority", ErrTargetMergeConflict)
		}
		seenAuthorities[contribution.Authority] = true
		merged.RequiredCapabilities = unionStrings(merged.RequiredCapabilities, contribution.Target.RequiredCapabilities)
		merged.RequiredProfiles = unionStrings(merged.RequiredProfiles, contribution.Target.RequiredProfiles)
		merged.PreferredProfiles = unionStrings(merged.PreferredProfiles, contribution.Target.PreferredProfiles)
		merged.AllowedFallbackProfiles = unionStrings(merged.AllowedFallbackProfiles, contribution.Target.AllowedFallbackProfiles)
		merged.ProhibitedProfiles = unionStrings(merged.ProhibitedProfiles, contribution.Target.ProhibitedProfiles)
		merged.TelemetryRequirements = unionStrings(merged.TelemetryRequirements, contribution.Target.TelemetryRequirements)
		if contribution.Target.BudgetProfile != "" {
			merged.BudgetProfile = contribution.Target.BudgetProfile
		}
		if contribution.Target.ReasoningTier != "" {
			merged.ReasoningTier = contribution.Target.ReasoningTier
		}
		switch contribution.Target.APIPolicy {
		case APIPolicyForbid:
			apiForbid = true
		case APIPolicyRequired:
			apiRequired = true
		case APIPolicyPreferNot:
			apiPreferNot = true
		}
		if len(contribution.Target.TransportPolicy) > 0 {
			if !transportConstrained {
				transportSet = append([]TransportClass(nil), contribution.Target.TransportPolicy...)
				transportConstrained = true
			} else {
				transportSet = intersectTransports(transportSet, contribution.Target.TransportPolicy)
			}
		}
	}
	if apiForbid && apiRequired || len(transportSet) == 0 && transportConstrained {
		return EffectiveExecutionTarget{}, ErrTargetMergeConflict
	}
	if apiForbid {
		merged.APIPolicy = APIPolicyForbid
	} else if apiRequired {
		merged.APIPolicy = APIPolicyRequired
	} else if apiPreferNot {
		merged.APIPolicy = APIPolicyPreferNot
	}
	merged.TransportPolicy = transportSet
	merged.AllowedFallbackProfiles = subtractStrings(merged.AllowedFallbackProfiles, merged.ProhibitedProfiles)
	if err := merged.Validate(); err != nil {
		return EffectiveExecutionTarget{}, err
	}
	authorities := make([]TargetAuthority, 0, len(ordered))
	for _, contribution := range ordered {
		authorities = append(authorities, contribution.Authority)
	}
	return EffectiveExecutionTarget{Target: merged, Authorities: authorities}, nil
}

func targetAuthorityRank(authority TargetAuthority) int {
	switch authority {
	case AuthorityPlatformSecurity:
		return 0
	case AuthorityOrganization:
		return 1
	case AuthorityOperator:
		return 2
	case AuthorityPackage:
		return 3
	case AuthorityAgent:
		return 4
	case AuthorityGraph:
		return 5
	case AuthorityNode:
		return 6
	case AuthorityLearned:
		return 7
	default:
		return -1
	}
}

func unionStrings(left, right []string) []string {
	values := append([]string(nil), left...)
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range right {
		if value != "" && !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	sort.Strings(values)
	return values
}
func subtractStrings(values, prohibited []string) []string {
	out := []string{}
	for _, value := range values {
		blocked := false
		for _, deny := range prohibited {
			if value == deny {
				blocked = true
			}
		}
		if !blocked {
			out = append(out, value)
		}
	}
	return out
}
func intersectTransports(left, right []TransportClass) []TransportClass {
	out := []TransportClass{}
	for _, a := range left {
		for _, b := range right {
			if a == b {
				out = append(out, a)
			}
		}
	}
	return unionTransport(out)
}
func unionTransport(values []TransportClass) []TransportClass {
	seen := map[TransportClass]bool{}
	out := []TransportClass{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
