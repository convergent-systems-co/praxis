package develop

import (
	"errors"
	"time"
)

type PlanningTelemetry struct {
	BaselineBuilds       uint64
	BaselineReuses       uint64
	DeltaPlans           uint64
	FullReplans          uint64
	ProjectTokens        uint64
	SlicePlanningTokens  uint64
	ProjectPlanningTime  time.Duration
	SlicePlanningTime    time.Duration
	SlicesCompleted      uint64
	ConformanceFailures  uint64
	FirstUsefulActionSum time.Duration
}

type PlanningMetrics struct {
	PlanningTokensPerCompletedSlice float64
	PlanningTimePerCompletedSlice   time.Duration
	BaselineReuseRate               float64
	ConformanceFailureRate          float64
	MeanTimeToFirstUsefulAction     time.Duration
}

func (t PlanningTelemetry) Metrics() (PlanningMetrics, error) {
	if t.BaselineReuses+t.DeltaPlans+t.FullReplans > 0 && t.BaselineBuilds == 0 {
		return PlanningMetrics{}, errors.New("planning activity cannot reference a baseline that was never built")
	}
	var m PlanningMetrics
	if t.SlicesCompleted > 0 {
		m.PlanningTokensPerCompletedSlice = float64(t.ProjectTokens+t.SlicePlanningTokens) / float64(t.SlicesCompleted)
		m.PlanningTimePerCompletedSlice = (t.ProjectPlanningTime+t.SlicePlanningTime) / time.Duration(t.SlicesCompleted)
		m.ConformanceFailureRate = float64(t.ConformanceFailures) / float64(t.SlicesCompleted)
		m.MeanTimeToFirstUsefulAction = t.FirstUsefulActionSum / time.Duration(t.SlicesCompleted)
	}
	attempts := t.BaselineReuses + t.DeltaPlans + t.FullReplans
	if attempts > 0 {
		m.BaselineReuseRate = float64(t.BaselineReuses) / float64(attempts)
	}
	return m, nil
}

// EstimatedRepeatedPlanningAvoided is intentionally evidence-based: the caller
// provides a measured/controlled per-slice project-planning baseline rather than
// Praxis inventing a hypothetical savings number.
func (t PlanningTelemetry) EstimatedRepeatedPlanningAvoided(measuredProjectTokensPerSlice uint64) uint64 {
	if measuredProjectTokensPerSlice == 0 || t.SlicesCompleted <= 1 {
		return 0
	}
	withoutReuse := measuredProjectTokensPerSlice * t.SlicesCompleted
	actual := t.ProjectTokens + t.SlicePlanningTokens
	if actual >= withoutReuse {
		return 0
	}
	return withoutReuse - actual
}
