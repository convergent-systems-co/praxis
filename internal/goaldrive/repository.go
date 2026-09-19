package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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

type DirtyStartRepository interface{ DirtyStartAllowed() bool }

// ConsequenceRepository fingerprints what the checkout carries beyond the
// published branch: uncommitted paths and unpublished commits.
type ConsequenceRepository interface {
	Fingerprint(ctx context.Context) (fingerprint string, files []string, commits []string, err error)
}

// recordConsequence binds the checkout's current consequence to a record
// that leaves work unpublished (a BLOCKED turn, or a progressing turn whose
// checkpoint was retained locally with --no-push) so recovery can later
// admit exactly that state. Best effort: a
// failure to fingerprint leaves the record without a binding, which makes
// the turn unrecoverable rather than wrongly recoverable.
func recordConsequence(ctx context.Context, repo RepositoryAdapter, record *TurnRecord) {
	fingerprinter, ok := repo.(ConsequenceRepository)
	if !ok {
		return
	}
	fingerprint, files, commits, err := fingerprinter.Fingerprint(ctx)
	if err != nil || (len(files) == 0 && len(commits) == 0) {
		return
	}
	record.ConsequenceFingerprint, record.ConsequenceFiles, record.ConsequenceCommits = fingerprint, files, commits
}

// RecoveryStartRepository admits a checkout that carries a bound recovery
// consequence: uncommitted changes or unpublished local commits whose
// fingerprint matches the recovered turn.
type RecoveryStartRepository interface {
	RecoveryStartAllowed() bool
	VerifyRecoveryConsequence(ctx context.Context) error
}

// DeclaredValidator is implemented by repository adapters that can run the
// repository's own declared validation (an executable ./.praxis/validate).
// Validation is controller-owned: it runs after the provider's local commit
// and before any checkpoint is accepted.
type DeclaredValidator interface {
	DeclaredValidation() (string, bool)
	RunDeclaredValidation(context.Context) (string, error)
}

