package policy

import (
	"errors"
	"fmt"
	"sort"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type Effect string

const (
	Allow           Effect = "allow"
	Deny            Effect = "deny"
	RequireApproval Effect = "require_approval"
)

type Rule struct {
	ID                string
	Version           string
	AuthorityRank     int
	Effect            Effect
	ActorKind         string
	Capability        string
	Operation         string
	ScopePrefix       string
	RequiredApproval  string
	RequiredEnforcement []string
	ReasonCode        string
}

type Request struct {
	Actor      contracts.PrincipalRef
	Capability string
	Operation  string
	Scope      string
}

type Decision struct {
	Effect              Effect
	RuleID              string
	RuleVersion         string
	ReasonCode          string
	RequiredApproval    string
	RequiredEnforcement []string
}

func Evaluate(rules []Rule, req Request) (Decision, error) {
	if err := req.Actor.Validate(); err != nil {
		return Decision{}, err
	}
	if req.Capability == "" || req.Operation == "" || req.Scope == "" {
		return Decision{}, errors.New("policy capability, operation, and scope are required")
	}
	matched := make([]Rule, 0)
	for _, rule := range rules {
		if rule.ID == "" || rule.Version == "" || rule.AuthorityRank < 0 {
			return Decision{}, errors.New("invalid policy rule identity/version/authority")
		}
		if rule.ActorKind != "" && rule.ActorKind != req.Actor.Kind { continue }
		if rule.Capability != "" && rule.Capability != req.Capability { continue }
		if rule.Operation != "" && rule.Operation != req.Operation { continue }
		if rule.ScopePrefix != "" && !hasScopePrefix(req.Scope, rule.ScopePrefix) { continue }
		matched = append(matched, rule)
	}
	if len(matched) == 0 {
		return Decision{Effect: Deny, ReasonCode: "no_matching_allow"}, nil
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].AuthorityRank != matched[j].AuthorityRank {
			return matched[i].AuthorityRank > matched[j].AuthorityRank
		}
		return effectRank(matched[i].Effect) > effectRank(matched[j].Effect)
	})
	winner := matched[0]
	if winner.Effect != Allow && winner.Effect != Deny && winner.Effect != RequireApproval {
		return Decision{}, fmt.Errorf("unknown policy effect %q", winner.Effect)
	}
	return Decision{Effect: winner.Effect, RuleID: winner.ID, RuleVersion: winner.Version, ReasonCode: winner.ReasonCode, RequiredApproval: winner.RequiredApproval, RequiredEnforcement: append([]string(nil), winner.RequiredEnforcement...)}, nil
}

func effectRank(e Effect) int {
	switch e {
	case Deny: return 3
	case RequireApproval: return 2
	case Allow: return 1
	default: return 0
	}
}

func hasScopePrefix(scope, prefix string) bool {
	if scope == prefix { return true }
	if len(scope) <= len(prefix) || scope[:len(prefix)] != prefix { return false }
	return scope[len(prefix)] == ':'
}
