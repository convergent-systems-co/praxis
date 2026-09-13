package kernel

import "testing"

func TestTerminalRunCannotTransition(t *testing.T) {
	if CanTransitionRun(RunSucceeded, RunRunning) {
		t.Fatal("terminal run must not transition")
	}
}

func TestCancellingCanEnterReconciliation(t *testing.T) {
	if !CanTransitionRun(RunCancelling, RunReconciling) {
		t.Fatal("cancelling run with ambiguous effects must be able to reconcile")
	}
}

func TestRunningCanSucceed(t *testing.T) {
	if err := ValidateRunTransition(RunRunning, RunSucceeded); err != nil {
		t.Fatalf("valid transition rejected: %v", err)
	}
}
