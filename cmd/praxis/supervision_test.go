package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestSupervisionCLIInterventionAndObservationUseDurableActivityStream(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	t.Setenv("PRAXIS_DB", dbPath)
	t.Setenv("PRAXIS_ACTOR_ID", "human-operator")
	t.Setenv("PRAXIS_ACTOR_KIND", "human")
	if err := runSuperviseCommand([]string{"comment", "--goal-id", "goal-1", "--goal-version", "1", "--invocation-id", "inv-1", "--turn-id", "turn-1", "--text", "check the bounded assumption"}); err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenSQLite(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log := goaldrive.ActivityLog{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "observer", Kind: "cli"}}
	events, err := log.Load(context.Background(), "inv-1", "turn-1", 0)
	if err != nil || len(events) != 1 || events[0].Type != goaldrive.ActivityHumanComment || events[0].Data["text"] != "check the bounded assumption" {
		t.Fatalf("CLI intervention was not durable: %+v %v", events, err)
	}
	if err := runSuperviseCommand([]string{"observe", "--goal-id", "goal-1", "--goal-version", "1", "--invocation-id", "inv-1", "--turn-id", "turn-1"}); err != nil {
		t.Fatal(err)
	}
}
