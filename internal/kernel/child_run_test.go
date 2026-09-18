package kernel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/scheduler"
)

func TestDeriveChildAuthorityCanOnlyNarrow(t *testing.T) {
	parent := RunAuthority{
		Scope: "workspace:*",
		Capabilities: []CapabilityGrant{
			{Capability: "workspace.file", Operations: []string{"read", "write"}, Scope: "workspace:*"},
			{Capability: "network.fetch", Operations: []string{"get"}, Scope: "network:github:*"},
		},
		Quota: scheduler.Quota{MaxActiveSlices: 8, MaxAttempts: 20, MaxTokens: 100000, MaxCostMicros: 5000000},
		Depth: 0, MaxDepth: 3,
	}
	child, err := DeriveChildAuthority(parent, ChildRequirements{
		Scope: "workspace:repo-a",
		Capabilities: []CapabilityGrant{{Capability: "workspace.file", Operations: []string{"read"}, Scope: "workspace:repo-a"}},
		Quota: scheduler.Quota{MaxActiveSlices: 2, MaxAttempts: 5, MaxTokens: 12000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Scope != "workspace:repo-a" || child.Depth != 1 || child.Quota.MaxActiveSlices != 2 || child.Quota.MaxAttempts != 5 || child.Quota.MaxTokens != 12000 || child.Quota.MaxCostMicros != 5000000 {
		t.Fatalf("unexpected child authority: %+v", child)
	}
	if len(child.Capabilities) != 1 || len(child.Capabilities[0].Operations) != 1 || child.Capabilities[0].Operations[0] != "read" {
		t.Fatalf("unexpected child capabilities: %+v", child.Capabilities)
	}
}

func TestDeriveChildAuthorityRejectsExpansion(t *testing.T) {
	parent := RunAuthority{
		Scope: "workspace:repo-a",
		Capabilities: []CapabilityGrant{{Capability: "workspace.file", Operations: []string{"read"}, Scope: "workspace:repo-a"}},
		Quota: scheduler.Quota{MaxTokens: 1000}, Depth: 1, MaxDepth: 2,
	}
	cases := []ChildRequirements{
		{Scope: "workspace:*"},
		{Scope: "workspace:repo-a", Capabilities: []CapabilityGrant{{Capability: "workspace.file", Operations: []string{"write"}, Scope: "workspace:repo-a"}}},
		{Scope: "workspace:repo-a", Capabilities: []CapabilityGrant{{Capability: "network.fetch", Operations: []string{"get"}, Scope: "network:*"}}},
	}
	for i, request := range cases {
		if _, err := DeriveChildAuthority(parent, request); err == nil {
			t.Fatalf("case %d expanded parent authority", i)
		}
	}
}

func TestDeriveChildAuthorityEnforcesDepth(t *testing.T) {
	parent := RunAuthority{Scope: "workspace:*", Depth: 2, MaxDepth: 2}
	if _, err := DeriveChildAuthority(parent, ChildRequirements{Scope: "workspace:repo-a"}); err == nil {
		t.Fatal("child beyond max depth must be rejected")
	}
}

func TestRunTreeParentCancellationPropagatesToDescendants(t *testing.T) {
	tree := NewRunTree()
	rootCtx, err := tree.RegisterRoot(context.Background(), "root")
	if err != nil {
		t.Fatal(err)
	}
	childCtx, err := tree.RegisterChild("root", "child")
	if err != nil {
		t.Fatal(err)
	}
	grandchildCtx, err := tree.RegisterChild("child", "grandchild")
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Cancel("root"); err != nil {
		t.Fatal(err)
	}
	for name, ctx := range map[string]context.Context{"root": rootCtx, "child": childCtx, "grandchild": grandchildCtx} {
		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatalf("%s: expected cancelled context, got %v", name, ctx.Err())
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("%s: cancellation did not propagate", name)
		}
	}
}

func TestRunTreeCannotRegisterChildUnderUnknownParent(t *testing.T) {
	tree := NewRunTree()
	if _, err := tree.RegisterChild("missing", "child"); err == nil {
		t.Fatal("unknown parent must fail")
	}
}
