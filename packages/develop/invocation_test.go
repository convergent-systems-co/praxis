package develop

import "testing"

func TestInvocationContractMatchesGraph(t *testing.T) {
	contract := InvocationContract()
	if err := contract.Validate(); err != nil { t.Fatal(err) }
	g := Graph()
	if contract.GraphID != g.ID || contract.GraphVersion != g.Version || contract.EntryPointID != g.ID {
		t.Fatalf("invocation contract drifted from graph: contract=%+v graph=%+v", contract, g)
	}
}

func TestDashboardIsOptionalCapability(t *testing.T) {
	contract := InvocationContract()
	found := false
	for _, capability := range contract.OptionalCapabilities {
		if capability == "presentation.dashboard" { found = true }
	}
	if !found { t.Fatal("dashboard must remain optional presentation capability") }
}