// LocatedRepository exposes the exact path and branch a worker is bound to.
type LocatedRepository interface{ Location() (path, branch string) }

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
	if state == contracts.RepositoryDirty || state == contracts.RepositoryLocalAhead {
		// Uncommitted changes and unpublished local commits are admitted only
		// as the exact bound recovery consequence, or (dirty only) as a
		// persisted provider-workspace migration input.
		if recovery, ok := repo.(RecoveryStartRepository); ok && recovery.RecoveryStartAllowed() {
			if err := recovery.VerifyRecoveryConsequence(ctx); err != nil {
				return RepositorySnapshot{}, fmt.Errorf("%w: %s: %v", ErrUnsafeRepository, state, err)
			}
			return snapshot, nil
		}
		if allowed, ok := repo.(DirtyStartRepository); state == contracts.RepositoryDirty && ok && allowed.DirtyStartAllowed() {
			return snapshot, nil
		}
		return RepositorySnapshot{}, fmt.Errorf("%w: %s", ErrUnsafeRepository, state)
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
	if located, ok := repo.(LocatedRepository); ok && req.RepositoryPath == "" {
		req.RepositoryPath, req.RepositoryBranch = located.Location()
	}
	if validator, ok := repo.(DeclaredValidator); ok {
		req.DeclaredValidation, req.ValidationDeclared = validator.DeclaredValidation()
	}
	_, req, err = c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, err
	}
	if err := c.emit(ctx, ActivityExecutionStarted, req, map[string]string{"mode": string(req.Mode)}); err != nil {
		return TurnRecord{}, err
	}
	if req.Recovery != nil {
		if err := c.emit(ctx, ActivityRecoveryBound, req, map[string]string{"recovered_turn": req.Recovery.RecoveredTurnID, "fingerprint": req.Recovery.Fingerprint, "files": strings.Join(req.Recovery.Files, ","), "base_head": req.Recovery.BaseHead, "commits": strings.Join(req.Recovery.Commits, ","), "provenance": req.Recovery.Provenance, "blocker": req.Recovery.Blocker}); err != nil {
			return TurnRecord{}, err
		}
	}
	if err := c.emit(ctx, ActivityWorkSelected, req, map[string]string{"objective": req.ChildObjective}); err != nil {
		return TurnRecord{}, err
	}
	if err := c.emitEnvelope(ctx, req); err != nil {
		return TurnRecord{}, err
	}
	record, workerErr := c.invokeRepositoryTurn(ctx, req, repo)
	interrupted := ctx.Err() != nil
	// Everything durable after the worker returns runs on a context that
	// survives the interruption that may have stopped the worker (#163).
	ctx = context.WithoutCancel(ctx)
	if req.Lease != nil && !req.Lease.Held(ctx) {
		// Authority moved on (lease expired or reconciled): this process
		// records nothing and publishes nothing; reconciliation owns the
		// disposition (#163 B7).
		return record, fmt.Errorf("%w: turn %s", ErrLeaseLost, req.TurnID)
	}
	if workerErr != nil {
		if interrupted {
			record.Outcome, record.Progress = OutcomeBlocked, false
			record.Blocker = "execution interrupted (operator signal or turn timeout) while the provider was running; provider consequence unknown, checkout observed at interruption: " + workerErr.Error()
			if emitErr := c.emit(ctx, ActivityExecutionInterrupted, req, map[string]string{"reason": workerErr.Error(), "consequence": "unknown"}); emitErr != nil {
				return TurnRecord{}, emitErr
			}
			workerErr = errors.New(record.Blocker)
		}
		recordConsequence(ctx, repo, &record)
		if err := c.recordTurn(ctx, &record); err != nil {
			return TurnRecord{}, fmt.Errorf("record worker interruption: %w (worker: %v)", err, workerErr)
		}
		if emitErr := c.emitTurnOutcome(ctx, req, record, workerErr); emitErr != nil {
			return TurnRecord{}, emitErr
		}
		return record, workerErr
	}
	if !req.NoPush {
		if req.Lease != nil && !req.Lease.Held(ctx) {
			return record, fmt.Errorf("%w: turn %s; checkpoint %s not published", ErrLeaseLost, req.TurnID, record.EndHead)
		}
		if err := PublishCheckpoint(ctx, repo, record); err != nil {
			blocked := record
			blocked.Outcome = OutcomeBlocked
			blocked.Progress = false
			blocked.Blocker = err.Error()
			recordConsequence(ctx, repo, &blocked)
			if appendErr := c.recordTurn(ctx, &blocked); appendErr != nil {
				return TurnRecord{}, fmt.Errorf("record checkpoint publication failure: %w", appendErr)
			}
			if emitErr := c.emitTurnOutcome(ctx, req, blocked, err); emitErr != nil {
				return TurnRecord{}, emitErr
			}
			return blocked, err
		}
	}
	record.CheckpointPublished = record.Progress && !req.NoPush
	if record.Progress && !record.CheckpointPublished {
		recordConsequence(ctx, repo, &record)
	}
	record, err = c.settleCompletion(ctx, req, repo, record)
	if err != nil {
		return TurnRecord{}, err
	}
	if err := c.recordTurn(ctx, &record); err != nil {
		return TurnRecord{}, err
	}
	if err := c.emitTurnOutcome(ctx, req, record, nil); err != nil {
		return TurnRecord{}, err
	}
	return record, nil
}

