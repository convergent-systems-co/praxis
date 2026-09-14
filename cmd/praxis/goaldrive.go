package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
)

var errGoalDriveDispatchDependencies = errors.New("native goal-drive dispatch requires a registered worker provider and an authoritative GoalStore key provider")

// dispatchGoalDrive is the only CLI entry into Goal-drive. It parses the
// shared invocation contract and fails closed until setup-time providers can
// construct the proven goaldrive.Runtime; contract resolution is not execution.
func dispatchGoalDrive(_ context.Context, out normalizedOutput, _ func(string) string) error {
	invocation, err := goaldrive.ParseInvocation(out.Options)
	if err != nil {
		return fmt.Errorf("parse goal-drive invocation: %w", err)
	}
	if invocation.Input.Kind != "goal_id" {
		return goaldrive.ErrGoalExecutionInput
	}
	if invocation.GoalVersion == "" {
		return goaldrive.ErrExactGoalVersionRequired
	}
	return errGoalDriveDispatchDependencies
}
