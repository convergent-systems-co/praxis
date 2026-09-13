package kernel

import (
	"context"
	"testing"
)

type scriptedExecutor struct{ outcomes map[string]string }

func (s scriptedExecutor) ExecuteNode(_ context.Context, _ GraphDef, node NodeDef, _ *RunExecution) (NodeResult, error) {
	return NodeResult{Outcome: s.outcomes[node.ID], Evidence: []string{"evidence:" + node.ID}}, nil
}

func TestRunExecutesDeterministicFixture(t *testing.T) {
	g := GraphDef{ID: "g", Version: "1", EntryNode: "a", MaxTransitions: 4, Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}, {ID: "b", Class: NodeDeterministic}, {ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded}}, Transitions: []TransitionDef{{From: "a", Outcome: "next", To: "b"}, {From: "b", Outcome: "done", To: "done"}}}
	run := &RunExecution{RunID: "r1"}
	err := Run(context.Background(), g, run, scriptedExecutor{outcomes: map[string]string{"a": "next", "b": "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if run.State != RunSucceeded || run.CurrentNode != "done" || run.TransitionCount != 2 || len(run.Evidence) != 2 {
		t.Fatalf("unexpected run state: %+v", run)
	}
}

func TestRunStopsBoundedLoop(t *testing.T) {
	g := GraphDef{ID: "g", Version: "1", EntryNode: "a", MaxTransitions: 3, Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}}, Transitions: []TransitionDef{{From: "a", Outcome: "again", To: "a"}}}
	run := &RunExecution{RunID: "r1"}
	if err := Run(context.Background(), g, run, scriptedExecutor{outcomes: map[string]string{"a": "again"}}); err == nil {
		t.Fatal("bounded loop must terminate with error")
	}
	if run.State != RunFailed || run.TransitionCount != 3 {
		t.Fatalf("unexpected bounded loop state: %+v", run)
	}
}
