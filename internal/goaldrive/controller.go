package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	ErrUnsafeRepository       = errors.New("Goal-drive repository state is not safe for a worker turn")
	ErrNoProgressLimit        = errors.New("Goal-drive no-progress limit reached")
	ErrGoalSettled            = errors.New("Goal generation is settled")
	ErrGoalCompletionPending  = errors.New("Goal completion candidate awaits evaluation and settlement")
	ErrSupervisedTerminated   = errors.New("supervised Goal-drive invocation terminated after its persisted checkpoint")
	ErrInvocationModeMismatch = errors.New("Goal-drive invocation mode cannot change across turns")
	ErrHumanAuthorityGate     = errors.New("HUMAN_AUTHORITY_REQUIRED: authority gate cannot be executed by a worker")
	ErrSafetyActivation       = errors.New("WorkPlan safety activation is absent or inconsistent")
	ErrPlanAuthority          = errors.New("WorkPlan governing authority is absent, revoked, or inconsistent")
)

// PlanAuthorityVerifier proves that the authority governing a safety-bearing
// accepted WorkPlan is currently effective. Historical authority evidence
// stays immutable and inspectable, but only current authority may govern a new
// selection, execution, completion, or gate request.
//
// The same store is also the authenticated root for the two other facts a
// safety-bearing generation must not take from a self-declared or plaintext
// source: whether a Goal is safety-bearing at all (I9) and whether a ledger
// completion is genuine (I11). One interface, one trust root: the GoalStore's
// installation-storage-key-authenticated records.
type PlanAuthorityVerifier interface {
	VerifyGoverningAuthority(context.Context, goals.GoalBaseline, time.Time) error
	// GoalSafetyClassified reports durable, authenticated Goal classification.
	GoalSafetyClassified(context.Context, string) (bool, error)
	// SealCompletion authenticates the exact bytes of a qualified completion.
	SealCompletion(context.Context, goals.GoalBaseline, []byte, time.Time) (string, error)
	// LoadSealedCompletion returns the sealed bytes for a completion digest or
	// contracts.ErrCompletionUnauthenticated.
	LoadSealedCompletion(context.Context, goals.GoalBaseline, string, time.Time) ([]byte, error)
	// VerifyGateCompletion re-resolves a gate completion's request, decision,
	// dossier and ceremony lineage; contracts.ErrGateAuthorityNotEffective
	// marks an authentic but no-longer-current decision.
	VerifyGateCompletion(context.Context, goals.GoalBaseline, contracts.WorkCandidate, []contracts.GovernedArtifactEvidence, string, string, time.Time) error
}

type SafetyActivationVerifier interface {
	Verify(context.Context, contracts.WorkPlanSafetyBinding) error
}

type AuthorityGateCoordinator interface {
	ReconcileAuthorityGate(context.Context, goals.GoalBaseline, contracts.WorkCandidate, []contracts.GovernedArtifactEvidence, time.Time) (contracts.AuthorityGateResult, error)
}

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
	Context        *WorkerContext         `json:"context,omitempty"`
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
	// Lease is the admitted turn's lease handle; publication and recording
	// require it to still be held (#162, #163).
	Lease                            *TurnLease
	Mode                             ExecutionMode
	WorkCandidates                   []contracts.WorkCandidate
	WorkRelationships                []contracts.WorkRelationship
	GoalBaseline                     *goals.GoalBaseline
	RepositoryPath, RepositoryBranch string
	ValidationDeclared               bool
	DeclaredValidation               string
	Recovery                         *WorkerRecoveryContext
	Context                          *WorkerContext
}

type Controller struct {
	Ledger            Ledger
	Worker            Worker
	Providers         *Registry
	AuthorityRequests AuthorityRequestReader
	NoProgressLimit   int
	Activity          *ActivityLog
	SafetyActivation  SafetyActivationVerifier
	// GoverningAuthority is required for safety-bearing plans and is applied
	// at every turn boundary, so restart, recovery and continuous subsequent
	// turns all re-establish current authority.
	GoverningAuthority PlanAuthorityVerifier
	AuthorityGates     AuthorityGateCoordinator
}

