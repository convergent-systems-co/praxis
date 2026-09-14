package clientadapt

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestAdaptersPreserveInvocationSemanticsAndDegradeOptionalAffordances(t *testing.T) {
	contract := contracts.InvocationContract{Version: "v1", PackageID: "package/develop", PackageVersion: "1", GraphID: "graph/develop", GraphVersion: "2", EntryPointID: "develop", Aliases: []string{"develop"}, OptionalCapabilities: []string{"presentation.dashboard"}}
	rich, err := BuildPlan(contract, Surface{ID: "rich-client", Affordances: map[string]bool{"presentation.dashboard": true}})
	if err != nil {
		t.Fatal(err)
	}
	minimal, err := BuildPlan(contract, Surface{ID: "minimal-client", Affordances: map[string]bool{}})
	if err != nil {
		t.Fatal(err)
	}
	if rich.PackageID != minimal.PackageID || rich.GraphID != minimal.GraphID || rich.GraphVersion != minimal.GraphVersion || rich.EntryPointID != minimal.EntryPointID {
		t.Fatalf("adapters changed canonical invocation semantics: rich=%+v minimal=%+v", rich, minimal)
	}
	if len(rich.OmittedOptional) != 0 || len(minimal.OmittedOptional) != 1 || minimal.OmittedOptional[0] != "presentation.dashboard" {
		t.Fatalf("optional affordance did not degrade explicitly: rich=%+v minimal=%+v", rich, minimal)
	}
	if _, err := BuildPlan(contract, Surface{}); err == nil {
		t.Fatal("adapter without identity must fail closed")
	}
}