func (c Controller) invokeRepositoryTurn(ctx context.Context, req TurnRequest, repo RepositoryAdapter) (TurnRecord, error) {
	worker, err := c.worker(req)
	if err != nil {
		return TurnRecord{}, err
	}
	if derived, ok := worker.(RepositoryDerivedWorker); ok && derived.RepositoryResultIsControllerOwned() {
		if err := c.emit(ctx, ActivityActionStarted, req, map[string]string{"action": "provider.execute"}); err != nil {
			return TurnRecord{}, err
		}
		result, workerErr := worker.Execute(ctx, c.workerRequest(req))
		ctx = context.WithoutCancel(ctx)
		if workerErr != nil {
			if err := c.emit(ctx, ActivityActionFailed, req, map[string]string{"action": "provider.execute", "error": workerErr.Error()}); err != nil {
				return TurnRecord{}, err
			}
		} else {
			if err := c.emit(ctx, ActivityActionCompleted, req, map[string]string{"action": "provider.execute"}); err != nil {
				return TurnRecord{}, err
			}
		}
		base := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, Mode: req.Mode, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, ExecutorID: result.ExecutorID, CheckpointEvidence: result.CheckpointEvidence}
		if workerErr != nil {
			base.Outcome, base.Blocker = OutcomeBlocked, workerErr.Error()
			base.EndHead = c.observedHead(ctx, repo)
			return base, workerErr
		}
		return c.deriveRepositoryOutcome(ctx, req, repo, base)
	}
	// A worker that reports its own outcome (the environment command worker)
	// is still not the authority on the repository: the same inspection
	// decides clean tree, progress, and declared validation, and its
	// reported EndHead must be what the checkout shows. A claimed
	// NO_PROGRESS is inspected too, so uncommitted work is never dropped.
	record, workerErr := c.invoke(ctx, req)
	ctx = context.WithoutCancel(ctx)
	if workerErr != nil {
		if record.EndHead == "" {
			record.EndHead = c.observedHead(ctx, repo)
		}
		return record, workerErr
	}
	if record.Outcome == OutcomeBlocked || record.Outcome == OutcomeUserDecisionRequired {
		// The worker stopped on its own account; the checkout must still not
		// be left dirty, or the consequence would be silently lost.
		if snapshot, snapErr := repo.Snapshot(ctx); snapErr == nil {
			record.EndHead = snapshot.Head
			if !snapshot.Clean {
				record.Outcome = OutcomeBlocked
				record.Blocker = "provider left repository with uncommitted changes; no checkpoint is valid"
				return record, errors.New(record.Blocker)
			}
		}
		return record, nil
	}
	claimed := record.EndHead
	record.Progress = false
	derived, err := c.deriveRepositoryOutcome(ctx, req, repo, record)
	if err != nil {
		return derived, err
	}
	if claimed != "" && derived.EndHead != claimed {
		derived.Outcome, derived.Progress = OutcomeBlocked, false
		derived.Blocker = fmt.Sprintf("worker reported end head %s but the checkout is at %s; no checkpoint is valid", claimed, derived.EndHead)
		return derived, errors.New(derived.Blocker)
	}
	return derived, nil
}

// observedHead reads the checkout HEAD after a failed worker so the BLOCKED
// record names the consequence it left; it is best effort and never fails
// the turn.
func (c Controller) observedHead(ctx context.Context, repo RepositoryAdapter) string {
	snapshot, err := repo.Snapshot(ctx)
	if err != nil {
		return ""
	}
	return snapshot.Head
}

