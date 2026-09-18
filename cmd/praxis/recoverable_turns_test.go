package main

import (
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// TestRecoverableTurnsNameExactBlockedTurns proves inspect emits a recovery
// template only for BLOCKED turns without checkpoint whose objective has not
// progressed since, and that the template binds the exact turn identity.
func TestRecoverableTurnsNameExactBlockedTurns(t *testing.T) {
	baseline := goals.GoalBaseline{ID: "goal:weather-app", Version: "2"}
	turns := []goaldrive.TurnRecord{
		{TurnID: "live-001:turn:1", InvocationID: "live-001", ChildObjective: "unit:a", Outcome: goaldrive.OutcomeBlocked, EndHead: "0ac6bb8", Blocker: "provider left repository with uncommitted changes"},
		{TurnID: "live-002:turn:2", InvocationID: "live-002", ChildObjective: "unit:b", Outcome: goaldrive.OutcomeBlocked},
		{TurnID: "live-003:turn:3", InvocationID: "live-003", ChildObjective: "unit:b", Outcome: goaldrive.OutcomeContinue, Progress: true},
		{TurnID: "live-004:turn:4", InvocationID: "live-004", ChildObjective: "unit:c", Outcome: goaldrive.OutcomeContinue, Progress: true},
	}
	out := recoverableTurns(baseline, turns)
	if len(out) != 1 || out[0]["turn_id"] != "live-001:turn:1" {
		t.Fatalf("only the unprogressed blocked turn is recoverable: %v", out)
	}
	template := out[0]["recover_template"].(map[string]any)
	command := template["command"].(string)
	for _, want := range []string{"praxis goal-drive", "--goal-id=goal:weather-app", "--goal-version=2", "--mode=supervised", "--recover-turn=live-001:turn:1"} {
		if !strings.Contains(command, want) {
			t.Fatalf("recover template lacks %q: %s", want, command)
		}
	}
	if placeholderPattern.MatchString(command) {
		t.Fatalf("the durable part of the template carries no placeholder: %s", command)
	}
	if supplies := template["operator_supplies"].([]string); len(supplies) != 4 || !strings.HasPrefix(supplies[1], "--invocation-id=") {
		t.Fatalf("operator intent is named, not invented: %v", supplies)
	}
}
