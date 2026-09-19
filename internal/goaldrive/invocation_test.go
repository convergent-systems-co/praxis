package goaldrive

import (
	"testing"
)

func TestParseInvocationNormalizesGoalInputAndControlOptions(t *testing.T) {
	request, err := ParseInvocation(map[string]string{"goal-id": "goal:inventory", "provider": "codex", "invocation-id": "inv-1", "goal-version": "7", "max-turns": "3", "turn-timeout": "2m", "no-progress-limit": "2", "no-push": "true"})
	if err != nil {
		t.Fatal(err)
	}
	if request.Input.Kind != "goal_id" || request.Input.GoalID != "goal:inventory" || request.ProviderID != "codex" || request.InvocationID != "inv-1" || request.GoalVersion != "7" || request.Mode != ModeSupervised || request.MaxTurns != 3 || request.TurnTimeout.String() != "2m0s" || request.NoProgressLimit != 2 || !request.NoPush || !request.RequireClean {
		t.Fatalf("unexpected normalized invocation: %+v", request)
	}
}

func TestParseInvocationFailsClosedForAmbiguousOrInvalidOptions(t *testing.T) {
	cases := []map[string]string{
		{"goal": "literal", "goal-id": "goal-1", "provider": "codex"},
		{"goal": "literal"},
		{"goal": "literal", "provider": "codex", "invocation-id": "inv-1", "mode": "invalid"},
		{"goal": "literal", "provider": "codex", "max-turns": "0"},
		{"goal": "literal", "provider": "codex", "turn-timeout": "0s"},
		{"goal": "literal", "provider": "codex", "no-push": "maybe"},
	}
	for _, options := range cases {
		if _, err := ParseInvocation(options); err == nil {
			t.Fatalf("expected invalid invocation to fail: %#v", options)
		}
	}
}
