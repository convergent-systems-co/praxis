package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestStatusCommandReadsDurableRunFromExplicitDatabase(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	journal := &kernel.EventJournal{Store: state.NewSQLiteEventStore(db), Actor: actor, CommandID: "seed-status", CorrelationID: "status-test"}
	run := &kernel.RunExecution{RunID: "run-status", GraphID: "graph-status", GraphVersion: "1", CurrentNode: "work", State: kernel.RunRunning, AttemptCounts: map[string]int{}}
	observation := kernel.RunObservation{Kind: kernel.ObservationRunStarted, RunID: run.RunID, GraphID: run.GraphID, GraphVersion: run.GraphVersion, NodeID: run.CurrentNode, State: run.State}
	if err := journal.ObserveRun(ctx, observation); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := statusCommand(ctx, []string{"run-status", "--db", path}, &out, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	var got runStatusOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.RunID != "run-status" || got.GraphID != "graph-status" || got.CurrentNode != "work" || got.State != kernel.RunRunning || got.AggregateVersion != 1 {
		t.Fatalf("unexpected status output: %+v", got)
	}
}

func TestStatusCommandRequiresExplicitDatabaseTarget(t *testing.T) {
	var out bytes.Buffer
	if err := statusCommand(context.Background(), []string{"run-status"}, &out, func(string) string { return "" }); err == nil {
		t.Fatal("expected status without --db or PRAXIS_DB to fail")
	}
}

func TestStatusCommandAcceptsPraxisDBEnvironment(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "agent-1", Kind: "agent"}
	journal := &kernel.EventJournal{Store: state.NewSQLiteEventStore(db), Actor: actor, CommandID: "seed-env-status", CorrelationID: "status-env-test"}
	observation := kernel.RunObservation{Kind: kernel.ObservationRunStarted, RunID: "run-env", GraphID: "graph-env", GraphVersion: "1", NodeID: "start", State: kernel.RunRunning}
	if err := journal.ObserveRun(ctx, observation); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := statusCommand(ctx, []string{"run-env"}, &out, func(key string) string {
		if key == "PRAXIS_DB" {
			return path
		}
		return ""
	}); err != nil {
		t.Fatal(err)
	}
}
