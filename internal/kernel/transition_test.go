package kernel

import "testing"

func TestResolveTransition(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "start",
		Nodes: []NodeDef{{ID: "start", Class: NodeCondition}, {ID: "done", Class: NodeTerminal}},
		Transitions: []TransitionDef{{From: "start", Outcome: "ok", To: "done"}},
	}
	to, err := ResolveTransition(g, "start", "ok")
	if err != nil { t.Fatal(err) }
	if to != "done" { t.Fatalf("expected done, got %s", to) }
}

func TestResolveTransitionRejectsUnknownOutcome(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "start",
		Nodes: []NodeDef{{ID: "start", Class: NodeCondition}, {ID: "done", Class: NodeTerminal}},
		Transitions: []TransitionDef{{From: "start", Outcome: "ok", To: "done"}},
	}
	if _, err := ResolveTransition(g, "start", "bad"); err == nil {
		t.Fatal("unknown outcome must be rejected")
	}
}
