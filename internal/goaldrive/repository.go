package goaldrive

import (
	"context"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type RepositorySnapshot struct {
	Clean    bool
	Relation contracts.RepositoryRelation
	Head     string
}

// RepositoryAdapter is the controller-owned VCS boundary. A worker never
// receives this interface and therefore cannot fetch, fast-forward, push, or
// verify remote authority.
type RepositoryAdapter interface {
	Snapshot(context.Context) (RepositorySnapshot, error)
	FastForward(context.Context) error
	PushAndVerify(context.Context, string) error
}

func PrepareRepository(ctx context.Context, repo RepositoryAdapter) (RepositorySnapshot, error) {
	if repo == nil {
		return RepositorySnapshot{}, errors.New("Goal-drive repository adapter is required")
	}
	snapshot, err := repo.Snapshot(ctx)
	if err != nil {
		return RepositorySnapshot{}, err
	}
	state := contracts.ClassifyRepositoryState(snapshot.Clean, snapshot.Relation)
	if state == contracts.RepositoryRemoteAhead {
		if err := repo.FastForward(ctx); err != nil {
			return RepositorySnapshot{}, fmt.Errorf("fast-forward remote-ahead repository: %w", err)
		}
		snapshot, err = repo.Snapshot(ctx)
		if err != nil {
			return RepositorySnapshot{}, err
		}
		state = contracts.ClassifyRepositoryState(snapshot.Clean, snapshot.Relation)
	}
	if state != contracts.RepositorySynced {
		return RepositorySnapshot{}, fmt.Errorf("%w: %s", ErrUnsafeRepository, state)
	}
	if snapshot.Head == "" {
		return RepositorySnapshot{}, errors.New("synchronized repository has no authoritative HEAD")
	}
	return snapshot, nil
}

func PublishCheckpoint(ctx context.Context, repo RepositoryAdapter, record TurnRecord) error {
	if repo == nil {
		return errors.New("Goal-drive repository adapter is required")
	}
	if !record.Progress {
		return nil
	}
	if record.EndHead == "" {
		return errors.New("progressing turn has no checkpoint HEAD")
	}
	if err := repo.PushAndVerify(ctx, record.EndHead); err != nil {
		return fmt.Errorf("publish and verify Goal checkpoint: %w", err)
	}
	return nil
}

// ExecuteTurnWithRepository composes synchronization, one worker turn, and
// controller-owned checkpoint publication. A durable ledger record remains
// available even when publication fails, so recovery cannot fabricate success.
func (c Controller) ExecuteTurnWithRepository(ctx context.Context, req TurnRequest, repo RepositoryAdapter) (TurnRecord, error) {
	snapshot, err := PrepareRepository(ctx, repo)
	if err != nil {
		return TurnRecord{}, err
	}
	if req.StartHead != "" && req.StartHead != snapshot.Head {
		return TurnRecord{}, fmt.Errorf("requested start HEAD %q differs from synchronized HEAD %q", req.StartHead, snapshot.Head)
	}
	req.StartHead = snapshot.Head
	req.Repository = contracts.RepositorySynced
	turns, err := c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, err
	}
	record, workerErr := c.invoke(ctx, req)
	if workerErr != nil {
		if _, appendErr := c.Ledger.Append(ctx, int64(len(turns)), record); appendErr != nil {
			return TurnRecord{}, fmt.Errorf("record worker interruption: %w (worker: %v)", appendErr, workerErr)
		}
		return record, workerErr
	}
	if err := PublishCheckpoint(ctx, repo, record); err != nil {
		blocked := record
		blocked.Outcome = OutcomeBlocked
		blocked.Progress = false
		blocked.Blocker = err.Error()
		if _, appendErr := c.Ledger.Append(ctx, int64(len(turns)), blocked); appendErr != nil {
			return TurnRecord{}, fmt.Errorf("record checkpoint publication failure: %w", appendErr)
		}
		return blocked, err
	}
	if _, err := c.Ledger.Append(ctx, int64(len(turns)), record); err != nil {
		return TurnRecord{}, err
	}
	return record, nil
}
