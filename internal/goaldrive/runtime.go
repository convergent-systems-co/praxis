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
}

func (r Runtime) Execute(ctx context.Context, invocation InvocationRequest) (TurnRecord, error) {
	if invocation.Input.Kind != contracts.GoalInputID || invocation.Input.GoalID == "" {
		return TurnRecord{}, ErrGoalExecutionInput
	}
	if invocation.GoalVersion == "" {
		return TurnRecord{}, ErrExactGoalVersionRequired
	}
	if invocation.Mode != ModeSupervised {
		return TurnRecord{}, ErrSupervisedRuntimeOption
	}
	if invocation.MaxTurns > 1 {
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
	turns, err := r.Controller.Ledger.Load(ctx, invocation.Input.GoalID, invocation.GoalVersion)
	if err != nil {
		return TurnRecord{}, fmt.Errorf("recover Goal-drive ledger: %w", err)
	}
	turnID := invocation.InvocationID + ":turn:" + strconv.Itoa(len(turns)+1)
	turnCtx := ctx
	if invocation.TurnTimeout > 0 {
		var cancel context.CancelFunc
		turnCtx, cancel = context.WithTimeout(ctx, invocation.TurnTimeout)
		defer cancel()
	}
	return r.Controller.ExecuteTurnWithRepository(turnCtx, TurnRequest{
		GoalID: invocation.Input.GoalID, GoalVersion: invocation.GoalVersion,
		InvocationID: invocation.InvocationID, TurnID: turnID,
		GraphID: r.GraphID, GraphVersion: r.GraphVersion,
		ProviderID: invocation.ProviderID, Mode: ModeSupervised,
		NoPush:       invocation.NoPush,
		GoalBaseline: &baseline,
	}, r.Repository)
}
