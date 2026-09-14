package goaldrive

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func fixtureTurn() TurnRecord {
	return TurnRecord{GoalID: "goal-1", GoalVersion: "1", TurnID: "turn-1", ChildObjective: "inventory issues", GraphID: "praxis.package.goals.default", GraphVersion: "0.1.0", StartHead: "a", EndHead: "b", Outcome: OutcomeContinue, Progress: true, CheckpointEvidence: []string{"commit:b"}}
}

func TestLedgerAppendsAndReplaysTurns(t *testing.T) {
	store := eventstore.NewMemoryStore()
	ledger := Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if _, err := ledger.Append(context.Background(), 0, fixtureTurn()); err != nil {
		t.Fatal(err)
	}
	second := fixtureTurn()
	second.TurnID = "turn-2"
	second.Outcome = OutcomeComplete
	second.EndHead = "c"
	if _, err := ledger.Append(context.Background(), 1, second); err != nil {
		t.Fatal(err)
	}
	turns, err := ledger.Load(context.Background(), "goal-1", "1")
	if err != nil || len(turns) != 2 || turns[1].Outcome != OutcomeComplete {
		t.Fatalf("replay mismatch: %+v %v", turns, err)
	}
}

func TestLedgerRejectsStaleAppendAndAuthorityConfusion(t *testing.T) {
	store := eventstore.NewMemoryStore()
	ledger := Ledger{Store: store, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if _, err := ledger.Append(context.Background(), 0, fixtureTurn()); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Append(context.Background(), 0, fixtureTurn()); !errors.Is(err, eventstore.ErrVersionConflict) {
		t.Fatalf("expected stale append conflict, got %v", err)
	}
	invalid := fixtureTurn()
	invalid.Outcome = OutcomeComplete
	invalid.Progress = false
	if _, err := ledger.Append(context.Background(), 1, invalid); err == nil {
		t.Fatal("completion without progress must fail closed")
	}
}

func TestLedgerSurvivesSQLiteRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	ledger := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if _, err := ledger.Append(ctx, 0, fixtureTurn()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resumed := Ledger{Store: state.NewSQLiteEventStore(reopened), Actor: ledger.Actor}
	turns, err := resumed.Load(ctx, "goal-1", "1")
	if err != nil || len(turns) != 1 || turns[0].TurnID != "turn-1" {
		t.Fatalf("restart replay mismatch: %+v %v", turns, err)
	}
}
