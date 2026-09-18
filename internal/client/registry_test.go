package client

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func testContract() contracts.InvocationContract {
	return contracts.InvocationContract{Version: "v1", PackageID: "p", PackageVersion: "1", GraphID: "g", GraphVersion: "1", EntryPointID: "entry", Aliases: []string{"develop"}, Options: []contracts.InvocationOption{{Name: "dashboard", Type: "bool", Default: "false"}}}
}

func TestRegistryAppliesDefaults(t *testing.T) {
	r, err := NewRegistry([]contracts.InvocationContract{testContract()})
	if err != nil { t.Fatal(err) }
	contract, values, err := r.Resolve(Invocation{EntryPoint: "develop", Options: map[string]string{}})
	if err != nil { t.Fatal(err) }
	if contract.EntryPointID != "entry" || values["dashboard"] != "false" { t.Fatalf("unexpected resolution: %+v %+v", contract, values) }
}

func TestRegistryRejectsUnknownOption(t *testing.T) {
	r, err := NewRegistry([]contracts.InvocationContract{testContract()})
	if err != nil { t.Fatal(err) }
	if _, _, err := r.Resolve(Invocation{EntryPoint: "develop", Options: map[string]string{"godmode": "true"}}); err == nil {
		t.Fatal("unknown option must not bypass contract validation")
	}
}
