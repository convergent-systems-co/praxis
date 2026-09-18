package develop

import (
	"testing"
	"time"
)

func TestPlanningMetricsAmortizeProjectCostAcrossSlices(t *testing.T) {
	tel:=PlanningTelemetry{BaselineBuilds:1,BaselineReuses:3,DeltaPlans:1,ProjectTokens:1000,SlicePlanningTokens:500,ProjectPlanningTime:10*time.Minute,SlicePlanningTime:5*time.Minute,SlicesCompleted:5,FirstUsefulActionSum:5*time.Minute}
	m,err:=tel.Metrics(); if err!=nil { t.Fatal(err) }
	if m.PlanningTokensPerCompletedSlice!=300 { t.Fatalf("expected 300 tokens/slice, got %f",m.PlanningTokensPerCompletedSlice) }
	if m.BaselineReuseRate!=0.75 { t.Fatalf("expected reuse rate .75, got %f",m.BaselineReuseRate) }
	if m.MeanTimeToFirstUsefulAction!=time.Minute { t.Fatalf("unexpected mean first action %s",m.MeanTimeToFirstUsefulAction) }
}

func TestEstimatedAvoidedPlanningRequiresMeasuredComparator(t *testing.T) {
	tel:=PlanningTelemetry{ProjectTokens:1000,SlicePlanningTokens:500,SlicesCompleted:5}
	if got:=tel.EstimatedRepeatedPlanningAvoided(0); got!=0 { t.Fatalf("must not invent counterfactual savings, got %d",got) }
	if got:=tel.EstimatedRepeatedPlanningAvoided(1000); got!=3500 { t.Fatalf("expected measured avoided planning 3500, got %d",got) }
}

func TestPlanningTelemetryRejectsReuseWithoutBaseline(t *testing.T) {
	_,err:= (PlanningTelemetry{BaselineReuses:1}).Metrics(); if err==nil { t.Fatal("reuse without a built baseline must fail") }
}

func TestNoSavingsReportedWhenActualCostIsHigher(t *testing.T) {
	tel:=PlanningTelemetry{ProjectTokens:4000,SlicePlanningTokens:2000,SlicesCompleted:3}
	if got:=tel.EstimatedRepeatedPlanningAvoided(1000); got!=0 { t.Fatalf("negative savings must clamp to zero, got %d",got) }
}
