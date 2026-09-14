package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrUnsafeRepository       = errors.New("Goal-drive repository state is not safe for a worker turn")
	ErrNoProgressLimit        = errors.New("Goal-drive no-progress limit reached")
	ErrSupervisedTerminated   = errors.New("supervised Goal-drive invocation terminated after its persisted checkpoint")
	ErrInvocationModeMismatch = errors.New("Goal-drive invocation mode cannot change across turns")
)

type AuthorityRequestReader interface {
	PendingAuthorityRequests(context.Context, string, string, time.Time) ([]contracts.AuthorityRequest, error)
}

type AuthorityRequiredError struct{ Requests []contracts.AuthorityRequest }

func (e *AuthorityRequiredError) Error() string {
	return fmt.Sprintf("Goal-drive authority required for %d pending request(s)", len(e.Requests))
}

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
	GoalID, GoalVersion, InvocationID, TurnID, ChildObjective, GraphID, GraphVersion, StartHead string
	Repository                                                                                  contracts.RepositoryState
	ProviderID                                                                                  string
	Mode                                                                                        ExecutionMode
	WorkCandidates                                                                              []contracts.WorkCandidate
	WorkRelationships                                                                           []contracts.WorkRelationship
	GoalBaseline                                                                                *goals.GoalBaseline
}

type Controller struct {
	Ledger            Ledger
	Worker            Worker
	Providers         *Registry
	AuthorityRequests AuthorityRequestReader
	NoProgressLimit   int
}

func (c Controller) ExecuteTurn(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	turns, req, err := c.prepare(ctx, req)
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

func (c Controller) prepare(ctx context.Context, req TurnRequest) ([]TurnRecord, TurnRequest, error) {
	if req.InvocationID == "" {
		return nil, TurnRequest{}, errors.New("Goal-drive invocation identity is required")
	}
	if req.Mode != ModeSupervised && req.Mode != ModeContinuous {
		return nil, TurnRequest{}, fmt.Errorf("unsupported Goal-drive execution mode %q", req.Mode)
	}
	if req.ChildObjective == "" {
		if len(req.WorkCandidates) == 0 && req.GoalBaseline != nil {
			candidates, relationships, err := MaterializeGoalWork(*req.GoalBaseline)
			if err != nil {
				return nil, TurnRequest{}, err
			}
			req.WorkCandidates, req.WorkRelationships = candidates, relationships
		}
		candidate, err := contracts.SelectRunnableWork(req.WorkCandidates, req.WorkRelationships)
		if err != nil {
			if errors.Is(err, contracts.ErrNoRunnableWork) && c.AuthorityRequests != nil {
				pending, readErr := c.AuthorityRequests.PendingAuthorityRequests(ctx, req.GoalID, req.GoalVersion, time.Now().UTC())
				if readErr != nil {
					return nil, TurnRequest{}, fmt.Errorf("load pending authority: %w", readErr)
				}
				if len(pending) > 0 {
					return nil, TurnRequest{}, &AuthorityRequiredError{Requests: pending}
				}
			}
			return nil, TurnRequest{}, err
		}
		req.ChildObjective = candidate.ID
	}
	if _, err := c.worker(req); err != nil {
		return nil, TurnRequest{}, err
	}
	if req.Repository != contracts.RepositorySynced {
		return nil, TurnRequest{}, fmt.Errorf("%w: %s", ErrUnsafeRepository, req.Repository)
	}
	turns, err := c.Ledger.Load(ctx, req.GoalID, req.GoalVersion)
	if err != nil {
		return nil, TurnRequest{}, err
	}
	limit := c.NoProgressLimit
	if limit <= 0 {
		limit = 1
	}
	noProgress := 0
	for _, turn := range turns {
		if turn.InvocationID == req.InvocationID {
			if turn.Mode != req.Mode {
				return nil, TurnRequest{}, ErrInvocationModeMismatch
			}
			if req.Mode == ModeSupervised && turn.Progress {
				return nil, TurnRequest{}, ErrSupervisedTerminated
			}
		}
		if turn.ChildObjective == req.ChildObjective && turn.Outcome == OutcomeNoProgress {
			noProgress++
		}
	}
	if noProgress >= limit {
		return nil, TurnRequest{}, ErrNoProgressLimit
	}
	return turns, req, nil
}

func (c Controller) worker(req TurnRequest) (Worker, error) {
	if c.Providers != nil {
		return c.Providers.Resolve(req.ProviderID)
	}
	if c.Worker == nil {
		return nil, errors.New("Goal-drive worker is required")
	}
	return c.Worker, nil
}

func (c Controller) invoke(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	worker, err := c.worker(req)
	if err != nil {
		return TurnRecord{}, err
	}
	result, workerErr := worker.Execute(ctx, WorkerRequest{GoalID: req.GoalID, GoalVersion: req.GoalVersion, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead})
	base := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, Mode: req.Mode, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, EndHead: result.EndHead, ExecutorID: result.ExecutorID, CheckpointEvidence: result.CheckpointEvidence}
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
