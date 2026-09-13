package state

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type qualificationRunAuthorizer struct{}

func (qualificationRunAuthorizer) AuthorizeRunControl(_ context.Context, _ contracts.PrincipalRef, _ kernel.RunExecution, _ kernel.RunControlOperation) error {
	return nil
}

func TestQualificationRunControlSurvivesSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}

	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteEventStore(db)
	journal := &kernel.EventJournal{Store: store, Actor: actor, CommandID: "start-command", CorrelationID: "qualification"}
	run := &kernel.RunExecution{RunID: "qual-run", GraphID: "qual-graph", GraphVersion: "1", CurrentNode: "work", State: kernel.RunRunning, AttemptCounts: map[string]int{}}
	start := kernel.RunObservation{Kind: kernel.ObservationRunStarted, RunID: run.RunID, GraphID: run.GraphID, GraphVersion: run.GraphVersion, NodeID: run.CurrentNode, State: run.State}
	if err := journal.ObserveRun(ctx, start); err != nil {
		t.Fatal(err)
	}
	control := kernel.RunControl{Store: store, Actor: actor, Authorizer: qualificationRunAuthorizer{}}
	if _, err := control.Cancel(ctx, run.RunID, "cancel-command", "qualification"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	replayed, err := (kernel.RunControl{Store: NewSQLiteEventStore(reopened), Actor: actor}).Status(ctx, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Run.State != kernel.RunCancelled {
		t.Fatalf("expected cancellation to survive restart, got %s", replayed.Run.State)
	}
	if replayed.AggregateVersion != 2 {
		t.Fatalf("expected aggregate version 2 after restart, got %d", replayed.AggregateVersion)
	}
}
