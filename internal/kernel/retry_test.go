package kernel

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type retryExecutor struct {
	failures int
	calls    int
	class    FailureClass
}

func (r *retryExecutor) ExecuteNode(_ context.Context, _ GraphDef, _ NodeDef, _ *RunExecution) (NodeResult, error) {
	r.calls++
	if r.calls <= r.failures {
		return NodeResult{}, &ExecutionError{Class: r.class, Err: errors.New("fixture failure")}
	}
	return NodeResult{Outcome: "done", Evidence: []string{"ok"}}, nil
}

func TestRunRetriesDeclaredFailureClass(t *testing.T) {
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "work", MaxTransitions: 2,
		Nodes: []NodeDef{
			{ID: "work", Class: NodeCapability, Retry: RetryPolicy{MaxAttempts: 3, Retryable: []FailureClass{FailureExecutor}}},
			{ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded},
		},
		Transitions: []TransitionDef{{From: "work", Outcome: "done", To: "done"}},
	}
	executor := &retryExecutor{failures: 2, class: FailureExecutor}
	run := &RunExecution{RunID: "retry-success"}
	if err := Run(context.Background(), graph, run, executor); err != nil {
		t.Fatal(err)
	}
	if executor.calls != 3 || run.AttemptCounts["work"] != 3 || run.State != RunSucceeded {
		t.Fatalf("unexpected retry result calls=%d run=%+v", executor.calls, run)
	}
}

func TestRunDoesNotRetryPolicyFailure(t *testing.T) {
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "work",
		Nodes: []NodeDef{
			{ID: "work", Class: NodeCapability, Retry: RetryPolicy{MaxAttempts: 3, Retryable: []FailureClass{FailureExecutor}}},
			{ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded},
		},
		Transitions: []TransitionDef{{From: "work", Outcome: "done", To: "done"}},
	}
	executor := &retryExecutor{failures: 3, class: FailurePolicyInvariant}
	run := &RunExecution{RunID: "retry-denied"}
	if err := Run(context.Background(), graph, run, executor); err == nil {
		t.Fatal("policy failure must fail run")
	}
	if executor.calls != 1 || run.State != RunFailed {
		t.Fatalf("policy failure retried or wrong state: calls=%d run=%+v", executor.calls, run)
	}
}

func TestRetryAttemptsPersistAndRemainExhaustedAfterReplay(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{
		Store: store, Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		CommandID: "cmd-1", CorrelationID: "corr-1",
	}
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "work",
		Nodes: []NodeDef{
			{ID: "work", Class: NodeCapability, Retry: RetryPolicy{MaxAttempts: 2, Retryable: []FailureClass{FailureExecutor}}},
			{ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded},
		},
		Transitions: []TransitionDef{{From: "work", Outcome: "done", To: "done"}},
	}
	executor := &retryExecutor{failures: 3, class: FailureExecutor}
	run := &RunExecution{RunID: "retry-replay"}
	if err := RunObserved(context.Background(), graph, run, executor, journal); err == nil {
		t.Fatal("expected exhausted retry failure")
	}
	events, err := store.LoadAggregate(context.Background(), run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := ReplayRun(events)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.AttemptCounts["work"] != 2 || replayed.State != RunFailed {
		t.Fatalf("retry exhaustion not preserved: %+v", replayed)
	}
}

func TestGraphRejectsAutomaticRetryOfUnknownExternalEffect(t *testing.T) {
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "work",
		Nodes: []NodeDef{
			{ID: "work", Class: NodeCapability, Retry: RetryPolicy{MaxAttempts: 2, Retryable: []FailureClass{FailureExternalEffectUnknown}}},
		},
	}
	if err := graph.Validate(); err == nil {
		t.Fatal("unknown external effect must not be automatically retryable")
	}
}
