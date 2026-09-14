package main

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
)

func TestDispatchGoalDriveRequiresExactDurableGeneration(t *testing.T) {
	out := normalizedOutput{EntryPointID: "goal-drive", Options: map[string]string{"goal-id": "goal-1", "provider": "local", "invocation-id": "inv-1"}}
	if !errors.Is(dispatchGoalDrive(context.Background(), out, nil), goaldrive.ErrExactGoalVersionRequired) {
		t.Fatal("goal-drive dispatch must require an exact Goal Baseline version")
	}
}

func TestDispatchGoalDriveFailsClosedBeforeExecutionDependencies(t *testing.T) {
	out := normalizedOutput{EntryPointID: "goal-drive", Options: map[string]string{"goal-id": "goal-1", "goal-version": "7", "provider": "local", "invocation-id": "inv-1"}}
	if !errors.Is(dispatchGoalDrive(context.Background(), out, nil), errGoalDriveDispatchDependencies) {
		t.Fatal("CLI must fail closed when runtime provider/key dependencies are unavailable")
	}
}
