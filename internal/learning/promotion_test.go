package learning

import "testing"

func TestPolicyViolationBlocksPromotionDespitePerformanceGain(t *testing.T) {
	policy := PromotionPolicy{MinIndependentEvidence: 2, MinSuccessRate: 0.9, MaxRegressionRate: 0.05, RequireNoPolicyViolation: true}
	metrics := PromotionMetrics{IndependentEvidence: 10, SuccessRate: 1, RegressionRate: 0, PolicyViolations: 1, LatencyImprovement: 0.9, TokenImprovement: 0.9}
	if err := policy.Evaluate(metrics); err == nil {
		t.Fatal("performance improvement must not optimize away a policy violation")
	}
}

func TestPromotionRequiresIndependentEvidence(t *testing.T) {
	policy := PromotionPolicy{MinIndependentEvidence: 3, MinSuccessRate: 0.8, MaxRegressionRate: 0.1}
	metrics := PromotionMetrics{IndependentEvidence: 2, SuccessRate: 1}
	if err := policy.Evaluate(metrics); err == nil {
		t.Fatal("insufficient independent evidence must block promotion")
	}
}
