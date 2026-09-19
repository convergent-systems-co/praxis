package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
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
	// Recovery binds this invocation to the uncommitted consequence of an
	// earlier BLOCKED turn of the same objective (see --recover-turn).
	Recovery *WorkerRecoveryContext
	// Leases is the durable liveness store for turn admission (#162, #163);
	// nil disables liveness tracking (single-process use only). LeaseTTL is
	// the heartbeat expiry (DefaultLeaseTTL when zero).
	Leases   LeaseStore
	LeaseTTL time.Duration
	// admitted names the invocations this process admitted, so continuous
	// mode may run several turns under one single-use invocation identity.
	admitted map[string]bool
}

func (r Runtime) Execute(ctx context.Context, invocation InvocationRequest) (TurnRecord, error) {
	// One process, one invocation: continuous mode reuses the identity across
	// its own turns; any other process is refused (single-use).
	r.admitted = map[string]bool{}
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
	// Atomic durable admission (#162): unique turn number, single-use
	// invocation identity, exclusive lease on the checkout scope.
	scope, admittedHead := "", ""
	if located, ok := r.Repository.(LocatedRepository); ok {
		path, branch := located.Location()
		scope = ScopeKey(path, branch)
		if git, ok := r.Repository.(GitRepository); ok {
			if head, err := git.run(ctx, "rev-parse", "--verify", "HEAD^{commit}"); err == nil {
				admittedHead = strings.TrimSpace(head)
			}
		}
	}
	if r.admitted == nil {
		return TurnRecord{}, errors.New("executeOne requires Execute to own the admitted-invocation set")
	}
	lease, err := Admit(ctx, r.Controller.Ledger, r.Leases, invocation.Input.GoalID, invocation.GoalVersion, invocation.InvocationID, invocation.Mode, scope, admittedHead, r.LeaseTTL, r.admitted)
	if err != nil {
		return TurnRecord{}, err
	}
	r.admitted[invocation.InvocationID] = true
	turnID := lease.Admission.TurnID
	turnCtx, cancelTurn := context.WithCancel(ctx)
	defer cancelTurn()
	lease.started.Store(true)
	go lease.heartbeat(cancelTurn)
	if r.Activity != nil {
		// Durable before the announcement: the announced observe command
		// must always find at least this event (#155).
		request := WorkerRequest{GoalID: invocation.Input.GoalID, GoalVersion: invocation.GoalVersion, InvocationID: invocation.InvocationID, TurnID: turnID, ProviderID: invocation.ProviderID, GraphID: r.GraphID, GraphVersion: r.GraphVersion}
		if _, err := r.Activity.Emit(ctx, ActivityTurnAllocated, request, r.Controller.Ledger.Actor, contracts.TrustObserved, "praxis.controller", map[string]string{"mode": string(invocation.Mode)}); err != nil {
			return TurnRecord{}, fmt.Errorf("record turn allocation: %w", err)
		}
	}
	if r.OnTurnAllocated != nil {
		r.OnTurnAllocated(turnID)
	}
	if invocation.TurnTimeout > 0 {
		var cancel context.CancelFunc
		turnCtx, cancel = context.WithTimeout(turnCtx, invocation.TurnTimeout)
		defer cancel()
	}
	mode := invocation.Mode
	turnRequest := TurnRequest{
		GoalID: invocation.Input.GoalID, GoalVersion: invocation.GoalVersion,
		InvocationID: invocation.InvocationID, TurnID: turnID,
		GraphID: r.GraphID, GraphVersion: r.GraphVersion,
		ProviderID: invocation.ProviderID, Mode: mode,
		NoPush: invocation.NoPush, GoalBaseline: &baseline,
		Recovery: r.Recovery, ChildObjective: recoveryObjective(r.Recovery),
		Lease: lease,
	}
	record, execErr := r.Controller.ExecuteTurnWithRepository(turnCtx, turnRequest, r.Repository)
	if execErr != nil && record.Outcome == "" {
		if durableErr := r.recordPreflightFailure(context.WithoutCancel(ctx), turnRequest, execErr); durableErr != nil {
			execErr = fmt.Errorf("record durable preflight failure: %w (preflight: %v)", durableErr, execErr)
		}
	}
	disposition := string(record.Outcome)
	if errors.Is(execErr, ErrLeaseLost) {
		return record, execErr
	}
	if disposition == "" {
		disposition = "failed"
	}
	if execErr != nil && ctx.Err() != nil {
		disposition = "interrupted"
	}
	if releaseErr := lease.Release(context.WithoutCancel(ctx), disposition); releaseErr != nil && execErr == nil {
		return record, fmt.Errorf("release turn lease: %w", releaseErr)
	}
	return record, execErr
}

// recordPreflightFailure closes an allocated turn whose worker never ran.
// The blocker and terminal disposition are one atomic append, so restart can
// never observe only the allocation or only a non-terminal blocker.
func (r Runtime) recordPreflightFailure(ctx context.Context, req TurnRequest, preflightErr error) error {
	if r.Activity == nil {
		return nil
	}
	stage := "turn-preflight"
	if errors.Is(preflightErr, ErrUnsafeRepository) || strings.Contains(preflightErr.Error(), "repository") || strings.Contains(preflightErr.Error(), "HEAD") {
		stage = "repository-preflight"
	}
	retryOf := recoveredTurn(req.Recovery)
	for attempt := 0; attempt < 5; attempt++ {
		existing, err := r.Activity.Load(ctx, req.InvocationID, req.TurnID, 0)
		if err != nil {
			return err
		}
		for _, event := range existing {
			if event.Type == ActivityExecutionStateChanged {
				return nil
			}
		}
		base := ActivityRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, TurnID: req.TurnID, ProviderID: req.ProviderID, Actor: r.Controller.Ledger.Actor, Source: "praxis.controller", Trust: contracts.TrustObserved}
		blocker := base
		blocker.Type = ActivityBlockerDetected
		blocker.Data = map[string]string{"stage": stage, "blocker": preflightErr.Error(), "retry_of": retryOf}
		terminal := base
		terminal.Type = ActivityExecutionStateChanged
		terminal.Data = map[string]string{"state": "blocked", "stage": stage, "retry_of": retryOf}
		if _, err := r.Activity.AppendBatch(ctx, int64(len(existing)), []ActivityRecord{blocker, terminal}); !errors.Is(err, eventstore.ErrVersionConflict) {
			return err
		}
	}
	return eventstore.ErrVersionConflict
}

// recoveryObjective pins a recovery turn to the objective of the turn it
// recovers; the controller never reselects work while consequence is bound.
func recoveryObjective(recovery *WorkerRecoveryContext) string {
	if recovery == nil {
		return ""
	}
	return recovery.Objective
}
