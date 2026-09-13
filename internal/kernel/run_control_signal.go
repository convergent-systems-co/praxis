package kernel

import (
	"context"
	"errors"
	"fmt"
)

// ResumeSignal satisfies the exact persisted wait and commits a resumed/runnable
// fact without executing any node in the calling client. A scheduler/runtime
// worker may subsequently continue the run through the normal graph executor.
// This keeps the CLI from becoming an execution-authority boundary.
func (c RunControl) ResumeSignal(ctx context.Context, runID string, kind WaitKind, ref, commandID, correlationID string) (RunStatus, error) {
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
	observation := baseObservation(status.Run, ObservationRunResumed, status.Run.CurrentNode)
	observation.State = RunRunnable

	if c.Committer != nil {
		if err := c.Actor.Validate(); err != nil {
			return RunStatus{}, fmt.Errorf("run control actor: %w", err)
		}
		version, err := c.Committer.CommitRunControl(ctx, c.Actor, RunControlResume, status.AggregateVersion, observation, commandID, correlationID)
		if err != nil {
			return RunStatus{}, fmt.Errorf("resume run %q: %w", runID, err)
		}
		status.AggregateVersion = version
	} else {
		if err := c.authorize(ctx, *status.Run, RunControlResume); err != nil {
			return RunStatus{}, err
		}
		journal, err := c.journal(commandID, correlationID, status.AggregateVersion)
		if err != nil {
			return RunStatus{}, err
		}
		if err := journal.ObserveRun(ctx, observation); err != nil {
			return RunStatus{}, fmt.Errorf("resume run %q: %w", runID, err)
		}
		status.AggregateVersion = journal.ExpectedVersion
	}
	if status.AggregateVersion <= 0 {
		return RunStatus{}, errors.New("resume did not advance authoritative aggregate version")
	}
	status.Run.State = RunRunnable
	status.Run.PendingWait = nil
	return status, nil
}
