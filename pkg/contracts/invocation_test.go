package contracts

import (
	"errors"
	"testing"
)

func TestInvocationContractUsesOwningCompatibilityPolicy(t *testing.T) {
	base := InvocationContract{Version: InvocationContractCurrentVersion(), PackageID: "research/pkg", PackageVersion: "1", GraphID: "research.graph", GraphVersion: "1", EntryPointID: "research.run", Aliases: []string{"research"}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	// Both forms existed in package-owned contracts before compatibility
	// ownership was centralized. The established numeric form remains readable
	// under identical semantics while v1 is the sole current write version.
	base.Version = "1"
	if err := base.Validate(); err != nil {
		t.Fatalf("supported historical invocation schema rejected: %v", err)
	}
	base.Version = "future-unknown"
	if err := base.Validate(); !errors.Is(err, ErrUnknownContractVersion) {
		t.Fatalf("unknown invocation schema did not fail through owning contract policy: %v", err)
	}
	owned, ok := clientContractCatalog.Lookup(invocationContractPolicy.Contract)
	if !ok || owned != invocationContractVersions {
		t.Fatal("invocation compatibility consumer is not bound to its canonical definition")
	}
}