// verifyGoverningAuthority is the shared execution-boundary predicate.
func (c Controller) verifyGoverningAuthority(ctx context.Context, baseline *goals.GoalBaseline) error {
	if !isSafetyBearing(baseline) {
		return nil
	}
	if c.GoverningAuthority == nil {
		return fmt.Errorf("%w: verifier is not configured", ErrPlanAuthority)
	}
	if err := c.GoverningAuthority.VerifyGoverningAuthority(ctx, *baseline, time.Now().UTC()); err != nil {
		return fmt.Errorf("%w: %v", ErrPlanAuthority, err)
	}
	return nil
}

func findCandidate(candidates []contracts.WorkCandidate, id string) (contracts.WorkCandidate, bool) {
	for _, candidate := range candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return contracts.WorkCandidate{}, false
}

// refuseGateDispatch is the provider-dispatch fence. Every route to a worker
// resolves it through Controller.worker, so an authority-gate objective can
// never reach a provider however the objective was supplied (selection,
// explicit, recovery, pinned, continuation). It classifies against the
// immutable accepted plan as well as the materialized candidates, so it does
// not depend on the caller having populated WorkCandidates.
func refuseGateDispatch(req TurnRequest) error {
	if req.ChildObjective == "" {
		return nil
	}
	if req.GoalBaseline != nil && req.GoalBaseline.WorkPlan != nil {
		if candidate, ok := findCandidate(req.GoalBaseline.WorkPlan.Candidates, req.ChildObjective); ok && candidate.Kind == contracts.WorkCandidateAuthorityGate {
			return fmt.Errorf("%w: %s", ErrHumanAuthorityGate, candidate.ID)
		}
	}
	if candidate, ok := findCandidate(req.WorkCandidates, req.ChildObjective); ok && candidate.Kind == contracts.WorkCandidateAuthorityGate {
		return fmt.Errorf("%w: %s", ErrHumanAuthorityGate, candidate.ID)
	}
	return nil
}

