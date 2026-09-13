package kernel

import (
	"context"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type RunControlOperation string

const (
	RunControlCancel RunControlOperation = "cancel"
	RunControlResume RunControlOperation = "resume"
)

// RunControlAuthorizer is the deterministic decision boundary for mutation
// stores that do not support an atomic authority+event commit. Durable
// production stores SHOULD use RunControlCommitter instead.
type RunControlAuthorizer interface {
	AuthorizeRunControl(ctx context.Context, actor contracts.PrincipalRef, run RunExecution, operation RunControlOperation) error
}

// RunControlCommitter atomically validates/consumes mutation authority and
// commits the supplied run observation at expectedVersion+1. It is the
// preferred durable mutation boundary because authority remains valid through
// the final event commit.
type RunControlCommitter interface {
	CommitRunControl(ctx context.Context, actor contracts.PrincipalRef, operation RunControlOperation, expectedVersion int64, observation RunObservation, commandID, correlationID string) (int64, error)
}

// RunControl reconstructs authoritative run state from the append-only event
// stream and performs control operations using optimistic aggregate versions.
// It intentionally maintains no mutable status table of its own.
type RunControl struct {
	Store      eventstore.Store
	Actor      contracts.PrincipalRef
	Authorizer RunControlAuthorizer
	Committer  RunControlCommitter
}

type RunStatus struct {
	Run              *RunExecution
	AggregateVersion int64
}

func (c RunControl) Status(ctx context.Context, runID string) (RunStatus, error) {
	if c.Store == nil || runID == "" {
		return RunStatus{}, errors.New("run control store and run id are required")
	}
	events, err := c.Store.LoadAggregate(ctx, runID, 0)
	if err != nil {
		return RunStatus{}, fmt.Errorf("load run %q: %w", runID, err)
	}
	if len(events) == 0 {
		return RunStatus{}, fmt.Errorf("run %q not found", runID)
	}
	run, version, err := ReplayRun(events)
	if err != nil {
		return RunStatus{}, fmt.Errorf("replay run %q: %w", runID, err)
	}
	return RunStatus{Run: run, AggregateVersion: version}, nil
}

// Cancel appends the terminal cancellation fact to the authoritative event
// stream. A terminal run cannot be cancelled again.
func (c RunControl) Cancel(ctx context.Context, runID, commandID, correlationID string) (RunStatus, error) {
	status, err := c.Status(ctx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	if status.Run.State.Terminal() {
		return RunStatus{}, fmt.Errorf("run %q is already terminal in state %q", runID, status.Run.State)
	}
	observation := baseObservation(status.Run, ObservationRunTerminal, status.Run.CurrentNode)
	observation.State = RunCancelled
	observation.FailureClass = FailureCancellation

	if c.Committer != nil {
		if err := c.Actor.Validate(); err != nil {
			return RunStatus{}, fmt.Errorf("run control actor: %w", err)
		}
		version, err := c.Committer.CommitRunControl(ctx, c.Actor, RunControlCancel, status.AggregateVersion, observation, commandID, correlationID)
		if err != nil {
			return RunStatus{}, fmt.Errorf("cancel run %q: %w", runID, err)
		}
		status.AggregateVersion = version
	} else {
		if err := c.authorize(ctx, *status.Run, RunControlCancel); err != nil {
			return RunStatus{}, err
		}
		journal, err := c.journal(commandID, correlationID, status.AggregateVersion)
		if err != nil {
			return RunStatus{}, err
		}
		if err := journal.ObserveRun(ctx, observation); err != nil {
			return RunStatus{}, fmt.Errorf("cancel run %q: %w", runID, err)
		}
		status.AggregateVersion = journal.ExpectedVersion
	}
	status.Run.State = RunCancelled
	status.Run.PendingWait = nil
	return status, nil
}

// Resume satisfies the exact persisted wait reference and continues execution
// through the normal kernel. With an atomic committer, the resume observation
// and capability consumption are committed together before execution resumes,
// so a crash cannot resurrect the old wait or reuse one-shot authority.
func (c RunControl) Resume(ctx context.Context, graph GraphDef, runID string, kind WaitKind, ref, commandID, correlationID string, executor NodeExecutor) (RunStatus, error) {
	if executor == nil {
		return RunStatus{}, errors.New("resume node executor is required")
	}
	status, err := c.Status(ctx, runID)
	if err != nil {
		return RunStatus{}, err
	}
	if status.Run.State.Terminal() {
		return RunStatus{}, fmt.Errorf("terminal run %q cannot be resumed", runID)
	}
	if err := SatisfyWait(status.Run, kind, ref); err != nil {
		return RunStatus{}, err
	}

	if c.Committer != nil {
		if err := c.Actor.Validate(); err != nil {
			return RunStatus{}, fmt.Errorf("run control actor: %w", err)
		}
		observation := baseObservation(status.Run, ObservationRunResumed, status.Run.CurrentNode)
		observation.State = RunRunning
		version, err := c.Committer.CommitRunControl(ctx, c.Actor, RunControlResume, status.AggregateVersion, observation, commandID, correlationID)
		if err != nil {
			return RunStatus{}, fmt.Errorf("resume run %q: %w", runID, err)
		}
		status.AggregateVersion = version
		status.Run.State = RunRunning
	} else {
		if err := c.authorize(ctx, *status.Run, RunControlResume); err != nil {
			return RunStatus{}, err
		}
	}

	journal, err := c.journal(commandID, correlationID, status.AggregateVersion)
	if err != nil {
		return RunStatus{}, err
	}
	if err := RunObserved(ctx, graph, status.Run, executor, journal); err != nil && !errors.Is(err, ErrRunSuspended) {
		return RunStatus{}, err
	}
	status.AggregateVersion = journal.ExpectedVersion
	return status, nil
}

func (c RunControl) authorize(ctx context.Context, run RunExecution, operation RunControlOperation) error {
	if c.Authorizer == nil {
		return errors.New("run control mutation requires an explicit authorizer or atomic committer")
	}
	if err := c.Actor.Validate(); err != nil {
		return fmt.Errorf("run control actor: %w", err)
	}
	if err := c.Authorizer.AuthorizeRunControl(ctx, c.Actor, run, operation); err != nil {
		return fmt.Errorf("run control %s denied: %w", operation, err)
	}
	return nil
}

func (c RunControl) journal(commandID, correlationID string, expectedVersion int64) (*EventJournal, error) {
	if c.Store == nil {
		return nil, errors.New("run control store is required")
	}
	if err := c.Actor.Validate(); err != nil {
		return nil, fmt.Errorf("run control actor: %w", err)
	}
	if commandID == "" || correlationID == "" {
		return nil, errors.New("run control command id and correlation id are required")
	}
	return &EventJournal{
		Store:           c.Store,
		Actor:           c.Actor,
		CommandID:       commandID,
		CorrelationID:   correlationID,
		Trust:           contracts.TrustObserved,
		ExpectedVersion: expectedVersion,
	}, nil
}
