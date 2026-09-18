package kernel

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fixedErrorExecutor struct{ err error }

func (f fixedErrorExecutor) ExecuteNode(_ context.Context, _ GraphDef, _ NodeDef, _ *RunExecution) (NodeResult, error) {
	return NodeResult{}, f.err
}

func TestUnknownExternalEffectPersistsReconcilingState(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{Store: store, Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, CommandID: "cmd-1", CorrelationID: "corr-1"}
	graph := GraphDef{ID: "g", Version: "1", EntryNode: "effect", Nodes: []NodeDef{{ID: "effect", Class: NodeCapability}}}
	run := &RunExecution{RunID: "run-reconcile"}
	err := RunObserved(context.Background(), graph, run, fixedErrorExecutor{err: &ExecutionError{Class: FailureExternalEffectUnknown, Err: errors.New("ambiguous remote outcome")}}, journal)
	if err == nil {
		t.Fatal("expected ambiguous effect error")
	}
	if run.State != RunReconciling {
		t.Fatalf("expected reconciling, got %s", run.State)
	}
	events, err := store.LoadAggregate(context.Background(), run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := ReplayRun(events)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.State != RunReconciling || replayed.CurrentNode != "effect" {
		t.Fatalf("reconciliation state lost on replay: %+v", replayed)
	}
}

func TestCancelledContextPersistsCancelledTerminalState(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{Store: store, Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, CommandID: "cmd-1", CorrelationID: "corr-1"}
	graph := GraphDef{ID: "g", Version: "1", EntryNode: "work", Nodes: []NodeDef{{ID: "work", Class: NodeDeterministic}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	run := &RunExecution{RunID: "run-cancel"}
	if err := RunObserved(ctx, graph, run, scriptedExecutor{outcomes: map[string]string{"work": "done"}}, journal); err == nil {
		t.Fatal("expected cancellation")
	}
	if run.State != RunCancelled {
		t.Fatalf("expected cancelled state, got %s", run.State)
	}
	events, err := store.LoadAggregate(context.Background(), run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := ReplayRun(events)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.State != RunCancelled {
		t.Fatalf("cancelled terminal state lost on replay: %+v", replayed)
	}
}

func TestRetryBudgetDoesNotResetAfterCrashBetweenAttempts(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{Store: store, Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}, CommandID: "cmd-1", CorrelationID: "corr-1"}
	observations := []RunObservation{
		{Kind: ObservationRunStarted, RunID: "run-retry-crash", GraphID: "g", GraphVersion: "1", NodeID: "work", State: RunRunning},
		{Kind: ObservationNodeAttemptFailed, RunID: "run-retry-crash", GraphID: "g", GraphVersion: "1", NodeID: "work", State: RunRunning, Attempt: 1, FailureClass: FailureExecutor},
	}
	for _, observation := range observations {
		if err := journal.ObserveRun(context.Background(), observation); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.LoadAggregate(context.Background(), "run-retry-crash", 0)
	if err != nil {
		t.Fatal(err)
	}
	replayed, version, err := ReplayRun(events)
	if err != nil {
		t.Fatal(err)
	}
	journal.ExpectedVersion = version
	journal.CommandID = "cmd-2"
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "work",
		Nodes: []NodeDef{{ID: "work", Class: NodeCapability, Retry: RetryPolicy{MaxAttempts: 2, Retryable: []FailureClass{FailureExecutor}}}},
	}
	executor := &retryExecutor{failures: 1, class: FailureExecutor}
	if err := RunObserved(context.Background(), graph, replayed, executor, journal); err == nil {
		t.Fatal("second failed attempt must exhaust total retry budget")
	}
	if replayed.AttemptCounts["work"] != 2 || executor.calls != 1 {
		t.Fatalf("retry budget reset after replay: attempts=%v calls=%d", replayed.AttemptCounts, executor.calls)
	}
}
