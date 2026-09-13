package kernel

import "testing"

func TestGraphRejectsUnboundedCycle(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "a",
		Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}, {ID: "b", Class: NodeCondition}},
		Transitions: []TransitionDef{{From: "a", Outcome: "next", To: "b"}, {From: "b", Outcome: "again", To: "a"}},
	}
	if err := g.Validate(); err == nil {
		t.Fatal("unbounded execution cycle must be rejected")
	}
}

func TestGraphAllowsBoundedCycle(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "a", MaxTransitions: 10,
		Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}, {ID: "b", Class: NodeCondition}},
		Transitions: []TransitionDef{{From: "a", Outcome: "next", To: "b"}, {From: "b", Outcome: "again", To: "a"}},
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("bounded cycle rejected: %v", err)
	}
}

func TestGraphRejectsDuplicateOutcome(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "a",
		Nodes: []NodeDef{{ID: "a", Class: NodeCondition}, {ID: "b", Class: NodeTerminal}, {ID: "c", Class: NodeTerminal}},
		Transitions: []TransitionDef{{From: "a", Outcome: "ok", To: "b"}, {From: "a", Outcome: "ok", To: "c"}},
	}
	if err := g.Validate(); err == nil {
		t.Fatal("duplicate outcome must be rejected")
	}
}
