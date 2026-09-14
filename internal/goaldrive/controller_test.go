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
	result WorkerResult
	err    error
}

func (w *fakeWorker) Execute(_ context.Context, _ WorkerRequest) (WorkerResult, error) {
	w.called++
	return w.result, w.err
}

func controllerFixture(worker Worker) Controller {
	return Controller{Ledger: Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Worker: worker, NoProgressLimit: 1}
}

func turnRequest() TurnRequest {
	return TurnRequest{GoalID: "goal-1", GoalVersion: "1", TurnID: "turn-1", ChildObjective: "inspect issue dependencies", GraphID: "praxis.package.goals.default", GraphVersion: "0.1.0", StartHead: "a", Repository: contracts.RepositorySynced}
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
