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
	GoalID         string                 `json:"goal_id"`
	GoalVersion    string                 `json:"goal_version"`
	InvocationID   string                 `json:"invocation_id"`
	TurnID         string                 `json:"turn_id"`
	ChildObjective string                 `json:"child_objective"`
	GraphID        string                 `json:"graph_id"`
	GraphVersion   string                 `json:"graph_version"`
	StartHead      string                 `json:"start_head"`
	ProviderID     string                 `json:"provider_id"`
	Activity       *ActivityLog           `json:"-"`
	ActivityActor  contracts.PrincipalRef `json:"-"`
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
	NoPush                                                                                      bool
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
	Activity          *ActivityLog
}

func (c Controller) ExecuteTurn(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	turns, req, err := c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, err
	}
	if err := c.emit(ctx, ActivityExecutionStarted, req, map[string]string{"mode": string(req.Mode)}); err != nil {
		return TurnRecord{}, fmt.Errorf("record execution start: %w", err)
	}
	if err := c.emit(ctx, ActivityWorkSelected, req, map[string]string{"objective": req.ChildObjective}); err != nil {
		return TurnRecord{}, fmt.Errorf("record work selection: %w", err)
	}
	record, workerErr := c.invoke(ctx, req)
	if _, err := c.Ledger.Append(ctx, int64(len(turns)), record); err != nil {
		if workerErr != nil {
			return TurnRecord{}, fmt.Errorf("record worker interruption: %w (worker: %v)", err, workerErr)
		}
		return TurnRecord{}, err
	}
	if err := c.emitTurnOutcome(ctx, req, record, workerErr); err != nil {
		return TurnRecord{}, fmt.Errorf("record execution outcome: %w", err)
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
					if emitErr := c.emit(ctx, ActivityAuthorityRequired, req, map[string]string{"count": fmt.Sprint(len(pending))}); emitErr != nil {
						return nil, TurnRequest{}, fmt.Errorf("record authority requirement: %w", emitErr)
					}
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
	if c.Activity != nil {
		events, loadErr := c.Activity.Load(ctx, req.InvocationID, req.TurnID, 0)
		if loadErr != nil {
			return nil, TurnRequest{}, fmt.Errorf("recover supervision activity before execution: %w", loadErr)
		}
		for _, event := range events {
			if event.Type == ActivityAuthorityRequired {
				if emitErr := c.emit(ctx, ActivityAuthorityResolved, req, map[string]string{"objective": req.ChildObjective}); emitErr != nil {
					return nil, TurnRequest{}, fmt.Errorf("record authority resolution: %w", emitErr)
				}
				break
			}
		}
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
	workerReq := c.workerRequest(req)
	if err := c.emit(ctx, ActivityActionStarted, req, map[string]string{"action": "provider.execute"}); err != nil {
		return TurnRecord{}, err
	}
	result, workerErr := worker.Execute(ctx, workerReq)
	if workerErr != nil {
		if err := c.emit(ctx, ActivityActionFailed, req, map[string]string{"action": "provider.execute", "error": workerErr.Error()}); err != nil {
			return TurnRecord{}, err
		}
	} else {
		if err := c.emit(ctx, ActivityActionCompleted, req, map[string]string{"action": "provider.execute"}); err != nil {
			return TurnRecord{}, err
		}
	}
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

func (c Controller) workerRequest(req TurnRequest) WorkerRequest {
	return WorkerRequest{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, ProviderID: req.ProviderID, Activity: c.Activity, ActivityActor: c.Ledger.Actor}
}

func (c Controller) emit(ctx context.Context, typ ActivityType, req TurnRequest, data map[string]string) error {
	if c.Activity == nil {
		return nil
	}
	_, err := c.Activity.Emit(ctx, typ, c.workerRequest(req), c.Ledger.Actor, contracts.TrustObserved, "praxis.controller", data)
	return err
}

func (c Controller) emitTurnOutcome(ctx context.Context, req TurnRequest, record TurnRecord, workerErr error) error {
	if c.Activity == nil {
		return nil
	}
	if errors.Is(workerErr, ErrExecutionSuspended) {
		if err := c.emit(ctx, ActivitySuspended, req, map[string]string{"reason": "human intervention"}); err != nil {
			return err
		}
	}
	if errors.Is(workerErr, ErrExecutionCancelled) {
		if err := c.emit(ctx, ActivityCancelled, req, map[string]string{"reason": "human intervention"}); err != nil {
			return err
		}
	}
	if workerErr != nil || record.Blocker != "" {
		if err := c.emit(ctx, ActivityBlockerDetected, req, map[string]string{"blocker": record.Blocker}); err != nil {
			return err
		}
	}
	if record.Progress {
		if err := c.emit(ctx, ActivityWorkProgress, req, map[string]string{"end_head": record.EndHead}); err != nil {
			return err
		}
		if err := c.emit(ctx, ActivityCheckpointCreated, req, map[string]string{"end_head": record.EndHead, "published": fmt.Sprint(record.CheckpointPublished)}); err != nil {
			return err
		}
	}
	if record.Outcome == OutcomeComplete {
		if err := c.emit(ctx, ActivityCompletionClaimed, req, map[string]string{"outcome": string(record.Outcome)}); err != nil {
			return err
		}
		if record.Progress {
			if err := c.emit(ctx, ActivityCompletionQualified, req, map[string]string{"outcome": string(record.Outcome)}); err != nil {
				return err
			}
		}
	}
	state := string(record.Outcome)
	if workerErr != nil {
		state = "blocked"
	}
	return c.emit(ctx, ActivityExecutionStateChanged, req, map[string]string{"state": state})
}
