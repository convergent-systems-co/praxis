package plugin

import "testing"

func TestQuarantinedPluginCannotRestart(t *testing.T) {
	if CanTransition(StateQuarantined, StateStarting) {
		t.Fatal("quarantined plugin must require explicit administrative recovery")
	}
}

func TestRepeatedFailuresTriggerQuarantine(t *testing.T) {
	window := FailureWindow{Failures: 3, MaxFailures: 3}
	if !window.ShouldQuarantine() {
		t.Fatal("failure threshold should trigger quarantine")
	}
}

func TestReadyPluginCanDrain(t *testing.T) {
	if err := ValidateTransition(StateReady, StateDraining); err != nil {
		t.Fatalf("ready plugin should be able to drain: %v", err)
	}
}
