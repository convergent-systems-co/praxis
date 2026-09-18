package kernel

import "fmt"

type RunState string

const (
	RunQueued       RunState = "queued"
	RunRunnable     RunState = "runnable"
	RunWaiting      RunState = "waiting"
	RunRunning      RunState = "running"
	RunSuspended    RunState = "suspended"
	RunCancelling   RunState = "cancelling"
	RunCancelled    RunState = "cancelled"
	RunSucceeded    RunState = "succeeded"
	RunFailed       RunState = "failed"
	RunReconciling  RunState = "reconciling"
)

func (s RunState) Terminal() bool {
	return s == RunCancelled || s == RunSucceeded || s == RunFailed
}

func CanTransitionRun(from, to RunState) bool {
	if from.Terminal() {
		return false
	}
	allowed := map[RunState]map[RunState]bool{
		RunQueued:      {RunRunnable: true, RunCancelled: true, RunFailed: true},
		RunRunnable:    {RunRunning: true, RunWaiting: true, RunCancelling: true, RunFailed: true},
		RunWaiting:     {RunRunnable: true, RunSuspended: true, RunCancelling: true, RunFailed: true, RunReconciling: true},
		RunRunning:     {RunWaiting: true, RunSuspended: true, RunCancelling: true, RunSucceeded: true, RunFailed: true, RunReconciling: true},
		RunSuspended:   {RunRunnable: true, RunCancelling: true, RunFailed: true, RunReconciling: true},
		RunCancelling:  {RunCancelled: true, RunReconciling: true, RunFailed: true},
		RunReconciling: {RunRunnable: true, RunWaiting: true, RunCancelled: true, RunSucceeded: true, RunFailed: true},
	}
	return allowed[from][to]
}

func ValidateRunTransition(from, to RunState) error {
	if !CanTransitionRun(from, to) {
		return fmt.Errorf("invalid run transition %q -> %q", from, to)
	}
	return nil
}
