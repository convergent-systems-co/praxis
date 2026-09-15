package goaldrive

import (
	"errors"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var ErrNoAcceptedGoalWorkPlan = errors.New("Goal has no accepted executable work plan")

// MaterializeGoalWork is the control-plane bridge from a verified Goal
// baseline to selector input. It intentionally refuses to infer work from
// OriginalIntent, SuccessCriteria, PlanRef, or model output.
func MaterializeGoalWork(baseline goals.GoalBaseline) ([]contracts.WorkCandidate, []contracts.WorkRelationship, error) {
	if baseline.WorkPlan == nil {
		return nil, nil, ErrNoAcceptedGoalWorkPlan
	}
	return contracts.MaterializeWorkPlan(*baseline.WorkPlan)
}
