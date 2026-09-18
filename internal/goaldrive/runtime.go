package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrExactGoalVersionRequired = errors.New("supervised Goal-drive requires an exact Goal Baseline version")
	ErrGoalExecutionInput       = errors.New("supervised Goal-drive accepts only an existing durable Goal identity")
	ErrSupervisedRuntimeOption  = errors.New("supervised Goal-drive option is not yet wired at the production runtime boundary")
	ErrContinuousTurnLimit      = errors.New("continuous Goal-drive bounded turn limit reached")
	ErrResumeRequired           = errors.New("Goal-drive execution requires an explicit supervision resume")
)

// BaselineStore is the durable Goal recovery boundary. Implementations must
// reload an immutable, digest-verified Goal Baseline; Runtime never creates a
// Goal or infers a generation from prose, PlanRef, or a latest pointer.
type BaselineStore interface {
	Load(context.Context, string, string, time.Time) (goals.GoalBaseline, error)
}

// Runtime composes the production supervised path after setup-time provider
// and key authority have been established. It performs exactly one bounded
// controller turn and never owns iteration or decomposition authority.
type Runtime struct {
	Controller   Controller
	Baselines    BaselineStore
	Repository   RepositoryAdapter
	GraphID      string
	GraphVersion string
	Activity     *ActivityLog
	// OnTurnAllocated is called with the exact turn identity the moment it
	// is allocated, before any provider execution, so the caller can make
	// the turn observable by identity before control blocks on the worker.
	OnTurnAllocated func(turnID string)
}

func (r Runtime) Execute(ctx context.Context, invocation InvocationRequest) (TurnRecord, error) {
	if invocation.Input.Kind != contracts.GoalInputID || invocation.Input.GoalID == "" {
		return TurnRecord{}, ErrGoalExecutionInput
	}
	if invocation.GoalVersion == "" {
		return TurnRecord{}, ErrExactGoalVersionRequired
	}
	if invocation.Mode == ModeSupervised && invocation.MaxTurns > 1 {
		return TurnRecord{}, fmt.Errorf("%w: multi-turn orchestration requires an explicit runtime boundary", ErrSupervisedRuntimeOption)
	}
	if r.Baselines == nil {
		return TurnRecord{}, errors.New("durable Goal Baseline store is required")
	}
	if r.Repository == nil {
		return TurnRecord{}, errors.New("controller repository adapter is required")
	}
	if r.GraphID == "" || r.GraphVersion == "" {
		return TurnRecord{}, errors.New("Goal-drive graph identity and version are required")
	}
	baseline, err := r.Baselines.Load(ctx, invocation.Input.GoalID, invocation.GoalVersion, time.Now().UTC())
	if err != nil {
		return TurnRecord{}, fmt.Errorf("recover exact Goal Baseline: %w", err)
	}
	if invocation.Mode == ModeSupervised {
		return r.executeOne(ctx, invocation, baseline)
	}
	limit := invocation.MaxTurns
	if limit <= 0 {
		limit = 8
	}
	var last TurnRecord
	for i := 0; i < limit; i++ {
		var err error
		last, err = r.executeOne(ctx, invocation, baseline)
		if err != nil || last.Outcome == OutcomeComplete || last.Outcome == OutcomeBlocked || last.Outcome == OutcomeNoProgress || last.Outcome == OutcomeUserDecisionRequired {
			return last, err
		}
	}
	return last, fmt.Errorf("%w: %d", ErrContinuousTurnLimit, limit)
}

func (r Runtime) executeOne(ctx context.Context, invocation InvocationRequest, baseline goals.GoalBaseline) (TurnRecord, error) {
	turns, err := r.Controller.Ledger.Load(ctx, invocation.Input.GoalID, invocation.GoalVersion)
	if err != nil {
		return TurnRecord{}, fmt.Errorf("recover Goal-drive ledger: %w", err)
	}
	if r.Activity != nil && len(turns) > 0 {
		previous := turns[len(turns)-1]
		events, loadErr := r.Activity.Load(ctx, invocation.InvocationID, previous.TurnID, 0)
		if loadErr != nil {
			return TurnRecord{}, fmt.Errorf("recover supervision state: %w", loadErr)
		}
		lastState := ActivityType("")
		resumeRequested := false
		for _, event := range events {
			if event.Type == ActivitySuspended || event.Type == ActivityCancelled || event.Type == ActivityResumed {
				lastState = event.Type
			}
			if event.Type == ActivityHumanCorrection && event.Data["operation"] == "resume" {
				resumeRequested = true
			}
		}
		if lastState == ActivitySuspended {
			if !resumeRequested {
				return TurnRecord{}, ErrResumeRequired
			}
			request := WorkerRequest{GoalID: previous.GoalID, GoalVersion: previous.GoalVersion, InvocationID: previous.InvocationID, TurnID: previous.TurnID, ProviderID: previous.ExecutorID}
			if r.Activity != nil {
				_, _ = r.Activity.Emit(ctx, ActivityResumed, request, r.Controller.Ledger.Actor, contracts.TrustObserved, "praxis.controller", map[string]string{"reason": "human resume decision"})
			}
		}
		if lastState == ActivityCancelled {
			return TurnRecord{}, ErrExecutionCancelled
		}
	}
	turnID := invocation.InvocationID + ":turn:" + strconv.Itoa(len(turns)+1)
	if r.OnTurnAllocated != nil {
		r.OnTurnAllocated(turnID)
	}
	turnCtx := ctx
	if invocation.TurnTimeout > 0 {
		var cancel context.CancelFunc
		turnCtx, cancel = context.WithTimeout(ctx, invocation.TurnTimeout)
		defer cancel()
	}
	mode := invocation.Mode
	return r.Controller.ExecuteTurnWithRepository(turnCtx, TurnRequest{
		GoalID: invocation.Input.GoalID, GoalVersion: invocation.GoalVersion,
		InvocationID: invocation.InvocationID, TurnID: turnID,
		GraphID: r.GraphID, GraphVersion: r.GraphVersion,
		ProviderID: invocation.ProviderID, Mode: mode,
		NoPush: invocation.NoPush, GoalBaseline: &baseline,
	}, r.Repository)
}
