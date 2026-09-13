package kernel

import "testing"

func checkpointGraph() GraphDef {
	return GraphDef{ID: "g1", Version: "v1", EntryNode: "a", MaxTransitions: 5, Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}, {ID: "done", Class: NodeTerminal}}, Transitions: []TransitionDef{{From: "a", Outcome: "ok", To: "done"}}}
}

func TestCheckpointRunningResumesWaiting(t *testing.T) {
	c := Checkpoint{RunID: "r1", GraphID: "g1", GraphVersion: "v1", CurrentNode: "a", RunState: RunRunning, TransitionCount: 1}
	if err := c.Validate(checkpointGraph()); err != nil { t.Fatal(err) }
	if got := c.ResumeState(); got != RunWaiting { t.Fatalf("expected waiting resume, got %s", got) }
}

func TestCheckpointCancellingWithEffectReconciles(t *testing.T) {
	c := Checkpoint{RunID: "r1", GraphID: "g1", GraphVersion: "v1", CurrentNode: "a", RunState: RunCancelling, OutstandingEffects: []string{"effect-1"}}
	if got := c.ResumeState(); got != RunReconciling { t.Fatalf("expected reconciling, got %s", got) }
}

func TestCheckpointRejectsVersionMismatch(t *testing.T) {
	c := Checkpoint{RunID: "r1", GraphID: "g1", GraphVersion: "v2", CurrentNode: "a", RunState: RunWaiting}
	if err := c.Validate(checkpointGraph()); err == nil { t.Fatal("graph version mismatch must fail") }
}
