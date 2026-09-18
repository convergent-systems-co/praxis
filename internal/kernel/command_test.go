package kernel

import (
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestPreflightRequiresExactApprovalAndLease(t *testing.T) {
	now := time.Now().UTC()
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	intent := contracts.ActionIntent{Version: "v1", ID: "i1", Actor: actor, Operation: "write", Target: "file:a", Scope: "workspace:1", Parameters: map[string]string{"digest": "sha256:a"}}
	digest, err := intent.Digest()
	if err != nil { t.Fatal(err) }
	approval := contracts.ApprovalBinding{ID: "a1", Approver: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, IntentDigest: digest, IssuedAt: now, RemainingUses: 1}
	lease := contracts.CapabilityLease{ID: "l1", Principal: actor, Capability: "workspace.write", Operations: []string{"write"}, Scope: "workspace:1", IssuedAt: now}

	result, err := Preflight(PreflightRequest{Intent: intent, Approval: &approval, Lease: &lease, Capability: "workspace.write", Now: now, RequireApproval: true, RequireLease: true})
	if err != nil { t.Fatalf("expected preflight success: %v", err) }
	if result.IntentDigest != digest || result.ApprovalID != "a1" || result.LeaseID != "l1" {
		t.Fatalf("unexpected preflight result: %#v", result)
	}
}

func TestPreflightRejectsMutationAfterApproval(t *testing.T) {
	now := time.Now().UTC()
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	intent := contracts.ActionIntent{Version: "v1", ID: "i1", Actor: actor, Operation: "write", Target: "file:a", Scope: "workspace:1", Parameters: map[string]string{"digest": "sha256:a"}}
	digest, _ := intent.Digest()
	approval := contracts.ApprovalBinding{ID: "a1", Approver: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, IntentDigest: digest, IssuedAt: now, RemainingUses: 1}
	intent.Parameters["digest"] = "sha256:b"

	if _, err := Preflight(PreflightRequest{Intent: intent, Approval: &approval, Now: now, RequireApproval: true}); err == nil {
		t.Fatal("mutated intent must require new approval")
	}
}
