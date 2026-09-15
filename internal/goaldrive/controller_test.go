package goaldrive

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fakeWorker struct {
	called int
	last   WorkerRequest
	result WorkerResult
	err    error
}

func (w *fakeWorker) Execute(_ context.Context, request WorkerRequest) (WorkerResult, error) {
	w.called++
	w.last = request
	return w.result, w.err
}

func controllerCandidate(id string, priority, sequence int, completed bool) contracts.WorkCandidate {
	return contracts.WorkCandidate{ID: id, Priority: priority, Sequence: sequence, Completed: completed, SourceRef: "docs/PLAN/003-post-release-roadmap.md#" + id, SourceDigest: "sha256:roadmap", Provenance: contracts.ProvenancePLAN}
}

func TestControllerSelectsOneAuthoritativeRunnableUnitWhenObjectiveIsAbsent(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	req := turnRequest()
	req.ChildObjective = ""
	req.WorkCandidates = []contracts.WorkCandidate{controllerCandidate("blocked", 0, 0, false), controllerCandidate("ready", 1, 0, false), controllerCandidate("done", 0, 1, true)}
	req.WorkRelationships = []contracts.WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: contracts.RelationshipHardDependency, SourceRef: "docs/PLAN/003-post-release-roadmap.md", SourceDigest: "sha256:roadmap", Provenance: contracts.ProvenancePLAN}}
	record, err := controller.ExecuteTurn(context.Background(), req)
	if err != nil || record.ChildObjective != "ready" || worker.called != 1 || worker.last.ChildObjective != "ready" {
		t.Fatalf("controller did not select exactly one authoritative runnable unit: record=%+v worker=%+v err=%v", record, worker.last, err)
	}
	next := req
	next.TurnID = "turn-2"
	if _, err := controller.ExecuteTurn(context.Background(), next); !errors.Is(err, ErrSupervisedTerminated) || worker.called != 1 {
		t.Fatalf("fresh selection must still stop at supervised boundary: err=%v calls=%d", err, worker.called)
	}
}

func TestControllerFailsClosedWhenObjectiveIsAbsentAndNoUnitIsRunnable(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	req := turnRequest()
	req.ChildObjective = ""
	req.WorkCandidates = []contracts.WorkCandidate{controllerCandidate("blocked", 0, 0, false)}
	req.WorkRelationships = []contracts.WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: contracts.RelationshipHardDependency, SourceRef: "docs/PLAN/003-post-release-roadmap.md", SourceDigest: "sha256:roadmap", Provenance: contracts.ProvenancePLAN}}
	if _, err := controller.ExecuteTurn(context.Background(), req); !errors.Is(err, contracts.ErrNoRunnableWork) || worker.called != 0 {
		t.Fatalf("no runnable authoritative unit must stop before worker: err=%v calls=%d", err, worker.called)
	}
}

func controllerFixture(worker Worker) Controller {
	return Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, NoProgressLimit: 1}
}

func turnRequest() TurnRequest {
	return TurnRequest{GoalID: "goal-1", GoalVersion: "1", InvocationID: "invocation-1", Mode: ModeSupervised, TurnID: "turn-1", ChildObjective: "inspect issue dependencies", GraphID: "praxis.package.goals.default", GraphVersion: "0.1.0", StartHead: "a", Repository: contracts.RepositorySynced}
}

func TestControllerInvokesOneBoundedTurnAndRecordsCompletion(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true, CheckpointEvidence: []string{"commit:b"}}}
	record, err := controllerFixture(worker).ExecuteTurn(context.Background(), turnRequest())
	if err != nil || record.Outcome != OutcomeComplete || !record.Progress || worker.called != 1 {
		t.Fatalf("unexpected controller result: %+v err=%v calls=%d", record, err, worker.called)
	}
}

func TestControllerRejectsUnsafeStateBeforeWorker(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	req := turnRequest()
	req.Repository = contracts.RepositoryDirty
	if _, err := controllerFixture(worker).ExecuteTurn(context.Background(), req); !errors.Is(err, ErrUnsafeRepository) || worker.called != 0 {
		t.Fatalf("unsafe state must stop before worker: err=%v calls=%d", err, worker.called)
	}
}

func TestControllerConvertsUnchangedContinueAndBoundsRetry(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeContinue, EndHead: "a", CheckpointValid: true}}
	controller := controllerFixture(worker)
	first, err := controller.ExecuteTurn(context.Background(), turnRequest())
	if err != nil || first.Outcome != OutcomeNoProgress || first.Progress {
		t.Fatalf("unchanged CONTINUE must become NO_PROGRESS: %+v %v", first, err)
	}
	second := turnRequest()
	second.TurnID = "turn-2"
	if _, err := controller.ExecuteTurn(context.Background(), second); !errors.Is(err, ErrNoProgressLimit) || worker.called != 1 {
		t.Fatalf("no-progress limit must stop repeated worker turns: err=%v calls=%d", err, worker.called)
	}
}

func TestControllerPersistsWorkerFailureAsBlocked(t *testing.T) {
	worker := &fakeWorker{err: errors.New("worker interrupted")}
	record, err := controllerFixture(worker).ExecuteTurn(context.Background(), turnRequest())
	if !errors.Is(err, worker.err) || record.Outcome != OutcomeBlocked || record.Blocker == "" {
		t.Fatalf("worker failure must be durable blocked evidence: %+v %v", record, err)
	}
}

func TestSupervisedInvocationStopsAfterPersistedProgress(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	if _, err := controller.ExecuteTurn(context.Background(), turnRequest()); err != nil {
		t.Fatal(err)
	}
	next := turnRequest()
	next.TurnID = "turn-2"
	next.ChildObjective = "select the next issue"
	if _, err := controller.ExecuteTurn(context.Background(), next); !errors.Is(err, ErrSupervisedTerminated) || worker.called != 1 {
		t.Fatalf("supervised invocation crossed checkpoint boundary: err=%v calls=%d", err, worker.called)
	}
	next.InvocationID = "invocation-2"
	next.StartHead = "b"
	worker.result.EndHead = "c"
	if _, err := controller.ExecuteTurn(context.Background(), next); err != nil || worker.called != 2 {
		t.Fatalf("new supervised invocation should be allowed: err=%v calls=%d", err, worker.called)
	}
}

func TestContinuousInvocationMayRepeatBoundedPrimitive(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	req := turnRequest()
	req.Mode = ModeContinuous
	if _, err := controller.ExecuteTurn(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	req.TurnID = "turn-2"
	req.StartHead = "b"
	worker.result.EndHead = "c"
	if _, err := controller.ExecuteTurn(context.Background(), req); err != nil || worker.called != 2 {
		t.Fatalf("continuous mode must own permitted repetition: err=%v calls=%d", err, worker.called)
	}
}

func TestInvocationModeCannotBeChangedToCrossBoundary(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	if _, err := controller.ExecuteTurn(context.Background(), turnRequest()); err != nil {
		t.Fatal(err)
	}
	req := turnRequest()
	req.TurnID = "turn-2"
	req.Mode = ModeContinuous
	if _, err := controller.ExecuteTurn(context.Background(), req); !errors.Is(err, ErrInvocationModeMismatch) || worker.called != 1 {
		t.Fatalf("mode change crossed invocation boundary: err=%v calls=%d", err, worker.called)
	}
}
