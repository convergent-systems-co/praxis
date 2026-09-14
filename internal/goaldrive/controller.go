package goaldrive

import (
	"context"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrUnsafeRepository = errors.New("Goal-drive repository state is not safe for a worker turn")
	ErrNoProgressLimit  = errors.New("Goal-drive no-progress limit reached")
)

// Worker receives exactly one bounded objective. It has no ledger or
// repository-synchronization authority.
type Worker interface {
	Execute(context.Context, WorkerRequest) (WorkerResult, error)
}

type WorkerRequest struct {
	GoalID         string
	GoalVersion    string
	TurnID         string
	ChildObjective string
	GraphID        string
	GraphVersion   string
	StartHead      string
}

type WorkerResult struct {
	Outcome            Outcome
	EndHead            string
	CheckpointValid    bool
	CheckpointEvidence []string
}

type TurnRequest struct {
	GoalID         string
	GoalVersion    string
	TurnID         string
	ChildObjective string
	GraphID        string
	GraphVersion   string
	StartHead      string
	Repository     contracts.RepositoryState
}

type Controller struct {
	Ledger          Ledger
	Worker          Worker
	NoProgressLimit int
}

// ExecuteTurn owns one worker boundary. It derives the aggregate version from
// durable ledger state, so a caller cannot mint a later turn by asserting an
// incorrect version.
func (c Controller) ExecuteTurn(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	if c.Worker == nil {
		return TurnRecord{}, errors.New("Goal-drive worker is required")
	}
	if req.Repository != contracts.RepositorySynced {
		return TurnRecord{}, fmt.Errorf("%w: %s", ErrUnsafeRepository, req.Repository)
	}
	turns, err := c.Ledger.Load(ctx, req.GoalID, req.GoalVersion)
	if err != nil {
		return TurnRecord{}, err
	}
	limit := c.NoProgressLimit
	if limit <= 0 {
		limit = 1
	}
	noProgress := 0
	for _, turn := range turns {
		if turn.ChildObjective == req.ChildObjective && turn.Outcome == OutcomeNoProgress {
			noProgress++
		}
	}
	if noProgress >= limit {
		return TurnRecord{}, ErrNoProgressLimit
	}
	result, workerErr := c.Worker.Execute(ctx, WorkerRequest{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead})
	if workerErr != nil {
		record := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, EndHead: result.EndHead, Outcome: OutcomeBlocked, Progress: false, CheckpointEvidence: result.CheckpointEvidence, Blocker: workerErr.Error()}
		_, appendErr := c.Ledger.Append(ctx, int64(len(turns)), record)
		if appendErr != nil {
			return TurnRecord{}, fmt.Errorf("record worker interruption: %w (worker: %v)", appendErr, workerErr)
		}
		return record, workerErr
	}
	if result.Outcome == OutcomeContinue || result.Outcome == OutcomeComplete {
		progress, progressErr := contracts.ValidateCheckpointProgress(req.StartHead, result.EndHead, true, result.CheckpointValid)
		if progressErr != nil {
			if result.Outcome == OutcomeContinue {
				result.Outcome = OutcomeNoProgress
			} else {
				return TurnRecord{}, progressErr
			}
		} else if !progress {
			if result.Outcome == OutcomeContinue {
				result.Outcome = OutcomeNoProgress
			} else {
				return TurnRecord{}, errors.New("COMPLETE turn made no progress")
			}
		}
		record := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, EndHead: result.EndHead, Outcome: result.Outcome, Progress: progress, CheckpointEvidence: result.CheckpointEvidence}
		if _, err := c.Ledger.Append(ctx, int64(len(turns)), record); err != nil {
			return TurnRecord{}, err
		}
		return record, nil
	}
	if result.Outcome == OutcomeNoProgress {
		result.CheckpointValid = false
	}
	record := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, EndHead: result.EndHead, Outcome: result.Outcome, Progress: false, CheckpointEvidence: result.CheckpointEvidence}
	if _, err := c.Ledger.Append(ctx, int64(len(turns)), record); err != nil {
		return TurnRecord{}, err
	}
	return record, nil
}
