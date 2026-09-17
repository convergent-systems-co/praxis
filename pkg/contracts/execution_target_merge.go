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
	Target        ExecutionTarget               `json:"target"`
	Authorities   []TargetAuthority             `json:"authorities"`
	Contributions []ExecutionTargetContribution `json:"contributions,omitempty"`
}

var ErrTargetMergeConflict = fmt.Errorf("execution target contributions have no safe deterministic merge")

// MergeExecutionTargets merges contributions in descending authority order.
// Hard constraints only narrow as weaker contributions are applied.
func MergeExecutionTargets(contributions []ExecutionTargetContribution) (EffectiveExecutionTarget, error) {
	if len(contributions) == 0 {
		return EffectiveExecutionTarget{}, fmt.Errorf("%w: no contributions", ErrTargetMergeConflict)
	}

	ordered := make([]ExecutionTargetContribution, len(contributions))
	for index := range contributions {
		ordered[index] = cloneContribution(contributions[index])
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		return targetAuthorityRank(ordered[left].Authority) < targetAuthorityRank(ordered[right].Authority)
	})

	scope := ordered[0].Target.Scope
	seenAuthorities := make(map[TargetAuthority]struct{}, len(ordered))
	for _, contribution := range ordered {
		if targetAuthorityRank(contribution.Authority) < 0 {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: unknown authority %q", ErrTargetMergeConflict, contribution.Authority)
		}
		if _, exists := seenAuthorities[contribution.Authority]; exists {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: duplicate authority %q", ErrTargetMergeConflict, contribution.Authority)
		}
		seenAuthorities[contribution.Authority] = struct{}{}
		if contribution.SourceRef == "" || contribution.SourceDigest == "" {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: authority %q lacks source provenance", ErrTargetMergeConflict, contribution.Authority)
		}
		if err := contribution.Target.Validate(); err != nil {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: authority %q: %v", ErrTargetMergeConflict, contribution.Authority, err)
		}
		if contribution.Target.Scope != scope {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: scope mismatch %q and %q", ErrTargetMergeConflict, scope, contribution.Target.Scope)
		}
	}

	merged := ExecutionTarget{
		Version:         "1",
		APIPolicy:       APIPolicyAllow,
		SourceAuthority: "merged",
		Scope:           scope,
	}
	authorities := make([]TargetAuthority, 0, len(ordered))
	transportConstrained := false
	apiForbidden := false
	apiRequired := false
	apiPreferNot := false

	for index, contribution := range ordered {
		target := contribution.Target
		authorities = append(authorities, contribution.Authority)
		merged.RequiredCapabilities = appendUniqueStrings(merged.RequiredCapabilities, target.RequiredCapabilities)
		merged.RequiredProfiles = appendUniqueStrings(merged.RequiredProfiles, target.RequiredProfiles)
		merged.PreferredProfiles = appendUniqueStrings(merged.PreferredProfiles, target.PreferredProfiles)
		merged.ProhibitedProfiles = appendUniqueStrings(merged.ProhibitedProfiles, target.ProhibitedProfiles)
		merged.TelemetryRequirements = appendUniqueStrings(merged.TelemetryRequirements, target.TelemetryRequirements)

		if index == 0 {
			merged.AllowedFallbackProfiles = appendUniqueStrings(nil, target.AllowedFallbackProfiles)
		} else {
			merged.AllowedFallbackProfiles = intersectStrings(merged.AllowedFallbackProfiles, target.AllowedFallbackProfiles)
		}

		if len(target.TransportPolicy) > 0 {
			if !transportConstrained {
				merged.TransportPolicy = appendUniqueTransports(nil, target.TransportPolicy)
				transportConstrained = true
			} else {
				merged.TransportPolicy = intersectTransports(merged.TransportPolicy, target.TransportPolicy)
				if len(merged.TransportPolicy) == 0 {
					return EffectiveExecutionTarget{}, fmt.Errorf("%w: transport policies have no common transport", ErrTargetMergeConflict)
				}
			}
		}

		switch target.APIPolicy {
		case APIPolicyForbid:
			apiForbidden = true
		case APIPolicyRequired:
			apiRequired = true
		case APIPolicyPreferNot:
			apiPreferNot = true
		}

		if !mergeNonemptySingleton(&merged.ReasoningTier, target.ReasoningTier) {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: incompatible reasoning tiers %q and %q", ErrTargetMergeConflict, merged.ReasoningTier, target.ReasoningTier)
		}
		if !mergeNonemptySingleton(&merged.BudgetProfile, target.BudgetProfile) {
			return EffectiveExecutionTarget{}, fmt.Errorf("%w: incompatible budget profiles %q and %q", ErrTargetMergeConflict, merged.BudgetProfile, target.BudgetProfile)
		}
	}

	if apiForbidden && apiRequired {
		return EffectiveExecutionTarget{}, fmt.Errorf("%w: API use is both forbidden and required", ErrTargetMergeConflict)
	}
	switch {
	case apiForbidden:
		merged.APIPolicy = APIPolicyForbid
		if transportConstrained {
			merged.TransportPolicy = removeTransport(merged.TransportPolicy, TransportMeteredAPI)
			if len(merged.TransportPolicy) == 0 {
				return EffectiveExecutionTarget{}, fmt.Errorf("%w: API prohibition leaves no permitted transport", ErrTargetMergeConflict)
			}
		}
	case apiRequired:
		merged.APIPolicy = APIPolicyRequired
	case apiPreferNot:
		merged.APIPolicy = APIPolicyPreferNot
	default:
		merged.APIPolicy = APIPolicyAllow
	}

	merged.AllowedFallbackProfiles = subtractStrings(merged.AllowedFallbackProfiles, merged.ProhibitedProfiles)
	if err := merged.Validate(); err != nil {
		return EffectiveExecutionTarget{}, fmt.Errorf("%w: effective target: %v", ErrTargetMergeConflict, err)
	}

	return EffectiveExecutionTarget{
		Target:        merged,
		Authorities:   authorities,
		Contributions: ordered,
	}, nil
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

