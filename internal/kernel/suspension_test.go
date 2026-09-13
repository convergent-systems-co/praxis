package kernel

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type humanWaitExecutor struct {
	resolved bool
}

func (h *humanWaitExecutor) ExecuteNode(_ context.Context, _ GraphDef, node NodeDef, _ *RunExecution) (NodeResult, error) {
	if node.ID != "approve" {
		return NodeResult{}, errors.New("unexpected node")
	}
	if !h.resolved {
		return NodeResult{Suspension: &Suspension{Kind: WaitApproval, Ref: "approval:change-42"}}, nil
	}
	return NodeResult{Outcome: "approved", Evidence: []string{"approval:change-42"}}, nil
}

func TestHumanNodeSuspendsDurablyAcrossDisconnectAndResume(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{
		Store: store, Actor: contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		CommandID: "cmd-1", CorrelationID: "corr-1",
	}
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "approve", MaxTransitions: 2,
		Nodes: []NodeDef{
			{ID: "approve", Class: NodeHuman},
			{ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded},
		},
		Transitions: []TransitionDef{{From: "approve", Outcome: "approved", To: "done"}},
	}
	executor := &humanWaitExecutor{}
	run := &RunExecution{RunID: "run-human"}
	if err := RunObserved(context.Background(), graph, run, executor, journal); !errors.Is(err, ErrRunSuspended) {
		t.Fatalf("expected durable suspension, got %v", err)
	}
	if run.State != RunSuspended || run.PendingWait == nil || run.PendingWait.Ref != "approval:change-42" {
		t.Fatalf("unexpected suspended run: %+v", run)
	}

	// Simulate client/process disconnect by rebuilding only from authoritative events.
	events, err := store.LoadAggregate(context.Background(), run.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	replayed, version, err := ReplayRun(events)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.State != RunSuspended || replayed.PendingWait == nil || replayed.PendingWait.Ref != "approval:change-42" {
		t.Fatalf("wait did not survive replay: %+v", replayed)
	}
	if err := RunObserved(context.Background(), graph, replayed, executor, journal); !errors.Is(err, ErrRunSuspended) {
		t.Fatalf("unresolved wait must remain suspended, got %v", err)
	}

	// This does not grant authority; it only clears the runtime wait after the
	// caller has validated the referenced approval through the approval boundary.
	if err := SatisfyWait(replayed, WaitApproval, "approval:change-42"); err != nil {
		t.Fatal(err)
	}
	journal.ExpectedVersion = version
	journal.CommandID = "cmd-2"
	executor.resolved = true
	if err := RunObserved(context.Background(), graph, replayed, executor, journal); err != nil {
		t.Fatal(err)
	}
	if replayed.State != RunSucceeded || replayed.CurrentNode != "done" {
		t.Fatalf("unexpected resumed run: %+v", replayed)
	}
}

func TestSatisfyWaitRejectsWrongReference(t *testing.T) {
	run := &RunExecution{State: RunSuspended, PendingWait: &Suspension{Kind: WaitApproval, Ref: "approval:a"}}
	if err := SatisfyWait(run, WaitApproval, "approval:b"); err == nil {
		t.Fatal("mismatched approval reference must not release wait")
	}
}
