package kernel

import (
	"context"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestResumeSignalClearsExactWaitAndLeavesRunRunnable(t *testing.T) {
	ctx := context.Background()
	store := eventstore.NewMemoryStore()
	actor := contracts.PrincipalRef{ID: "user-1", Kind: "user"}
	journal := &EventJournal{Store: store, Actor: actor, CommandID: "seed", CorrelationID: "corr"}
	run := &RunExecution{RunID: "run-signal", GraphID: "graph-1", GraphVersion: "1", CurrentNode: "work", State: RunSuspended, AttemptCounts: map[string]int{}, PendingWait: &Suspension{Kind: WaitApproval, Ref: "approval-1"}}
	started := baseObservation(run, ObservationRunStarted, "work")
	started.State = RunRunning
	if err := journal.ObserveRun(ctx, started); err != nil { t.Fatal(err) }
	wait := baseObservation(run, ObservationRunStateChanged, "work")
	wait.State = RunSuspended
	wait.Wait = run.PendingWait
	if err := journal.ObserveRun(ctx, wait); err != nil { t.Fatal(err) }

	control := RunControl{Store: store, Actor: actor, Authorizer: testRunControlAuthorizer{}}
	if _, err := control.ResumeSignal(ctx, run.RunID, WaitApproval, "wrong", "bad", "corr"); err == nil {
		t.Fatal("mismatched wait must fail")
	}
	resumed, err := control.ResumeSignal(ctx, run.RunID, WaitApproval, "approval-1", "resume", "corr")
	if err != nil { t.Fatal(err) }
	if resumed.Run.State != RunRunnable || resumed.Run.PendingWait != nil {
		t.Fatalf("expected runnable run with cleared wait: %#v", resumed.Run)
	}
	replayed, err := control.Status(ctx, run.RunID)
	if err != nil { t.Fatal(err) }
	if replayed.Run.State != RunRunnable || replayed.Run.PendingWait != nil {
		t.Fatalf("resume signal not durable: %#v", replayed.Run)
	}
}