func cloneContribution(contribution ExecutionTargetContribution) ExecutionTargetContribution {
	contribution.Target = cloneExecutionTarget(contribution.Target)
	return contribution
}

func cloneExecutionTarget(target ExecutionTarget) ExecutionTarget {
	target.RequiredCapabilities = append([]string(nil), target.RequiredCapabilities...)
	target.RequiredProfiles = append([]string(nil), target.RequiredProfiles...)
	target.PreferredProfiles = append([]string(nil), target.PreferredProfiles...)
	target.AllowedFallbackProfiles = append([]string(nil), target.AllowedFallbackProfiles...)
	target.ProhibitedProfiles = append([]string(nil), target.ProhibitedProfiles...)
	target.TransportPolicy = append([]TransportClass(nil), target.TransportPolicy...)
	target.TelemetryRequirements = append([]string(nil), target.TelemetryRequirements...)
	return target
}

func mergeNonemptySingleton(current *string, next string) bool {
	if next == "" {
		return true
	}
	if *current == "" {
		*current = next
		return true
	}
	return *current == next
}

func appendUniqueStrings(left, right []string) []string {
	out := append([]string(nil), left...)
	seen := make(map[string]struct{}, len(out)+len(right))
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intersectStrings(left, right []string) []string {
	allowed := make(map[string]struct{}, len(right))
	for _, value := range right {
		allowed[value] = struct{}{}
	}
	out := make([]string, 0, len(left))
	for _, value := range left {
		if _, exists := allowed[value]; exists {
			out = append(out, value)
		}
	}
	return out
}

func subtractStrings(values, prohibited []string) []string {
	blocked := make(map[string]struct{}, len(prohibited))
	for _, value := range prohibited {
		blocked[value] = struct{}{}
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := blocked[value]; !exists {
			out = append(out, value)
		}
	}
	return out
}

func appendUniqueTransports(left, right []TransportClass) []TransportClass {
	out := append([]TransportClass(nil), left...)
	seen := make(map[TransportClass]struct{}, len(out)+len(right))
	for _, value := range out {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intersectTransports(left, right []TransportClass) []TransportClass {
	allowed := make(map[TransportClass]struct{}, len(right))
	for _, value := range right {
		allowed[value] = struct{}{}
	}
	out := make([]TransportClass, 0, len(left))
	for _, value := range left {
		if _, exists := allowed[value]; exists {
			out = append(out, value)
		}
	}
	return out
}

func removeTransport(values []TransportClass, prohibited TransportClass) []TransportClass {
	out := make([]TransportClass, 0, len(values))
	for _, value := range values {
		if value != prohibited {
			out = append(out, value)
		}
	}
	return out
}
