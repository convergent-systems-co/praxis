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

type Worker interface {
	Execute(context.Context, WorkerRequest) (WorkerResult, error)
}

type WorkerRequest struct {
	GoalID         string `json:"goal_id"`
	GoalVersion    string `json:"goal_version"`
	TurnID         string `json:"turn_id"`
	ChildObjective string `json:"child_objective"`
	GraphID        string `json:"graph_id"`
	GraphVersion   string `json:"graph_version"`
	StartHead      string `json:"start_head"`
}

type WorkerResult struct {
	Outcome            Outcome  `json:"outcome"`
	EndHead            string   `json:"end_head,omitempty"`
	CheckpointValid    bool     `json:"checkpoint_valid"`
	CheckpointEvidence []string `json:"checkpoint_evidence,omitempty"`
	ExecutorID         string   `json:"executor_id,omitempty"`
}

type TurnRequest struct {
	GoalID, GoalVersion, TurnID, ChildObjective, GraphID, GraphVersion, StartHead string
	Repository                                                                    contracts.RepositoryState
}

type Controller struct {
	Ledger          Ledger
	Worker          Worker
	NoProgressLimit int
}

func (c Controller) ExecuteTurn(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	turns, err := c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, err
	}
	record, workerErr := c.invoke(ctx, req)
	if _, err := c.Ledger.Append(ctx, int64(len(turns)), record); err != nil {
		if workerErr != nil {
			return TurnRecord{}, fmt.Errorf("record worker interruption: %w (worker: %v)", err, workerErr)
		}
		return TurnRecord{}, err
	}
	return record, workerErr
}

func (c Controller) prepare(ctx context.Context, req TurnRequest) ([]TurnRecord, error) {
	if c.Worker == nil {
		return nil, errors.New("Goal-drive worker is required")
	}
	if req.Repository != contracts.RepositorySynced {
		return nil, fmt.Errorf("%w: %s", ErrUnsafeRepository, req.Repository)
	}
	turns, err := c.Ledger.Load(ctx, req.GoalID, req.GoalVersion)
	if err != nil {
		return nil, err
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
		return nil, ErrNoProgressLimit
	}
	return turns, nil
}

func (c Controller) invoke(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	result, workerErr := c.Worker.Execute(ctx, WorkerRequest{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead})
	base := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, EndHead: result.EndHead, ExecutorID: result.ExecutorID, CheckpointEvidence: result.CheckpointEvidence}
	if workerErr != nil {
		base.Outcome, base.Blocker = OutcomeBlocked, workerErr.Error()
		return base, workerErr
	}
	if result.Outcome == OutcomeContinue || result.Outcome == OutcomeComplete {
		progress, progressErr := contracts.ValidateCheckpointProgress(req.StartHead, result.EndHead, true, result.CheckpointValid)
		if progressErr != nil {
			if result.Outcome == OutcomeComplete {
				return TurnRecord{}, progressErr
			}
			result.Outcome = OutcomeNoProgress
		} else if !progress {
			if result.Outcome == OutcomeComplete {
				return TurnRecord{}, errors.New("COMPLETE turn made no progress")
			}
			result.Outcome = OutcomeNoProgress
		}
		base.Outcome, base.Progress = result.Outcome, progress
		return base, nil
	}
	base.Outcome = result.Outcome
	return base, nil
}
