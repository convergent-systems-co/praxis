package goaldrive

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fakeRepository struct {
	snapshots    []RepositorySnapshot
	fastForwards int
	publishes    []string
	snapshotErr  error
	pushErr      error
}

func (r *fakeRepository) Snapshot(context.Context) (RepositorySnapshot, error) {
	if r.snapshotErr != nil {
		return RepositorySnapshot{}, r.snapshotErr
	}
	if len(r.snapshots) == 0 {
		return RepositorySnapshot{}, errors.New("no snapshot")
	}
	snapshot := r.snapshots[0]
	if len(r.snapshots) > 1 {
		r.snapshots = r.snapshots[1:]
	}
	return snapshot, nil
}
func (r *fakeRepository) FastForward(context.Context) error { r.fastForwards++; return nil }
func (r *fakeRepository) PushAndVerify(_ context.Context, head string) error {
	r.publishes = append(r.publishes, head)
	return r.pushErr
}

func TestPrepareRepositoryFastForwardsCleanRemoteAhead(t *testing.T) {
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationRemoteAhead, Head: "remote"}, {Clean: true, Relation: contracts.RelationEqual, Head: "remote"}}}
	snapshot, err := PrepareRepository(context.Background(), repo)
	if err != nil || snapshot.Head != "remote" || repo.fastForwards != 1 {
		t.Fatalf("remote-ahead preparation mismatch: %+v err=%v ff=%d", snapshot, err, repo.fastForwards)
	}
}

func TestPrepareRepositoryRefusesDirtyRemoteAhead(t *testing.T) {
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: false, Relation: contracts.RelationRemoteAhead, Head: "local"}}}
	if _, err := PrepareRepository(context.Background(), repo); !errors.Is(err, ErrUnsafeRepository) || repo.fastForwards != 0 {
		t.Fatalf("dirty remote-ahead state must stop without fast-forward: %v ff=%d", err, repo.fastForwards)
	}
}

func TestExecuteTurnWithRepositoryPublishesOnlyValidatedProgress(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}, {Clean: true, Relation: contracts.RelationLocalAhead, Head: "b"}}}
	record, err := controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err != nil || record.EndHead != "b" || len(repo.publishes) != 1 || repo.publishes[0] != "b" {
		t.Fatalf("checkpoint publication mismatch: %+v err=%v publishes=%v", record, err, repo.publishes)
	}
}

func TestExecuteTurnWithRepositoryNoPushRetainsProgressWithoutPublication(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}}}
	req := turnRequest()
	req.NoPush = true
	repo.snapshots = append(repo.snapshots, RepositorySnapshot{Clean: true, Relation: contracts.RelationLocalAhead, Head: "b"})
	record, err := controller.ExecuteTurnWithRepository(context.Background(), req, repo)
	if err != nil || record.Outcome != OutcomeComplete || !record.Progress || record.CheckpointPublished || len(repo.publishes) != 0 {
		t.Fatalf("no-push must retain validated local progress without publication: %+v err=%v publishes=%v", record, err, repo.publishes)
	}
	turns, loadErr := controller.Ledger.Load(context.Background(), req.GoalID, req.GoalVersion)
	if loadErr != nil || len(turns) != 1 || !turns[0].Progress || turns[0].CheckpointPublished {
		t.Fatalf("no-push result must be durable and unpublished: %+v err=%v", turns, loadErr)
	}
}

func TestExecuteTurnWithRepositoryDoesNotPersistCompletionBeforeFailedPublish(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}, {Clean: true, Relation: contracts.RelationLocalAhead, Head: "b"}}, pushErr: errors.New("remote verification failed")}
	record, err := controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err == nil || record.Outcome != OutcomeBlocked {
		t.Fatalf("failed publication must return blocked evidence: %+v err=%v", record, err)
	}
	turns, loadErr := controller.Ledger.Load(context.Background(), "goal-1", "1")
	if loadErr != nil || len(turns) != 1 || turns[0].Outcome != OutcomeBlocked {
		t.Fatalf("failed publication must not persist completion: %+v err=%v", turns, loadErr)
	}
}

// TestExecuteTurnWithRepositoryRefusesWorkerHeadClaimTheCheckoutDoesNotShow
// proves a self-reporting worker cannot assert progress the repository does
// not show: the controller inspects the checkout and blocks on a mismatch.
func TestExecuteTurnWithRepositoryRefusesWorkerHeadClaimTheCheckoutDoesNotShow(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}, {Clean: true, Relation: contracts.RelationLocalAhead, Head: "z"}}}
	record, err := controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err == nil || record.Outcome != OutcomeBlocked || record.Progress || len(repo.publishes) != 0 || record.EndHead != "z" {
		t.Fatalf("a head claim the checkout does not show must block without publication: %+v err=%v publishes=%v", record, err, repo.publishes)
	}
	dirty := &fakeWorker{result: WorkerResult{Outcome: OutcomeContinue, EndHead: "b", CheckpointValid: true}}
	controller = controllerFixture(dirty)
	repo = &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}, {Clean: false, Relation: contracts.RelationLocalAhead, Head: "b"}}}
	record, err = controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err == nil || record.Outcome != OutcomeBlocked || record.Progress || len(repo.publishes) != 0 {
		t.Fatalf("a dirty checkout must block regardless of the worker's claim: %+v err=%v", record, err)
	}
}
