package goaldrive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type RepositorySnapshot struct {
	Clean      bool
	Relation   contracts.RepositoryRelation
	Head       string
	RemoteHead string
	BaseHead   string
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

// ConsequenceLineageRepository reports both the common recovery base and the
// authoritative upstream identity used by Fingerprint.
type ConsequenceLineageRepository interface {
	ConsequenceLineage(ctx context.Context) (baseHead, remoteHead string, err error)
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
	if len(commits) > 0 {
		if based, ok := repo.(ConsequenceLineageRepository); ok {
			if baseHead, remoteHead, baseErr := based.ConsequenceLineage(ctx); baseErr == nil {
				record.ConsequenceBaseHead, record.ConsequenceRemoteHead = baseHead, remoteHead
			}
		}
		if record.ConsequenceBaseHead == "" {
			record.ConsequenceBaseHead = record.StartHead
		}
	}
}

// RecoveryStartRepository admits a checkout that carries a bound recovery
// consequence: uncommitted changes or unpublished local commits whose
// fingerprint matches the recovered turn. Divergence additionally requires
// DivergedRecoveryRepository's ancestry proof.
type RecoveryStartRepository interface {
	RecoveryStartAllowed() bool
	VerifyRecoveryConsequence(ctx context.Context) error
}

// DivergedRecoveryRepository proves that a fetched authoritative remote
// advanced from a common base while the exact bound consequence remained on
// the local lineage. Ordinary repositories never receive this authority.
type DivergedRecoveryRepository interface {
	VerifyDivergedRecovery(ctx context.Context) (remoteHead, baseHead string, err error)
}

// CheckpointLineageVerifier is the controller-owned publication fence for a
// reconciled recovery checkpoint. The worker cannot satisfy or bypass it.
type CheckpointLineageVerifier interface {
	VerifyCheckpointLineage(ctx context.Context, remoteHead, retainedHead, checkpointHead string) (disposition string, err error)
}

type preflightFailure struct {
	stage string
	err   error
}

func (e *preflightFailure) Error() string { return e.err.Error() }
func (e *preflightFailure) Unwrap() error { return e.err }

func preflight(stage string, err error) error {
	if err == nil {
		return nil
	}
	return &preflightFailure{stage: stage, err: err}
}

// DeclaredValidator is implemented by repository adapters that can run the
// repository's own declared validation (an executable ./.praxis/validate).
// Validation is controller-owned: it runs after the provider's local commit
// and before any checkpoint is accepted.
type DeclaredValidator interface {
	DeclaredValidation() (string, bool)
	RunDeclaredValidation(context.Context) (string, error)
}

type BoundValidator interface {
	DeclaredValidator
	ValidationProfileDigest() (string, error)
}

// CheckpointValidator is the only validator a safety-bearing turn may use. Its
// runs are bound to one exact checkpoint and validation-profile digest, before
// and after execution.
type CheckpointValidator interface {
	BoundValidator
	RunBoundValidation(ctx context.Context, checkpoint, profileDigest string, args ...string) (string, error)
	// VerifyValidationBinding re-proves, without running anything, that the
	// checkout is still exactly the checkpoint and profile a result names.
	VerifyValidationBinding(ctx context.Context, checkpoint, profileDigest string) error
}

type CandidateConformanceValidator interface {
	RunDeclaredValidationWith(context.Context, ...string) (string, error)
}

const CandidateValidationAcknowledgement = "PRAXIS-VALIDATION"

// CheckpointArtifactReader returns exact bytes from the qualified checkpoint,
// never from the mutable working tree.
type CheckpointArtifactReader interface {
	ReadCheckpointArtifact(context.Context, string, string) ([]byte, error)
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
	if state == contracts.RepositoryDirty || state == contracts.RepositoryLocalAhead || state == contracts.RepositoryDiverged {
		// Uncommitted changes and unpublished local commits are admitted only
		// as the exact bound recovery consequence, or (dirty only) as a
		// persisted provider-workspace migration input.
		if recovery, ok := repo.(RecoveryStartRepository); ok && recovery.RecoveryStartAllowed() {
			if err := recovery.VerifyRecoveryConsequence(ctx); err != nil {
				return RepositorySnapshot{}, fmt.Errorf("%w: %s: %v", ErrUnsafeRepository, state, err)
			}
			if snapshot.Relation == contracts.RelationDiverged {
				diverged, ok := repo.(DivergedRecoveryRepository)
				if !ok {
					return RepositorySnapshot{}, fmt.Errorf("%w: %s: repository cannot prove divergent recovery lineage", ErrUnsafeRepository, state)
				}
				remoteHead, baseHead, err := diverged.VerifyDivergedRecovery(ctx)
				if err != nil {
					return RepositorySnapshot{}, fmt.Errorf("%w: %s: %v", ErrUnsafeRepository, state, err)
				}
				snapshot.RemoteHead, snapshot.BaseHead = remoteHead, baseHead
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
		return TurnRecord{}, preflight("repository-preflight", err)
	}
	if req.StartHead != "" && req.StartHead != snapshot.Head {
		return TurnRecord{}, preflight("repository-preflight", fmt.Errorf("requested start HEAD %q differs from synchronized HEAD %q", req.StartHead, snapshot.Head))
	}
	req.StartHead = snapshot.Head
	if req.Recovery != nil && snapshot.Relation == contracts.RelationDiverged {
		if snapshot.RemoteHead == "" || snapshot.BaseHead == "" || req.Recovery.BaseHead == "" || snapshot.BaseHead != req.Recovery.BaseHead {
			return TurnRecord{}, preflight("repository-preflight", fmt.Errorf("%w: divergent recovery base %q does not match the blocked turn base %q", ErrUnsafeRepository, snapshot.BaseHead, req.Recovery.BaseHead))
		}
		if len(req.Recovery.Commits) == 0 || req.Recovery.Commits[len(req.Recovery.Commits)-1] != snapshot.Head {
			return TurnRecord{}, preflight("repository-preflight", fmt.Errorf("%w: divergent recovery HEAD %q is not the retained consequence HEAD", ErrUnsafeRepository, snapshot.Head))
		}
		req.Recovery.RemoteHead = snapshot.RemoteHead
	}
	req.Repository = contracts.RepositorySynced
	if located, ok := repo.(LocatedRepository); ok && req.RepositoryPath == "" {
		req.RepositoryPath, req.RepositoryBranch = located.Location()
	}
	if validator, ok := repo.(DeclaredValidator); ok {
		req.DeclaredValidation, req.ValidationDeclared = validator.DeclaredValidation()
	}
	if req.GoalBaseline != nil && req.GoalBaseline.WorkPlan != nil && req.GoalBaseline.WorkPlan.Safety != nil {
		validator, ok := repo.(CheckpointValidator)
		if !ok || !req.ValidationDeclared {
			return TurnRecord{}, preflight("validation-preflight", errors.New("safety-bearing WorkPlan requires a declared, digest-bound validator"))
		}
		digest, digestErr := validator.ValidationProfileDigest()
		if digestErr != nil {
			return TurnRecord{}, preflight("validation-preflight", digestErr)
		}
		if digest != req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest {
			return TurnRecord{}, preflight("validation-preflight", fmt.Errorf("validation profile digest mismatch: got %s want %s", digest, req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest))
		}
	}
	_, req, err = c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, preflight("turn-preflight", err)
	}
	if err := c.emit(ctx, ActivityExecutionStarted, req, map[string]string{"mode": string(req.Mode)}); err != nil {
		return TurnRecord{}, preflight("dispatch-preflight", err)
	}
	if req.Recovery != nil {
		if err := c.emit(ctx, ActivityRecoveryBound, req, map[string]string{"recovered_turn": req.Recovery.RecoveredTurnID, "fingerprint": req.Recovery.Fingerprint, "files": strings.Join(req.Recovery.Files, ","), "base_head": req.Recovery.BaseHead, "remote_head": req.Recovery.RemoteHead, "commits": strings.Join(req.Recovery.Commits, ","), "provenance": req.Recovery.Provenance, "blocker": req.Recovery.Blocker}); err != nil {
			return TurnRecord{}, preflight("dispatch-preflight", err)
		}
	}
	if err := c.emit(ctx, ActivityWorkSelected, req, map[string]string{"objective": req.ChildObjective}); err != nil {
		return TurnRecord{}, preflight("dispatch-preflight", err)
	}
	if err := c.emitEnvelope(ctx, req); err != nil {
		return TurnRecord{}, preflight("dispatch-preflight", err)
	}
	record, workerErr, workerInvoked := c.invokeRepositoryTurn(ctx, req, repo)
	if workerErr != nil && !workerInvoked {
		return TurnRecord{}, preflight("dispatch-preflight", workerErr)
	}
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
	var qualification *unitQualification
	if !req.NoPush {
		if req.Lease != nil && !req.Lease.Held(ctx) {
			return record, fmt.Errorf("%w: turn %s; checkpoint %s not published", ErrLeaseLost, req.TurnID, record.EndHead)
		}
		if isSafetyBearing(req.GoalBaseline) && record.Progress && record.CompletionClaim != "" {
			// Candidate conformance is decided before publication so a
			// checkpoint that fails it is never published.
			if record.CompletionClaim != req.ChildObjective {
				// A worker proposes completion only of the unit the controller
				// selected for this turn; another unit's predicates and
				// evidence are not this turn's to qualify or publish.
				err = fmt.Errorf("completion claim %q is not the selected unit %q; no checkpoint is published", record.CompletionClaim, req.ChildObjective)
			} else if selected, found := findCandidate(req.WorkCandidates, record.CompletionClaim); !found {
				err = errors.New("completion claim does not name an accepted safety-bearing candidate")
			} else {
				var produced unitQualification
				if produced, err = c.qualifySafetyUnit(ctx, req, repo, record, selected); err == nil {
					qualification = &produced
				}
			}
		}
		if req.Recovery != nil && req.Recovery.RemoteHead != "" {
			verifier, ok := repo.(CheckpointLineageVerifier)
			if !ok {
				err = errors.New("repository cannot verify recovered checkpoint lineage")
			} else {
				retainedHead := req.StartHead
				record.RecoveryDisposition, err = verifier.VerifyCheckpointLineage(ctx, req.Recovery.RemoteHead, retainedHead, record.EndHead)
			}
			if err == nil {
				record.CheckpointEvidence = append(record.CheckpointEvidence, "repository:verified-recovery-lineage")
			}
		}
		if err == nil && record.Progress {
			// I10: publication is an outward effect. Every current predicate is
			// established before it, and the effect is bound to the exact
			// qualified checkpoint, not to whatever HEAD is by then.
			err = c.authorizeEffect(ctx, req, "checkpoint-publication", repo, record)
		}
		if err == nil {
			err = PublishCheckpoint(ctx, repo, record)
		}
		if err != nil {
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
	settled, err := c.settleCompletion(ctx, req, repo, record, qualification)
	if err != nil {
		if record.CheckpointPublished && !errors.Is(err, ErrLeaseLost) {
			// (A lost lease records nothing: reconciliation owns the
			// disposition of a turn whose authority moved on.)
			// The checkpoint is already outward. Governed completion was
			// refused, so the turn is durably recorded as published but NOT
			// governed: reality changed, and the record says so instead of
			// returning an error that leaves no trace.
			blocked := record
			// The checkpoint did validate (Progress stays true, as the ledger
			// requires of any published checkpoint); what was refused is the
			// governed completion.
			blocked.Outcome, blocked.UnitCompleted = OutcomeBlocked, false
			blocked.Blocker = "checkpoint " + record.EndHead + " was published but governed completion was refused: " + err.Error()
			if appendErr := c.recordTurn(ctx, &blocked); appendErr != nil {
				return TurnRecord{}, fmt.Errorf("record published-but-ungoverned checkpoint: %w (refusal: %v)", appendErr, err)
			}
			if emitErr := c.emitTurnOutcome(ctx, req, blocked, err); emitErr != nil {
				return TurnRecord{}, emitErr
			}
			return blocked, err
		}
		return TurnRecord{}, err
	}
	record = settled
	if err := c.recordTurn(ctx, &record); err != nil {
		return TurnRecord{}, err
	}
	if err := c.emitTurnOutcome(ctx, req, record, nil); err != nil {
		return TurnRecord{}, err
	}
	return record, nil
}

func (c Controller) invokeRepositoryTurn(ctx context.Context, req TurnRequest, repo RepositoryAdapter) (TurnRecord, error, bool) {
	worker, err := c.worker(req)
	if err != nil {
		return TurnRecord{}, err, false
	}
	if derived, ok := worker.(RepositoryDerivedWorker); ok && derived.RepositoryResultIsControllerOwned() {
		if err := c.emit(ctx, ActivityActionStarted, req, map[string]string{"action": "provider.execute"}); err != nil {
			return TurnRecord{}, err, false
		}
		result, workerErr := worker.Execute(ctx, c.workerRequest(req))
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
		base := TurnRecord{GoalID: req.GoalID, GoalVersion: req.GoalVersion, InvocationID: req.InvocationID, Mode: req.Mode, TurnID: req.TurnID, ChildObjective: req.ChildObjective, GraphID: req.GraphID, GraphVersion: req.GraphVersion, StartHead: req.StartHead, ExecutorID: result.ExecutorID, CheckpointEvidence: result.CheckpointEvidence, RetryOf: recoveredTurn(req.Recovery)}
		bindRecoveryLineage(&base, req.Recovery)
		if workerErr != nil {
			base.Outcome, base.Blocker = OutcomeBlocked, workerErr.Error()
			base.EndHead = c.observedHead(ctx, repo)
			return base, workerErr, true
		}
		derived, err := c.deriveRepositoryOutcome(ctx, req, repo, base)
		return derived, err, true
	}
	// A worker that reports its own outcome (the environment command worker)
	// is still not the authority on the repository: the same inspection
	// decides clean tree, progress, and declared validation, and its
	// reported EndHead must be what the checkout shows. A claimed
	// NO_PROGRESS is inspected too, so uncommitted work is never dropped.
	record, workerErr, workerInvoked := c.invoke(ctx, req)
	ctx = context.WithoutCancel(ctx)
	if workerErr != nil {
		if record.EndHead == "" {
			record.EndHead = c.observedHead(ctx, repo)
		}
		return record, workerErr, workerInvoked
	}
	if record.Outcome == OutcomeBlocked || record.Outcome == OutcomeUserDecisionRequired {
		// The worker stopped on its own account; the checkout must still not
		// be left dirty, or the consequence would be silently lost.
		if snapshot, snapErr := repo.Snapshot(ctx); snapErr == nil {
			record.EndHead = snapshot.Head
			if !snapshot.Clean {
				record.Outcome = OutcomeBlocked
				record.Blocker = "provider left repository with uncommitted changes; no checkpoint is valid"
				return record, errors.New(record.Blocker), workerInvoked
			}
		}
		return record, nil, workerInvoked
	}
	claimed := record.EndHead
	record.Progress = false
	derived, err := c.deriveRepositoryOutcome(ctx, req, repo, record)
	if err != nil {
		return derived, err, workerInvoked
	}
	if claimed != "" && derived.EndHead != claimed {
		derived.Outcome, derived.Progress = OutcomeBlocked, false
		derived.Blocker = fmt.Sprintf("worker reported end head %s but the checkout is at %s; no checkpoint is valid", claimed, derived.EndHead)
		return derived, errors.New(derived.Blocker), workerInvoked
	}
	return derived, nil, workerInvoked
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
	if req.Recovery != nil && req.Recovery.BaseHead != "" && len(req.Recovery.Commits) > 0 {
		claimStart = req.Recovery.BaseHead
	}
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
	if isSafetyBearing(req.GoalBaseline) {
		// Validation is mandatory and exact for a safety-bearing plan. There
		// is no "no declared validation" success: a validator that vanished
		// after admission, lost its executable bit, or changed digest blocks
		// the checkpoint, and the integrated run is bound to this exact
		// checkpoint and profile before and after it executes.
		bound, ok := repo.(CheckpointValidator)
		if !ok {
			base.Outcome, base.Blocker = OutcomeBlocked, "safety-bearing checkpoint lost its checkpoint-bound validator"
			return base, errors.New(base.Blocker)
		}
		command, declared := bound.DeclaredValidation()
		if !declared {
			base.Outcome, base.Blocker = OutcomeBlocked, "required validator is missing or not executable after worker execution; no checkpoint is valid"
			return base, errors.New(base.Blocker)
		}
		profile := req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest
		if err := c.emit(ctx, ActivityValidationStarted, req, map[string]string{"scope": "declared-validation", "command": command}); err != nil {
			return TurnRecord{}, err
		}
		output, runErr := bound.RunBoundValidation(ctx, snapshot.Head, profile)
		passed := runErr == nil
		if err := c.emit(ctx, ActivityValidationCompleted, req, map[string]string{"scope": "declared-validation", "command": command, "passed": fmt.Sprint(passed), "output": truncateForActivity(output)}); err != nil {
			return TurnRecord{}, err
		}
		if !passed {
			base.Outcome = OutcomeBlocked
			base.Blocker = "bound declared validation failed or its binding drifted; the local commit is retained as evidence and no checkpoint is valid: " + runErr.Error()
			return base, errors.New(base.Blocker)
		}
		base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:declared-validation-passed")
	} else if validator, ok := repo.(DeclaredValidator); ok {
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
		var units []string
		var err error
		if req.Recovery != nil && req.Recovery.RemoteHead != "" {
			recoveryClaims, recoveryOK := repo.(RecoveryCompletionClaimRepository)
			if !recoveryOK {
				err = errors.New("repository cannot isolate recovery completion claims")
			} else {
				units, err = recoveryClaims.RecoveryCompletionClaims(ctx, req.Recovery.RemoteHead, snapshot.Head)
			}
		} else {
			units, err = claims.CompletionClaims(ctx, claimStart, snapshot.Head)
		}
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

func bindRecoveryLineage(record *TurnRecord, recovery *WorkerRecoveryContext) {
	if record == nil || recovery == nil {
		return
	}
	record.RecoveryBaseHead = recovery.BaseHead
	record.RecoveryRemoteHead = recovery.RemoteHead
	if len(recovery.Commits) > 0 {
		record.RecoveryRetainedHead = recovery.Commits[len(recovery.Commits)-1]
	}
}

// unitCompletionPredicates are the controller-owned conditions under which a
// worker's completion proposal becomes a durable unit completion: a
// progressing turn, a published checkpoint, and the repository's declared
// validation passed on that checkpoint (or no validation is declared). A
// failed validation never reaches here: the turn is BLOCKED without a
// checkpoint.
func unitCompletionPredicates(record TurnRecord, safetyBearing bool) []string {
	var missing []string
	if !record.Progress {
		missing = append(missing, "validated progress")
	}
	if !record.CheckpointPublished {
		missing = append(missing, "published checkpoint")
	}
	passed := containsEvidence(record.CheckpointEvidence, "repository:declared-validation-passed")
	if !passed && !safetyBearing && containsEvidence(record.CheckpointEvidence, "repository:no-declared-validation") {
		passed = true
	}
	if !passed {
		missing = append(missing, "declared validation passed")
	}
	return missing
}

// authorizeEffect is the single I10 boundary. Every effect that changes the
// world or records governed consequence for a safety-bearing generation (the
// checkpoint push, a unit completion, a gate completion) establishes ALL of its
// current predicates here, before the effect:
//
//   - the kernel activation still matches the running process and package;
//   - the plan's governing authority is still effective;
//   - the turn's lease is still held;
//   - the validation results are still attributable to the exact checkpoint and
//     profile (content identity, not working-tree cleanliness).
//
// Publication pushes the exact qualified commit, so a change after this check
// cannot substitute different content; a revocation that lands in the remaining
// interval is detected by the settlement re-check and recorded truthfully as
// published-but-ungoverned (see ExecuteTurnWithRepository).
func (c Controller) authorizeEffect(ctx context.Context, req TurnRequest, effect string, repo RepositoryAdapter, record TurnRecord) error {
	if !isSafetyBearing(req.GoalBaseline) {
		return nil
	}
	if c.SafetyActivation == nil {
		return fmt.Errorf("%w: verifier is not configured (%s)", ErrSafetyActivation, effect)
	}
	if err := c.SafetyActivation.Verify(ctx, *req.GoalBaseline.WorkPlan.Safety); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSafetyActivation, effect, err)
	}
	if err := c.verifyGoverningAuthority(ctx, req.GoalBaseline); err != nil {
		return fmt.Errorf("%s: %w", effect, err)
	}
	if req.Lease != nil && !req.Lease.Held(ctx) {
		return fmt.Errorf("%w: turn %s; %s refused", ErrLeaseLost, req.TurnID, effect)
	}
	if repo != nil && record.EndHead != "" {
		validator, ok := repo.(CheckpointValidator)
		if !ok {
			return fmt.Errorf("%s requires a checkpoint-bound validator", effect)
		}
		if err := validator.VerifyValidationBinding(ctx, record.EndHead, req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest); err != nil {
			return fmt.Errorf("validation binding drifted before %s: %w", effect, err)
		}
	}
	return nil
}

func isSafetyBearing(baseline *goals.GoalBaseline) bool {
	return baseline != nil && baseline.WorkPlan != nil && baseline.WorkPlan.Safety != nil
}

func containsEvidence(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// unitQualification is the evidence one safety-bearing unit earns from bound
// candidate-conformance runs and exact-checkpoint governed-output capture.
type unitQualification struct {
	evidence  []string
	artifacts []contracts.GovernedArtifactEvidence
}

// qualifySafetyUnit runs every accepted candidate-conformance predicate and
// captures every governed output, all against the one exact checkpoint and
// validation profile of the turn. It performs no durable write, so it can run
// before publication: a checkpoint that fails candidate conformance is never
// published.
func (c Controller) qualifySafetyUnit(ctx context.Context, req TurnRequest, repo RepositoryAdapter, record TurnRecord, selected contracts.WorkCandidate) (unitQualification, error) {
	var out unitQualification
	validator, ok := repo.(CheckpointValidator)
	if !ok {
		return out, errors.New("safety-bearing completion requires a checkpoint-bound validator")
	}
	if !containsEvidence(record.CheckpointEvidence, "repository:declared-validation-passed") {
		return out, errors.New("safety-bearing completion requires integrated-pass evidence bound to the checkpoint")
	}
	profile := req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest
	for _, predicate := range selected.QualificationPredicates {
		output, err := validator.RunBoundValidation(ctx, record.EndHead, profile, predicate)
		if err != nil {
			return out, fmt.Errorf("candidate conformance %s: %w", predicate, err)
		}
		ack := CandidateValidationAcknowledgement + " " + predicate
		acknowledged := false
		for _, line := range strings.Split(output, "\n") {
			if strings.TrimSpace(line) == ack {
				acknowledged = true
				break
			}
		}
		if !acknowledged {
			return out, fmt.Errorf("candidate conformance %s returned malformed or unhandled evidence", predicate)
		}
		sum := sha256.Sum256([]byte(output))
		out.evidence = append(out.evidence, "conformance:"+predicate+":sha256:"+hex.EncodeToString(sum[:]))
	}
	var outputs []contracts.GovernedOutputContract
	if len(selected.Specification) > 0 {
		var parseErr error
		outputs, parseErr = contracts.ParseGovernedOutputContracts(selected.Specification)
		if parseErr != nil {
			return out, fmt.Errorf("parse governed outputs: %w", parseErr)
		}
	}
	if len(outputs) > 0 {
		reader, ok := repo.(CheckpointArtifactReader)
		if !ok {
			return out, errors.New("governed outputs require exact checkpoint artifact resolution")
		}
		for _, output := range outputs {
			body, readErr := reader.ReadCheckpointArtifact(ctx, record.EndHead, output.SourceRef)
			if readErr != nil {
				return out, fmt.Errorf("resolve governed output %s: %w", output.Role, readErr)
			}
			if len(body) == 0 || len(body) > contracts.MaxGovernedArtifactBytes {
				return out, fmt.Errorf("governed output %s is missing or exceeds the byte bound", output.Role)
			}
			sum := sha256.Sum256(body)
			out.artifacts = append(out.artifacts, contracts.GovernedArtifactEvidence{
				ProducerCandidateID: selected.ID, ProducerSpecificationHash: selected.SourceDigest,
				ValidationProfileDigest: profile, ConformanceQualified: true,
				Checkpoint: record.EndHead, Role: output.Role, EvidenceClass: output.EvidenceClass,
				SourceRef: output.SourceRef, SchemaID: output.SchemaID,
				Digest: "sha256:" + hex.EncodeToString(sum[:]), Bytes: body,
			})
		}
	}
	return out, nil
}

// settleCompletion turns an accepted completion proposal into durable unit
// completion evidence and, when the generation is thereby complete, into
// the Goal-level COMPLETE outcome. The worker proposes; the controller
// decides (#158).
func (c Controller) settleCompletion(ctx context.Context, req TurnRequest, repo RepositoryAdapter, record TurnRecord, qualification *unitQualification) (TurnRecord, error) {
	if record.CompletionClaim == "" {
		return record, nil
	}
	if isSafetyBearing(req.GoalBaseline) && record.CompletionClaim != req.ChildObjective {
		return record, fmt.Errorf("completion claim %q is not the selected unit %q", record.CompletionClaim, req.ChildObjective)
	}
	if err := c.emit(ctx, ActivityCompletionClaimed, req, map[string]string{"scope": "unit", "unit": record.CompletionClaim, "end_head": record.EndHead}); err != nil {
		return record, err
	}
	safety := isSafetyBearing(req.GoalBaseline)
	if missing := unitCompletionPredicates(record, safety); len(missing) > 0 {
		return record, c.emit(ctx, ActivityValidationCompleted, req, map[string]string{"scope": "unit-completion", "unit": record.CompletionClaim, "passed": "false", "missing": strings.Join(missing, ", ")})
	}
	var requirements []string
	var selected *contracts.WorkCandidate
	for _, candidate := range req.WorkCandidates {
		if candidate.ID == record.CompletionClaim {
			candidateCopy := candidate
			selected = &candidateCopy
			for _, requirement := range candidate.Requirements {
				requirements = append(requirements, requirement.ID)
			}
		}
	}
	completion := UnitCompletion{GoalID: req.GoalID, GoalVersion: req.GoalVersion, UnitID: record.CompletionClaim, InvocationID: req.InvocationID, TurnID: req.TurnID, EndHead: record.EndHead, Requirements: requirements, Evidence: append([]string{"checkpoint:" + record.EndHead}, record.CheckpointEvidence...), CompletedAt: time.Now().UTC()}
	if safety {
		if selected == nil {
			return record, errors.New("completion claim does not name an accepted safety-bearing candidate")
		}
		// Qualification normally ran before publication; it is repeated here
		// only when it did not (never trusted from an earlier, unbound run).
		if qualification == nil {
			produced, err := c.qualifySafetyUnit(ctx, req, repo, record, *selected)
			if err != nil {
				return record, err
			}
			qualification = &produced
		}
		// A completion is a new safety-bearing consequence. Activation, the
		// governing authority, the lease and the validation binding to the exact
		// checkpoint must all hold at the instant it is recorded.
		if err := c.authorizeEffect(ctx, req, "completion-record", repo, record); err != nil {
			return record, err
		}
		completion.Evidence = append(completion.Evidence, qualification.evidence...)
		completion.MechanismTestsPassed = true
		completion.ConformanceQualified = true
		completion.SpecificationDigest = selected.SourceDigest
		completion.ValidationProfileDigest = req.GoalBaseline.WorkPlan.Safety.ValidationProfileDigest
		completion.GovernedArtifacts = qualification.artifacts
	}
	if err := c.recordCompletion(ctx, req.GoalBaseline, completion); err != nil {
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
	effective, err := c.effectiveCompletions(ctx, req.GoalBaseline, req.GoalID, req.GoalVersion)
	if err != nil {
		return record, err
	}
	completions := effective.Effective
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