// deriveRepositoryOutcome is the controller-owned checkpoint inspection:
// the tree must be clean, and the repository's declared validation, if any,
// must pass. Ordinarily HEAD must move from the turn's start. A bound recovery
// may instead qualify its unchanged unpublished commit span: the provider has
// reviewed the exact consequence and no corrective commit is required. A
// worker's completion proposal (commit trailer) is read here and settled
// after publication.
func (c Controller) deriveRepositoryOutcome(ctx context.Context, req TurnRequest, repo RepositoryAdapter, base TurnRecord) (TurnRecord, error) {
	if err := c.emit(ctx, ActivityValidationStarted, req, map[string]string{"scope": "repository-checkpoint"}); err != nil {
		return TurnRecord{}, err
	}
	snapshot, err := repo.Snapshot(ctx)
	if err != nil {
		base.Outcome, base.Blocker = OutcomeBlocked, fmt.Sprintf("inspect provider repository result: %v", err)
		return base, err
	}
	if err := c.emit(ctx, ActivityValidationCompleted, req, map[string]string{"scope": "repository-checkpoint", "clean": fmt.Sprint(snapshot.Clean)}); err != nil {
		return TurnRecord{}, err
	}
	base.EndHead = snapshot.Head
	if !snapshot.Clean {
		base.Outcome = OutcomeBlocked
		base.Blocker = "provider left repository with uncommitted changes; no checkpoint is valid"
		return base, errors.New(base.Blocker)
	}
	claimStart := req.StartHead
	recoveredUnchanged := snapshot.Head == req.StartHead && req.Recovery != nil && len(req.Recovery.Commits) > 0
	if snapshot.Head == req.StartHead && !recoveredUnchanged {
		base.Outcome = OutcomeNoProgress
		base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:head-unchanged")
		return base, nil
	}
	if recoveredUnchanged {
		last := req.Recovery.Commits[len(req.Recovery.Commits)-1]
		if req.Recovery.BaseHead == "" || req.Recovery.BaseHead == snapshot.Head || last != snapshot.Head {
			base.Outcome = OutcomeBlocked
			base.Blocker = "unchanged recovery does not name the exact unpublished commit span; no checkpoint is valid"
			return base, errors.New(base.Blocker)
		}
		claimStart = req.Recovery.BaseHead
		base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:validated-recovery-consequence")
	}
	if validator, ok := repo.(DeclaredValidator); ok {
		if command, declared := validator.DeclaredValidation(); declared {
			if err := c.emit(ctx, ActivityValidationStarted, req, map[string]string{"scope": "declared-validation", "command": command}); err != nil {
				return TurnRecord{}, err
			}
			output, runErr := validator.RunDeclaredValidation(ctx)
			passed := runErr == nil
			if err := c.emit(ctx, ActivityValidationCompleted, req, map[string]string{"scope": "declared-validation", "command": command, "passed": fmt.Sprint(passed), "output": truncateForActivity(output)}); err != nil {
				return TurnRecord{}, err
			}
			if !passed {
				base.Outcome = OutcomeBlocked
				base.Blocker = "declared validation failed; the local commit is retained as evidence and no checkpoint is valid: " + runErr.Error()
				return base, errors.New(base.Blocker)
			}
			base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:declared-validation-passed")
		} else {
			base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:no-declared-validation")
		}
	} else {
		base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:no-declared-validation")
	}
	// A worker never mints COMPLETE: the turn continues, and Goal completion
	// is derived by the controller from durable unit completions (#158).
	base.Outcome = OutcomeContinue
	base.Progress = true
	base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:validated-local-commit")
	if claims, ok := repo.(CompletionClaimRepository); ok {
		units, err := claims.CompletionClaims(ctx, claimStart, snapshot.Head)
		if err != nil {
			base.Outcome, base.Progress, base.Blocker = OutcomeBlocked, false, fmt.Sprintf("read unit completion claims: %v", err)
			return base, errors.New(base.Blocker)
		}
		for _, unit := range units {
			if unit != req.ChildObjective {
				base.Outcome, base.Progress = OutcomeBlocked, false
				base.Blocker = fmt.Sprintf("worker claimed completion of %s, which is not the selected unit %s; the local commit is retained as evidence and no checkpoint is valid", unit, req.ChildObjective)
				return base, errors.New(base.Blocker)
			}
			base.CompletionClaim = unit
		}
	}
	return base, nil
}

// unitCompletionPredicates are the controller-owned conditions under which a
// worker's completion proposal becomes a durable unit completion: a
// progressing turn, a published checkpoint, and the repository's declared
// validation passed on that checkpoint (or no validation is declared). A
// failed validation never reaches here: the turn is BLOCKED without a
// checkpoint.
func unitCompletionPredicates(record TurnRecord) []string {
	var missing []string
	if !record.Progress {
		missing = append(missing, "validated progress")
	}
	if !record.CheckpointPublished {
		missing = append(missing, "published checkpoint")
	}
	if !containsEvidence(record.CheckpointEvidence, "repository:declared-validation-passed") && !containsEvidence(record.CheckpointEvidence, "repository:no-declared-validation") {
		missing = append(missing, "declared validation passed")
	}
	return missing
}

