package kernel

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type runControlExecutor struct{}

func (runControlExecutor) ExecuteNode(_ context.Context, _ GraphDef, node NodeDef, _ *RunExecution) (NodeResult, error) {
	if node.ID == "work" {
		return NodeResult{Outcome: "done", Evidence: []string{"work-complete"}}, nil
	}
	return NodeResult{Outcome: "done"}, nil
}

func TestRunControlStatusAndCancelReplayAuthoritativeState(t *testing.T) {
	ctx := context.Background()
	store := eventstore.NewMemoryStore()
	actor := contracts.PrincipalRef{ID: "user-1", Kind: "user"}
	journal := &EventJournal{Store: store, Actor: actor, CommandID: "seed", CorrelationID: "corr"}
	run := &RunExecution{RunID: "run-1", GraphID: "graph-1", GraphVersion: "1", CurrentNode: "work", State: RunRunning, AttemptCounts: map[string]int{}}
	if err := journal.ObserveRun(ctx, baseObservation(run, ObservationRunStarted, "work")); err != nil {
		t.Fatal(err)
	}

	control := RunControl{Store: store, Actor: actor}
	status, err := control.Status(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Run.State != RunRunning || status.AggregateVersion != 1 {
		t.Fatalf("unexpected status: state=%s version=%d", status.Run.State, status.AggregateVersion)
	}

	cancelled, err := control.Cancel(ctx, "run-1", "cancel-1", "corr")
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Run.State != RunCancelled || cancelled.AggregateVersion != 2 {
		t.Fatalf("unexpected cancellation result: state=%s version=%d", cancelled.Run.State, cancelled.AggregateVersion)
	}

	replayed, err := control.Status(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.State != RunCancelled || replayed.AggregateVersion != 2 {
		t.Fatalf("cancellation was not durable: state=%s version=%d", replayed.Run.State, replayed.AggregateVersion)
	}
	if _, err := control.Cancel(ctx, "run-1", "cancel-2", "corr"); err == nil {
		t.Fatal("expected terminal run cancellation to fail")
	}
}

func TestRunControlResumeRequiresExactPersistedWaitAndContinues(t *testing.T) {
	ctx := context.Background()
	store := eventstore.NewMemoryStore()
	actor := contracts.PrincipalRef{ID: "user-1", Kind: "user"}
	journal := &EventJournal{Store: store, Actor: actor, CommandID: "seed", CorrelationID: "corr"}
	run := &RunExecution{RunID: "run-2", GraphID: "graph-2", GraphVersion: "1", CurrentNode: "work", State: RunSuspended, AttemptCounts: map[string]int{}, PendingWait: &Suspension{Kind: WaitApproval, Ref: "approval-7"}}
	started := baseObservation(run, ObservationRunStarted, "work")
	started.State = RunRunning
	if err := journal.ObserveRun(ctx, started); err != nil {
		t.Fatal(err)
	}
	wait := baseObservation(run, ObservationRunStateChanged, "work")
	wait.State = RunSuspended
	wait.Wait = run.PendingWait
	if err := journal.ObserveRun(ctx, wait); err != nil {
		t.Fatal(err)
	}

	graph := GraphDef{
		ID: "graph-2", Version: "1", EntryNode: "work", MaxTransitions: 4,
		Nodes: []NodeDef{{ID: "work", Class: NodeDeterministic}, {ID: "complete", Class: NodeTerminal, TerminalState: RunSucceeded}},
		Transitions: []TransitionDef{{From: "work", Outcome: "done", To: "complete"}},
	}
	control := RunControl{Store: store, Actor: actor}
	if _, err := control.Resume(ctx, graph, "run-2", WaitApproval, "wrong", "resume-bad", "corr", runControlExecutor{}); err == nil {
		t.Fatal("expected mismatched wait reference to fail")
	}

	resumed, err := control.Resume(ctx, graph, "run-2", WaitApproval, "approval-7", "resume-ok", "corr", runControlExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Run.State != RunSucceeded {
		t.Fatalf("expected succeeded run, got %s", resumed.Run.State)
	}
	if resumed.Run.PendingWait != nil {
		t.Fatal("expected wait to be cleared")
	}
	if len(resumed.Run.Evidence) != 1 || resumed.Run.Evidence[0] != "work-complete" {
		t.Fatalf("expected resumed evidence, got %#v", resumed.Run.Evidence)
	}

	replayed, err := control.Status(ctx, "run-2")
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.State != RunSucceeded {
		t.Fatalf("expected durable succeeded state, got %s", replayed.Run.State)
	}
}
