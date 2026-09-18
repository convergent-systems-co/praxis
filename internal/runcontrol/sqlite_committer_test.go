package runcontrol

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type completingExecutor struct{}

func (completingExecutor) ExecuteNode(_ context.Context, _ kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution) (kernel.NodeResult, error) {
	if node.ID == "work" {
		return kernel.NodeResult{Outcome: "done", Evidence: []string{"resumed-work"}}, nil
	}
	return kernel.NodeResult{Outcome: "done"}, nil
}

func TestSQLiteCommitterAtomicallyConsumesCancelLease(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	now := time.Date(2026, 9, 13, 15, 30, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	events := state.NewSQLiteEventStore(db)
	seed := &kernel.EventJournal{Store: events, Actor: actor, CommandID: "seed", CorrelationID: "corr", Now: func() time.Time { return now.Add(-time.Minute) }}
	run := &kernel.RunExecution{RunID: "run-cancel", GraphID: "graph-1", GraphVersion: "1", CurrentNode: "work", State: kernel.RunRunning, AttemptCounts: map[string]int{}}
	if err := seed.ObserveRun(ctx, kernel.RunObservation{Kind: kernel.ObservationRunStarted, RunID: run.RunID, GraphID: run.GraphID, GraphVersion: run.GraphVersion, NodeID: run.CurrentNode, State: run.State}); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"cancel-once", actor.ID, actor.Kind, kernel.RunControlCapability, []byte(`["cancel"]`), "run:run-cancel", now.Add(-time.Minute).Format(time.RFC3339Nano), 1); err != nil { t.Fatal(err) }

	control := kernel.RunControl{Store: events, Actor: actor, Committer: SQLiteCommitter{State: state.New(db), Now: func() time.Time { return now }}}
	result, err := control.Cancel(ctx, "run-cancel", "cancel-command", "corr")
	if err != nil { t.Fatal(err) }
	if result.Run.State != kernel.RunCancelled || result.AggregateVersion != 2 { t.Fatalf("unexpected cancel result: %+v", result) }
	var uses int
	if err := db.QueryRowContext(ctx, `SELECT remaining_uses FROM capability_leases WHERE lease_id='cancel-once'`).Scan(&uses); err != nil { t.Fatal(err) }
	if uses != 0 { t.Fatalf("expected one-shot cancel lease consumed, got %d", uses) }
	if _, err := control.Cancel(ctx, "run-cancel", "cancel-again", "corr"); err == nil { t.Fatal("expected terminal run to reject second cancellation") }
}

func TestSQLiteCommitterDurablyConsumesResumeLeaseBeforeExecution(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	now := time.Date(2026, 9, 13, 15, 30, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "operator-1", Kind: "user"}
	events := state.NewSQLiteEventStore(db)
	seed := &kernel.EventJournal{Store: events, Actor: actor, CommandID: "seed-resume", CorrelationID: "corr", Now: func() time.Time { return now.Add(-time.Minute) }}
	start := kernel.RunObservation{Kind: kernel.ObservationRunStarted, RunID: "run-resume", GraphID: "graph-resume", GraphVersion: "1", NodeID: "work", State: kernel.RunRunning}
	if err := seed.ObserveRun(ctx, start); err != nil { t.Fatal(err) }
	wait := kernel.RunObservation{Kind: kernel.ObservationRunStateChanged, RunID: "run-resume", GraphID: "graph-resume", GraphVersion: "1", NodeID: "work", State: kernel.RunSuspended, Wait: &kernel.Suspension{Kind: kernel.WaitApproval, Ref: "approval-1"}}
	if err := seed.ObserveRun(ctx, wait); err != nil { t.Fatal(err) }
	if _, err := db.ExecContext(ctx, `INSERT INTO capability_leases(lease_id,principal_id,principal_kind,capability,operations_json,scope,issued_at,remaining_uses,version) VALUES(?,?,?,?,?,?,?,?,1)`,
		"resume-once", actor.ID, actor.Kind, kernel.RunControlCapability, []byte(`["resume"]`), "run:run-resume", now.Add(-time.Minute).Format(time.RFC3339Nano), 1); err != nil { t.Fatal(err) }
	graph := kernel.GraphDef{ID: "graph-resume", Version: "1", EntryNode: "work", MaxTransitions: 4, Nodes: []kernel.NodeDef{{ID: "work", Class: kernel.NodeDeterministic}, {ID: "complete", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded}}, Transitions: []kernel.TransitionDef{{From: "work", Outcome: "done", To: "complete"}}}
	control := kernel.RunControl{Store: events, Actor: actor, Committer: SQLiteCommitter{State: state.New(db), Now: func() time.Time { return now }}}
	result, err := control.Resume(ctx, graph, "run-resume", kernel.WaitApproval, "approval-1", "resume-command", "corr", completingExecutor{})
	if err != nil { t.Fatal(err) }
	if result.Run.State != kernel.RunSucceeded { t.Fatalf("expected successful resumed run, got %s", result.Run.State) }
	var uses int
	if err := db.QueryRowContext(ctx, `SELECT remaining_uses FROM capability_leases WHERE lease_id='resume-once'`).Scan(&uses); err != nil { t.Fatal(err) }
	if uses != 0 { t.Fatalf("expected one-shot resume lease consumed, got %d", uses) }
	replayed, err := control.Status(ctx, "run-resume")
	if err != nil { t.Fatal(err) }
	if replayed.Run.PendingWait != nil || replayed.Run.State != kernel.RunSucceeded { t.Fatalf("unexpected replay after resume: %+v", replayed.Run) }
}
