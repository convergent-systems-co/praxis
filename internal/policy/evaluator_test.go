package policy

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func policyRequest() Request {
	return Request{Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, Capability: "workspace.write", Operation: "write", Scope: "workspace:1:src"}
}

func TestHigherAuthorityDenyBeatsLowerAllow(t *testing.T) {
	rules := []Rule{
		{ID: "user-allow", Version: "1", AuthorityRank: 10, Effect: Allow, Capability: "workspace.write", Operation: "write", ScopePrefix: "workspace:1"},
		{ID: "org-deny", Version: "1", AuthorityRank: 20, Effect: Deny, Capability: "workspace.write", Operation: "write", ScopePrefix: "workspace:1", ReasonCode: "protected"},
	}
	d, err := Evaluate(rules, policyRequest())
	if err != nil { t.Fatal(err) }
	if d.Effect != Deny || d.RuleID != "org-deny" { t.Fatalf("unexpected decision: %+v", d) }
}

func TestDenyWinsAtEqualAuthority(t *testing.T) {
	rules := []Rule{
		{ID: "allow", Version: "1", AuthorityRank: 10, Effect: Allow, Capability: "workspace.write", Operation: "write"},
		{ID: "deny", Version: "1", AuthorityRank: 10, Effect: Deny, Capability: "workspace.write", Operation: "write"},
	}
	d, err := Evaluate(rules, policyRequest())
	if err != nil { t.Fatal(err) }
	if d.Effect != Deny { t.Fatalf("deny must win tie, got %+v", d) }
}

func TestNoMatchingRuleFailsClosed(t *testing.T) {
	d, err := Evaluate(nil, policyRequest())
	if err != nil { t.Fatal(err) }
	if d.Effect != Deny { t.Fatalf("no rule must deny, got %+v", d) }
}