func containsEvidence(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// settleCompletion turns an accepted completion proposal into durable unit
// completion evidence and, when the generation is thereby complete, into
// the Goal-level COMPLETE outcome. The worker proposes; the controller
// decides (#158).
func (c Controller) settleCompletion(ctx context.Context, req TurnRequest, repo RepositoryAdapter, record TurnRecord) (TurnRecord, error) {
	if record.CompletionClaim == "" {
		return record, nil
	}
	if err := c.emit(ctx, ActivityCompletionClaimed, req, map[string]string{"scope": "unit", "unit": record.CompletionClaim, "end_head": record.EndHead}); err != nil {
		return record, err
	}
	if missing := unitCompletionPredicates(record); len(missing) > 0 {
		return record, c.emit(ctx, ActivityValidationCompleted, req, map[string]string{"scope": "unit-completion", "unit": record.CompletionClaim, "passed": "false", "missing": strings.Join(missing, ", ")})
	}
	var requirements []string
	for _, candidate := range req.WorkCandidates {
		if candidate.ID == record.CompletionClaim {
			for _, requirement := range candidate.Requirements {
				requirements = append(requirements, requirement.ID)
			}
		}
	}
	completion := UnitCompletion{GoalID: req.GoalID, GoalVersion: req.GoalVersion, UnitID: record.CompletionClaim, InvocationID: req.InvocationID, TurnID: req.TurnID, EndHead: record.EndHead, Requirements: requirements, Evidence: append([]string{"checkpoint:" + record.EndHead}, record.CheckpointEvidence...), CompletedAt: time.Now().UTC()}
	if err := c.Ledger.RecordCompletion(ctx, completion); err != nil {
		return record, fmt.Errorf("record unit completion: %w", err)
	}
	record.UnitCompleted = true
	if err := c.emit(ctx, ActivityUnitCompleted, req, map[string]string{"unit": completion.UnitID, "end_head": completion.EndHead, "requirements": strings.Join(requirements, ",")}); err != nil {
		return record, err
	}
	return c.deriveGoalCandidate(ctx, req, repo, record)
}

// deriveGoalCandidate records the GOAL_COMPLETION_CANDIDATE and its
// deterministic evaluation when the generation's units are all complete.
// It is shared by turn-time settlement and by later materialization of a
// completion the turn earned (#164), so both derive exactly the same state.
func (c Controller) deriveGoalCandidate(ctx context.Context, req TurnRequest, repo RepositoryAdapter, record TurnRecord) (TurnRecord, error) {
	if req.GoalBaseline == nil {
		return record, nil
	}
	completions, err := c.Ledger.LoadCompletions(ctx, req.GoalID, req.GoalVersion)
	if err != nil {
		return record, err
	}
	assessment, err := AssessGoalCompletion(*req.GoalBaseline, completions)
	if err != nil {
		return record, err
	}
	if assessment.AllUnitsComplete {
		// All units complete is a GOAL_COMPLETION_CANDIDATE: the accepted
		// decomposition has been executed. It is evidence for Goal
		// completion, never proof of it. The deterministic verifier then
		// evaluates the final integrated consequence against the original
		// contract; everything it cannot verify stays UNKNOWN. Settlement is
		// a separate, authority-bearing act, so the invocation stops.
		candidate := GoalCompletionCandidate{GoalID: req.GoalID, GoalVersion: req.GoalVersion, GoalDigest: req.GoalBaseline.Digest, InvocationID: req.InvocationID, TurnID: req.TurnID, FinalHead: record.EndHead, Units: completions, Assessment: assessment, CandidateAt: time.Now().UTC()}
		if err := c.Ledger.RecordGoalCompletionCandidate(ctx, candidate); err != nil {
			return record, fmt.Errorf("record Goal completion candidate: %w", err)
		}
		if err := c.emit(ctx, ActivityCompletionClaimed, req, map[string]string{"scope": "goal-candidate", "final_head": record.EndHead, "units": strconv.Itoa(len(completions)), "uncovered_criteria": strings.Join(assessment.UncoveredCriteria, ","), "authoritative": "false"}); err != nil {
			return record, err
		}
		var verifier IntegratedValidationRepository
		if v, ok := repo.(IntegratedValidationRepository); ok {
			verifier = v
		}
		evaluation, err := EvaluateDeterministically(ctx, *req.GoalBaseline, candidate, verifier, c.Ledger.Actor)
		if err != nil {
			return record, fmt.Errorf("deterministic Goal evaluation: %w", err)
		}
		digest, err := c.Ledger.RecordGoalEvaluation(ctx, evaluation)
		if err != nil {
			return record, fmt.Errorf("record Goal evaluation: %w", err)
		}
		if err := c.emit(ctx, ActivityValidationCompleted, req, map[string]string{"scope": "goal-evaluation", "evaluator": EvaluatorDeterministic, "outcome": string(evaluation.Outcome), "evaluation_digest": digest, "unresolved": strings.Join(UnresolvedRefs(evaluation), ","), "settle_with": "praxis goals-lifecycle --operation=complete --goal-id=" + req.GoalID + " --goal-version=" + req.GoalVersion}); err != nil {
			return record, err
		}
		record.GoalCandidate = true
		record.GoalEvaluation = string(evaluation.Outcome)
		record.Outcome = OutcomeUserDecisionRequired
	}
	return record, nil
}

func truncateForActivity(text string) string {
	const limit = 2000
	if len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}