func (c Controller) ExecuteTurn(ctx context.Context, req TurnRequest) (TurnRecord, error) {
	_, req, err := c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, err
	}
	if err := c.emit(ctx, ActivityExecutionStarted, req, map[string]string{"mode": string(req.Mode)}); err != nil {
		return TurnRecord{}, fmt.Errorf("record execution start: %w", err)
	}
	if err := c.emit(ctx, ActivityWorkSelected, req, map[string]string{"objective": req.ChildObjective}); err != nil {
		return TurnRecord{}, fmt.Errorf("record work selection: %w", err)
	}
	if err := c.emitEnvelope(ctx, req); err != nil {
		return TurnRecord{}, err
	}
	record, workerErr, _ := c.invoke(ctx, req)
	if err := c.recordTurn(ctx, &record); err != nil {
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
	safety, err := c.safetyBearing(ctx, req.GoalBaseline)
	if err != nil {
		return nil, TurnRequest{}, err
	}
	if safety {
		if c.SafetyActivation == nil {
			return nil, TurnRequest{}, fmt.Errorf("%w: verifier is not configured", ErrSafetyActivation)
		}
		if err := c.SafetyActivation.Verify(ctx, *req.GoalBaseline.WorkPlan.Safety); err != nil {
			return nil, TurnRequest{}, fmt.Errorf("%w: %v", ErrSafetyActivation, err)
		}
		if err := c.verifyGoverningAuthority(ctx, req.GoalBaseline); err != nil {
			return nil, TurnRequest{}, err
		}
		// Selector input for a safety-bearing plan derives only from the
		// baseline's accepted plan (whose exact bytes the authority verifier
		// just proved equal to the persisted generation), never from a
		// caller-supplied candidate slice.
		plan := req.GoalBaseline.WorkPlan
		req.WorkCandidates = append([]contracts.WorkCandidate(nil), plan.Candidates...)
		req.WorkRelationships = append([]contracts.WorkRelationship(nil), plan.Relationships...)
	}
	if req.ChildObjective == "" {
		if len(req.WorkCandidates) == 0 && req.GoalBaseline != nil {
			candidates, relationships, err := MaterializeGoalWork(*req.GoalBaseline)
			if err != nil {
				return nil, TurnRequest{}, err
			}
			req.WorkCandidates, req.WorkRelationships = candidates, relationships
		}
		// Eligibility derives from durable, controller-recorded unit
		// completions overlaid on the immutable plan (#158).
		// Completions are consumed only through the authenticating boundary:
		// sealed evidence, gate lineage re-resolved, current authority.
		effective, err := c.effectiveCompletions(ctx, req.GoalBaseline, req.GoalID, req.GoalVersion)
		if err != nil {
			return nil, TurnRequest{}, fmt.Errorf("load unit completions: %w", err)
		}
		completions := effective.Effective
		req.WorkCandidates = ApplyCompletions(req.WorkCandidates, completions)
		goalState, err := c.Ledger.LoadGoalCompletion(ctx, req.GoalID, req.GoalVersion)
		if err != nil {
			return nil, TurnRequest{}, fmt.Errorf("load Goal completion state: %w", err)
		}
		if goalState.Decision != nil {
			return nil, TurnRequest{}, fmt.Errorf("%w: Goal %s/%s is settled %s by %s at %s%s", ErrGoalSettled, req.GoalID, req.GoalVersion, goalState.Decision.Status, goalState.Decision.DecidedBy.ID, goalState.Decision.DecidedAt.UTC().Format(time.RFC3339), successorHint(goalState.Succession))
		}
		if goalState.Candidate != nil {
			return nil, TurnRequest{}, fmt.Errorf("%w: Goal %s/%s completion candidate from turn %s awaits evaluation and settlement: praxis goals-lifecycle --operation=complete --goal-id=%s --goal-version=%s", ErrGoalCompletionPending, req.GoalID, req.GoalVersion, goalState.Candidate.TurnID, req.GoalID, req.GoalVersion)
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
		if candidate.Kind == contracts.WorkCandidateAuthorityGate {
			return nil, TurnRequest{}, c.coordinateGate(ctx, req, candidate, completions)
		}
		req.ChildObjective = candidate.ID
	} else if req.GoalBaseline != nil && req.GoalBaseline.WorkPlan != nil {
		// An explicit objective (caller-supplied, pinned, or recovered) is
		// materialized and classified against the accepted plan before any
		// provider is resolved, exactly like a selected one.
		candidate, found := findCandidate(req.GoalBaseline.WorkPlan.Candidates, req.ChildObjective)
		if safety && !found {
			return nil, TurnRequest{}, fmt.Errorf("objective %q is not a candidate of the accepted safety-bearing plan", req.ChildObjective)
		}
		if found && (candidate.Kind == contracts.WorkCandidateAuthorityGate || safety) {
			effective, err := c.effectiveCompletions(ctx, req.GoalBaseline, req.GoalID, req.GoalVersion)
			if err != nil {
				return nil, TurnRequest{}, fmt.Errorf("load unit completions: %w", err)
			}
			completions := effective.Effective
			for _, completion := range completions {
				if completion.UnitID == candidate.ID {
					return nil, TurnRequest{}, fmt.Errorf("objective %q is already complete (turn %s)", candidate.ID, completion.TurnID)
				}
			}
			req.WorkCandidates = ApplyCompletions(req.WorkCandidates, completions)
			if candidate.Kind == contracts.WorkCandidateAuthorityGate {
				return nil, TurnRequest{}, c.coordinateGate(ctx, req, candidate, completions)
			}
			// I3/I11: an explicit, pinned or recovered objective is new work
			// like a selected one. It must be runnable against the current,
			// authenticated completion state, so a unit whose hard prerequisite
			// (a gate whose decision was revoked, or an incomplete gate) is not
			// currently decided cannot be resumed on the strength of history.
			assessment, assessErr := contracts.AssessWorkCandidates(req.WorkCandidates, req.WorkRelationships)
			if assessErr != nil {
				return nil, TurnRequest{}, assessErr
			}
			for _, item := range assessment.Candidates {
				if item.Candidate.ID == candidate.ID && item.Readiness != contracts.WorkReady {
					return nil, TurnRequest{}, fmt.Errorf("objective %q is not currently runnable: blocked by %s", candidate.ID, strings.Join(item.BlockedBy, ", "))
				}
			}
		}
	}
	if err := refuseGateDispatch(req); err != nil {
		return nil, TurnRequest{}, err
	}
	worker, err := c.worker(req)
	if err != nil {
		return nil, TurnRequest{}, err
	}
	// The worker must be able to bring about every consequence the
	// checkpoint contract requires; otherwise the turn fails here, before
	// any implementation begins, with durable evidence of the class
	// "checkpoint-required action unavailable to worker".
	if err := CheckCapabilities(worker, req.ProviderID, RequiredRepositoryCapabilities()); err != nil {
		var capErr *CapabilityError
		if errors.As(err, &capErr) {
			if emitErr := c.emit(ctx, ActivityCapabilityUnsatisfied, req, map[string]string{"provider": req.ProviderID, "required": joinCapabilities(capErr.Required), "granted": joinCapabilities(capErr.Granted), "missing": joinCapabilities(capErr.Missing), "evidence_class": "checkpoint-required action unavailable to worker"}); emitErr != nil {
				return nil, TurnRequest{}, emitErr
			}
		}
		return nil, TurnRequest{}, err
	}
	if req.GoalBaseline != nil && req.Context == nil {
		granted := RequiredRepositoryCapabilities()
		if declaring, ok := worker.(CapabilityDeclaringWorker); ok {
			granted = declaring.Capabilities()
		}
		if len(req.WorkCandidates) == 0 {
			candidates, relationships, err := MaterializeGoalWork(*req.GoalBaseline)
			if err != nil {
				return nil, TurnRequest{}, err
			}
			req.WorkCandidates, req.WorkRelationships = candidates, relationships
		}
		var envelope *WorkerExecutionEnvelope
		if declaring, ok := worker.(EnvelopeDeclaringWorker); ok {
			envelope = declaring.ExecutionEnvelope()
		}
		workerContext, err := BuildWorkerContext(req.GoalBaseline, req.WorkCandidates, req.WorkRelationships, req.ChildObjective, WorkerRepositoryContext{Path: req.RepositoryPath, Branch: req.RepositoryBranch, StartHead: req.StartHead}, granted, req.ValidationDeclared, req.DeclaredValidation, req.Recovery, envelope)
		if err != nil {
			return nil, TurnRequest{}, err
		}
		req.Context = workerContext
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

// coordinateGate routes an authority-gate objective, however it arrived, to
// controller-owned gate coordination. It never resolves a provider. It always
// returns a non-nil error: a pending request, a recorded completion (start a
// fresh turn), or a refusal.
func (c Controller) coordinateGate(ctx context.Context, req TurnRequest, candidate contracts.WorkCandidate, completions []UnitCompletion) error {
	if c.AuthorityGates == nil || req.GoalBaseline == nil {
		return fmt.Errorf("%w: %s", ErrHumanAuthorityGate, candidate.ID)
	}
	var artifacts []contracts.GovernedArtifactEvidence
	for _, completion := range completions {
		artifacts = append(artifacts, completion.GovernedArtifacts...)
	}
	result, gateErr := c.AuthorityGates.ReconcileAuthorityGate(ctx, *req.GoalBaseline, candidate, artifacts, time.Now().UTC())
	if gateErr != nil {
		return gateErr
	}
	if !result.Approved {
		if emitErr := c.emit(ctx, ActivityAuthorityRequired, req, map[string]string{"gate": candidate.ID}); emitErr != nil {
			return emitErr
		}
		return &AuthorityRequiredError{Requests: []contracts.AuthorityRequest{result.Request}}
	}
	// Recording a completion is a new consequence: every current predicate the
	// effect requires must hold at this instant, not only inside the
	// coordinator that just answered.
	if err := c.authorizeEffect(ctx, req, "gate-completion", nil, TurnRecord{}); err != nil {
		return err
	}
	requestDigest, _ := result.Request.Digest()
	completion := UnitCompletion{GoalID: req.GoalID, GoalVersion: req.GoalVersion, UnitID: candidate.ID, InvocationID: req.InvocationID, TurnID: req.TurnID, EndHead: "authority:" + result.DecisionDigest, CompletedAt: time.Now().UTC(), AuthorityGate: true, SpecificationDigest: candidate.SourceDigest, Evidence: []string{"dossier:" + result.Request.DossierDigest, "authority-request:" + requestDigest, "authority-decision:" + result.DecisionDigest}}
	if err := c.recordCompletion(ctx, req.GoalBaseline, completion); err != nil {
		return fmt.Errorf("record authority gate completion: %w", err)
	}
	return fmt.Errorf("%w: %s resolved and recorded; start a fresh turn", ErrHumanAuthorityGate, candidate.ID)
}

func (c Controller) worker(req TurnRequest) (Worker, error) {
	if err := refuseGateDispatch(req); err != nil {
		return nil, err
	}
	if c.Providers != nil {
		return c.Providers.Resolve(req.ProviderID)
	}
	if c.Worker == nil {
		return nil, errors.New("Goal-drive worker is required")
	}
	return c.Worker, nil
}

func (c Controller) invoke(ctx context.Context, req TurnRequest) (TurnRecord, error, bool) {
	worker, err := c.worker(req)
	if err != nil {
		return TurnRecord{}, err, false
	}
	workerReq := c.workerRequest(req)
	if err := c.emit(ctx, ActivityActionStarted, req, map[string]string{"action": "provider.execute"}); err != nil {
		return TurnRecord{}, err, false
	}
	result, workerErr := worker.Execute(ctx, workerReq)
	// Durable writes after the worker returns must survive the cancellation
	// that may have interrupted it (#163).
	ctx = context.WithoutCancel(ctx)
	if workerErr != nil {
		if err := c.emit(ctx, ActivityActionFailed, req, map[string]string{"action": "provider.execute", "error": workerErr.Error()}); err != nil {
			return TurnRecord{}, err, true
		}
	} else {
		if err := c.emit(ctx, ActivityActionCompleted, req, map[string]string{"action": "provider.execute"}); err != nil {
			return TurnRecord{}, err, true
		}
	}
	base := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, Mode: req.Mode, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, EndHead: result.EndHead, ExecutorID: result.ExecutorID, CheckpointEvidence: result.CheckpointEvidence, RetryOf: recoveredTurn(req.Recovery)}
	bindRecoveryLineage(&base, req.Recovery)
	if workerErr != nil {
		base.Outcome, base.Blocker = OutcomeBlocked, workerErr.Error()
		return base, workerErr, true
	}
	if result.Outcome == OutcomeContinue || result.Outcome == OutcomeComplete {
		progress, progressErr := contracts.ValidateCheckpointProgress(req.StartHead, result.EndHead, true, result.CheckpointValid)
		if progressErr != nil {
			if result.Outcome == OutcomeComplete {
				return TurnRecord{}, progressErr, true
			}
			result.Outcome = OutcomeNoProgress
		} else if !progress {
			if result.Outcome == OutcomeComplete {
				return TurnRecord{}, errors.New("COMPLETE turn made no progress"), true
			}
			result.Outcome = OutcomeNoProgress
		}
		base.Outcome, base.Progress = result.Outcome, progress
		return base, nil, true
	}
	base.Outcome = result.Outcome
	return base, nil, true
}

func recoveredTurn(recovery *WorkerRecoveryContext) string {
	if recovery == nil {
		return ""
	}
	return recovery.RecoveredTurnID
}

func (c Controller) workerRequest(req TurnRequest) WorkerRequest {
	return WorkerRequest{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, ProviderID: req.ProviderID, Context: req.Context, Activity: c.Activity, ActivityActor: c.Ledger.Actor}
}

// emitEnvelope records the execution envelope the worker context carries,
// before the provider starts, so the conditions a turn ran under are
// durable evidence (#170).
func (c Controller) emitEnvelope(ctx context.Context, req TurnRequest) error {
	if req.Context == nil || req.Context.Authority.Envelope == nil {
		return nil
	}
	envelope := req.Context.Authority.Envelope
	return c.emit(ctx, ActivityExecutionEnvelope, req, map[string]string{"interactive": fmt.Sprint(envelope.Interactive), "tools": strings.Join(envelope.Tools, " "), "shell_policy": envelope.ShellPolicy, "denial_policy": envelope.DenialPolicy})
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

func successorHint(succession *GoalSuccession) string {
	if succession == nil {
		return "; a successor generation may be created with praxis goals-lifecycle --operation=succeed"
	}
	return "; successor generation " + succession.SuccessorVersion + " carries the way forward"
}
