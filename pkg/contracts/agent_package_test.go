package contracts

import (
	"errors"
	"testing"
)

func TestPackageAgentDefinitionCompatibilityIsContractOwned(t *testing.T) {
	definition := PackageAgentDefinition{Version: PackageAgentDefinitionCurrentVersion(), Graphs: []PackageGraphBinding{{ID: "research.graph", Version: "1"}}}
	if err := definition.Validate(); err != nil {
		t.Fatal(err)
	}
	definition.Version = "future"
	if err := definition.Validate(); !errors.Is(err, ErrUnknownContractVersion) {
		t.Fatalf("unknown package definition contract did not fail closed: %v", err)
	}
	owned, ok := agentContractCatalog.Lookup(packageAgentDefinitionPolicy.Contract)
	if !ok || owned != packageAgentDefinitionVersions {
		t.Fatal("agent package definition is not bound to its canonical contract policy")
	}
}
