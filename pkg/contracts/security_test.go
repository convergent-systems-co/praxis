package contracts

import (
	"testing"
	"time"
)

func TestCryptoProfileValidation(t *testing.T) {
	for _, p := range []CryptoProfile{CryptoClassicalCompatible, CryptoPQPreferred, CryptoPQRequired, CryptoHybridHighAssurance} {
		if err := p.Validate(); err != nil {
			t.Fatalf("expected %q valid: %v", p, err)
		}
	}
	if err := CryptoProfile("made-up").Validate(); err == nil {
		t.Fatal("unknown crypto profile must fail closed")
	}
}

func TestCapabilityLeaseFailClosed(t *testing.T) {
	now := time.Now().UTC()
	expired := now.Add(-time.Second)
	lease := CapabilityLease{
		ID: "lease-1", Principal: PrincipalRef{ID: "plugin-1", Kind: "plugin"},
		Capability: "workspace.search.text", Scope: "workspace:1", ExpiresAt: &expired,
	}
	if err := lease.Validate(now); err == nil {
		t.Fatal("expired lease must be rejected")
	}
}

func TestApprovalRequiresExactBindingMode(t *testing.T) {
	now := time.Now().UTC()
	base := ApprovalBinding{
		ID: "approval-1", Approver: PrincipalRef{ID: "user-1", Kind: "human"},
		IssuedAt: now, RemainingUses: 1,
	}
	if err := base.Validate(now); err == nil {
		t.Fatal("approval without binding must fail")
	}
	base.IntentDigest = "sha256:abc"
	if err := base.Validate(now); err != nil {
		t.Fatalf("exact intent binding should validate: %v", err)
	}
	base.PolicyRef = "policy:1"
	if err := base.Validate(now); err == nil {
		t.Fatal("approval cannot bind both exact intent and policy")
	}
}

func TestActionIntentRequiresActorAndTarget(t *testing.T) {
	intent := ActionIntent{Version: "v1", ID: "a1", Actor: PrincipalRef{ID: "p1", Kind: "agent"}, Operation: "write", Target: "file:x", Scope: "workspace:1", CryptoProfile: CryptoPQPreferred}
	if err := intent.Validate(); err != nil {
		t.Fatalf("valid intent rejected: %v", err)
	}
	intent.Target = ""
	if err := intent.Validate(); err == nil {
		t.Fatal("intent without target must fail")
	}
}
