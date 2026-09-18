package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// seedThroughApplying manually appends Planned/Approved/Prepared/Applying
// journal entries for the given plan/step, without ever calling
// driver.Apply, simulating a crash immediately after Applying was durably
// recorded but before the driver's outcome was journaled — the exact
// window PLAN-016 WU2 requires RunStep to resume from correctly.
func seedThroughApplying(t *testing.T, journal *Journal, req RunRequest, step contracts.LifecycleTransitionStep) {
	t.Helper()
	var previous contracts.LifecycleTransitionState
	for i, state := range []contracts.LifecycleTransitionState{contracts.LifecyclePlanned, contracts.LifecycleApproved, contracts.LifecyclePrepared, contracts.LifecycleApplying} {
		entry := journalEntry(req, step, i+1, state, previous, "", nil)
		if err := journal.Append(context.Background(), entry); err != nil {
			t.Fatalf("seed %s: %v", state, err)
		}
		previous = state
	}
}

func TestRunStepCommittedShortCircuitRejectsRestartEvidenceSubstitution(t *testing.T) {
	plan := migrationPlan(t, true)
	journal, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}, idempotent: true}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	if err := RunStep(context.Background(), journal, req, driver); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*RunRequest){
		func(r *RunRequest) { r.PreconditionDigest = migrationDigest("1") },
		func(r *RunRequest) { r.SnapshotDigest = migrationDigest("2") },
	} {
		changed := req
		mutate(&changed)
		if err := RunStep(context.Background(), journal, changed, driver); err == nil {
			t.Fatal("committed short-circuit accepted substituted restart evidence")
		}
	}
	if driver.calls != 1 {
		t.Fatalf("restart evidence refusal re-applied committed step: %d", driver.calls)
	}
}

func TestRunStepResumesFromApplyingWithoutIllegalSelfTransition(t *testing.T) {
	plan := migrationPlan(t, true)
	step := plan.Steps[0]
	journal, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	seedThroughApplying(t, journal, req, step)

	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}, idempotent: true}
	if err := RunStep(context.Background(), journal, req, driver); err != nil {
		t.Fatal(err)
	}
	if driver.calls != 1 {
		t.Fatalf("resume from Applying should call Apply exactly once, got %d", driver.calls)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 {
		t.Fatalf("resume from Applying must not append a second Applying entry; want 5 total entries, got %d: %+v", len(history), history)
	}
	last := history[len(history)-1]
	if last.State != contracts.LifecycleCommitted || last.PreviousState != contracts.LifecycleApplying {
		t.Fatalf("unexpected terminal transition: %+v", last)
	}
}

func TestRunStepResumeFromApplyingRequiresIdempotentDriver(t *testing.T) {
	plan := migrationPlan(t, true)
	step := plan.Steps[0]
	journal, err := NewJournal(eventstore.NewMemoryStore(), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	req := RunRequest{Plan: plan, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	seedThroughApplying(t, journal, req, step)

	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: plan.TargetManifestDigest}, idempotent: false}
	if err := RunStep(context.Background(), journal, req, driver); !errors.Is(err, ErrRetryNotSafe) {
		t.Fatalf("resume from Applying against a non-idempotent driver must refuse: %v", err)
	}
	if driver.calls != 0 {
		t.Fatalf("Apply must not be called when Idempotent refuses: %d calls", driver.calls)
	}
}

func migrationPlanVariant(t *testing.T) contracts.LifecyclePlan {
	t.Helper()
	plan := migrationPlan(t, true)
	plan.Steps = append([]contracts.LifecycleTransitionStep{}, plan.Steps...)
	plan.Steps[0].RecoveryStrategy = "a-different-recovery-strategy"
	plan.Digest = ""
	digest, err := plan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Digest = digest
	return plan
}

func TestRunStepFencesStalePlanDigestBeforeApplying(t *testing.T) {
	planA := migrationPlan(t, true)
	planB := migrationPlanVariant(t)
	if planA.Digest == planB.Digest {
		t.Fatal("test fixture did not actually vary the plan digest")
	}
	journal, err := NewJournal(eventstore.NewMemoryStore(), planA.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyFailedRecoverable, RecoveryAction: "retry"}}
	reqA := RunRequest{Plan: planA, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	if err := RunStep(context.Background(), journal, reqA, driver); err != nil {
		t.Fatal(err)
	}
	historyBefore, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	reqB := RunRequest{Plan: planB, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), RetryFailedRecoverable: true, Now: time.Now().UTC()}
	if err := RunStep(context.Background(), journal, reqB, driver); !errors.Is(err, ErrPlanDigestMismatch) {
		t.Fatalf("resuming under a substituted plan digest before applying must be refused: %v", err)
	}
	historyAfter, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(historyAfter) != len(historyBefore) {
		t.Fatalf("a refused pre-applying resume must not append any journal entry: before %d after %d", len(historyBefore), len(historyAfter))
	}
}

func TestRunStepFencesStalePlanDigestFromApplyingToReconcileRequired(t *testing.T) {
	planA := migrationPlan(t, true)
	planB := migrationPlanVariant(t)
	step := planA.Steps[0]
	journal, err := NewJournal(eventstore.NewMemoryStore(), planA.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	reqA := RunRequest{Plan: planA, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	seedThroughApplying(t, journal, reqA, step)

	driver := &migrationDriver{result: ApplyResult{Outcome: ApplyCommitted, ResultingManifestDigest: planA.TargetManifestDigest}, idempotent: true}
	reqB := RunRequest{Plan: planB, StepID: "migrate", PreconditionDigest: migrationDigest("e"), SnapshotDigest: migrationDigest("f"), Now: time.Now().UTC()}
	if err := RunStep(context.Background(), journal, reqB, driver); err != nil {
		t.Fatal(err)
	}
	if driver.calls != 0 {
		t.Fatalf("a substituted plan digest discovered at Applying must fence before ever calling Apply, got %d calls", driver.calls)
	}
	history, err := journal.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertReconcileEntriesValidate(t, history)
	last := history[len(history)-1]
	if last.State != contracts.LifecycleReconcileRequired || last.PreviousState != contracts.LifecycleApplying || last.RecoveryAction != "plan-digest-mismatch" {
		t.Fatalf("unexpected fencing transition: %+v", last)
	}
}
