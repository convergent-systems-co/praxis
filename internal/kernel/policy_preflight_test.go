package kernel

import (
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/policy"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPolicyDenyOverridesApprovalAndLease(t *testing.T) {
	now := time.Now().UTC()
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	intent := contracts.ActionIntent{Version: "v1", ID: "i1", Actor: actor, Operation: "write", Target: "file:a", Scope: "workspace:1"}
	digest, err := intent.Digest()
	if err != nil { t.Fatal(err) }
	approval := contracts.ApprovalBinding{ID: "a1", Approver: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, IntentDigest: digest, IssuedAt: now, RemainingUses: 1}
	lease := contracts.CapabilityLease{ID: "l1", Principal: actor, Capability: "workspace.write", Operations: []string{"write"}, Scope: "workspace:1", IssuedAt: now}
	rules := []policy.Rule{{ID: "deny", Version: "1", AuthorityRank: 100, Effect: policy.Deny, Capability: "workspace.write", Operation: "write", ScopePrefix: "workspace:1", ReasonCode: "protected"}}

	_, err = Preflight(PreflightRequest{Intent: intent, Approval: &approval, Lease: &lease, PolicyRules: rules, Capability: "workspace.write", Now: now, RequireApproval: true, RequireLease: true, RequirePolicy: true})
	if err == nil { t.Fatal("hard policy deny must override valid approval and lease") }
}

func TestPolicyRequireApprovalUsesExactBinding(t *testing.T) {
	now := time.Now().UTC()
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	intent := contracts.ActionIntent{Version: "v1", ID: "i1", Actor: actor, Operation: "write", Target: "file:a", Scope: "workspace:1"}
	rules := []policy.Rule{{ID: "approve", Version: "1", AuthorityRank: 10, Effect: policy.RequireApproval, Capability: "workspace.write", Operation: "write", ScopePrefix: "workspace:1"}}
	if _, err := Preflight(PreflightRequest{Intent: intent, PolicyRules: rules, Capability: "workspace.write", Now: now, RequirePolicy: true}); err == nil {
		t.Fatal("policy-required approval must block without binding")
	}
}
