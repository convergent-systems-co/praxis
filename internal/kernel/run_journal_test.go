package kernel

import (
	"context"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestRunObservedPersistsAuthoritativeSequence(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{
		Store:         store,
		Actor:         contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		CommandID:     "cmd-1",
		CorrelationID: "corr-1",
		Now: func() time.Time {
			return time.Unix(1, 0)
		},
	}
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "work", MaxTransitions: 2,
		Nodes: []NodeDef{
			{ID: "work", Class: NodeDeterministic},
			{ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded},
		},
		Transitions: []TransitionDef{{From: "work", Outcome: "done", To: "done"}},
	}
	run := &RunExecution{RunID: "run-1"}
	if err := RunObserved(context.Background(), graph, run, scriptedExecutor{outcomes: map[string]string{"work": "done"}}, journal); err != nil {
		t.Fatal(err)
	}
	if run.State != RunSucceeded {
		t.Fatalf("expected succeeded run, got %s", run.State)
	}
	events, err := store.LoadAggregate(context.Background(), "run-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"run.started", "run.node_completed", "run.transitioned", "run.terminal"}
	if len(events) != len(want) {
		t.Fatalf("expected %d events, got %d", len(want), len(events))
	}
	for i, event := range events {
		if event.Type != want[i] {
			t.Fatalf("event %d: expected %s, got %s", i, want[i], event.Type)
		}
		if event.AggregateVersion != int64(i+1) {
			t.Fatalf("event %d: expected aggregate version %d, got %d", i, i+1, event.AggregateVersion)
		}
	}
	if journal.ExpectedVersion != 4 {
		t.Fatalf("expected journal version 4, got %d", journal.ExpectedVersion)
	}
}

func TestRunObservedFailsClosedOnJournalVersionConflict(t *testing.T) {
	store := eventstore.NewMemoryStore()
	journal := &EventJournal{
		Store:           store,
		Actor:           contracts.PrincipalRef{ID: "agent-1", Kind: "agent"},
		CommandID:       "cmd-1",
		CorrelationID:   "corr-1",
		ExpectedVersion: 1,
	}
	graph := GraphDef{
		ID: "g", Version: "1", EntryNode: "done",
		Nodes: []NodeDef{{ID: "done", Class: NodeTerminal, TerminalState: RunSucceeded}},
	}
	run := &RunExecution{RunID: "run-1"}
	if err := RunObserved(context.Background(), graph, run, scriptedExecutor{outcomes: map[string]string{}}, journal); err == nil {
		t.Fatal("expected journal version conflict")
	}
	if run.State != RunFailed {
		t.Fatalf("journal failure must fail closed, got %s", run.State)
	}
}
