package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	if state == contracts.RepositoryDirty {
		if allowed, ok := repo.(DirtyStartRepository); !ok || !allowed.DirtyStartAllowed() {
			return RepositorySnapshot{}, fmt.Errorf("%w: %s", ErrUnsafeRepository, state)
		}
		return snapshot, nil
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
	turns, req, err := c.prepare(ctx, req)
	if err != nil {
		return TurnRecord{}, err
	}
	if err := c.emit(ctx, ActivityExecutionStarted, req, map[string]string{"mode": string(req.Mode)}); err != nil {
		return TurnRecord{}, err
	}
	if req.Recovery != nil {
		if err := c.emit(ctx, ActivityRecoveryBound, req, map[string]string{"recovered_turn": req.Recovery.RecoveredTurnID, "fingerprint": req.Recovery.Fingerprint, "files": strings.Join(req.Recovery.Files, ","), "blocker": req.Recovery.Blocker}); err != nil {
			return TurnRecord{}, err
		}
	}
	if err := c.emit(ctx, ActivityWorkSelected, req, map[string]string{"objective": req.ChildObjective}); err != nil {
		return TurnRecord{}, err
	}
	record, workerErr := c.invokeRepositoryTurn(ctx, req, repo)
	if workerErr != nil {
		if _, appendErr := c.Ledger.Append(ctx, int64(len(turns)), record); appendErr != nil {
			return TurnRecord{}, fmt.Errorf("record worker interruption: %w (worker: %v)", appendErr, workerErr)
		}
		if emitErr := c.emitTurnOutcome(ctx, req, record, workerErr); emitErr != nil {
			return TurnRecord{}, emitErr
		}
		return record, workerErr
	}
	if !req.NoPush {
		if err := PublishCheckpoint(ctx, repo, record); err != nil {
			blocked := record
			blocked.Outcome = OutcomeBlocked
			blocked.Progress = false
			blocked.Blocker = err.Error()
			if _, appendErr := c.Ledger.Append(ctx, int64(len(turns)), blocked); appendErr != nil {
				return TurnRecord{}, fmt.Errorf("record checkpoint publication failure: %w", appendErr)
			}
			if emitErr := c.emitTurnOutcome(ctx, req, blocked, err); emitErr != nil {
				return TurnRecord{}, emitErr
			}
			return blocked, err
		}
	}
	record.CheckpointPublished = record.Progress && !req.NoPush
	if _, err := c.Ledger.Append(ctx, int64(len(turns)), record); err != nil {
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
	derived, ok := worker.(RepositoryDerivedWorker)
	if !ok || !derived.RepositoryResultIsControllerOwned() {
		return c.invoke(ctx, req)
	}
	if err := c.emit(ctx, ActivityActionStarted, req, map[string]string{"action": "provider.execute"}); err != nil {
		return TurnRecord{}, err
	}
	result, workerErr := worker.Execute(ctx, c.workerRequest(req))
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
		return base, workerErr
	}
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
	if snapshot.Head == req.StartHead {
		base.Outcome = OutcomeNoProgress
		base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:head-unchanged")
		return base, nil
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
	}
	base.Outcome = OutcomeContinue
	base.Progress = true
	base.CheckpointEvidence = append(base.CheckpointEvidence, "repository:validated-local-commit")
	return base, nil
}

func truncateForActivity(text string) string {
	const limit = 2000
	if len(text) <= limit {
		return text
	}
	return text[len(text)-limit:]
}
