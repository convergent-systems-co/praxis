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
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}}}
	record, err := controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err != nil || record.EndHead != "b" || len(repo.publishes) != 1 || repo.publishes[0] != "b" {
		t.Fatalf("checkpoint publication mismatch: %+v err=%v publishes=%v", record, err, repo.publishes)
	}
}

func TestExecuteTurnWithRepositoryDoesNotPersistCompletionBeforeFailedPublish(t *testing.T) {
	worker := &fakeWorker{result: WorkerResult{Outcome: OutcomeComplete, EndHead: "b", CheckpointValid: true}}
	controller := controllerFixture(worker)
	repo := &fakeRepository{snapshots: []RepositorySnapshot{{Clean: true, Relation: contracts.RelationEqual, Head: "a"}}, pushErr: errors.New("remote verification failed")}
	record, err := controller.ExecuteTurnWithRepository(context.Background(), turnRequest(), repo)
	if err == nil || record.Outcome != OutcomeBlocked {
		t.Fatalf("failed publication must return blocked evidence: %+v err=%v", record, err)
	}
	turns, loadErr := controller.Ledger.Load(context.Background(), "goal-1", "1")
	if loadErr != nil || len(turns) != 1 || turns[0].Outcome != OutcomeBlocked {
		t.Fatalf("failed publication must not persist completion: %+v err=%v", turns, loadErr)
	}
}
