package learning

import "errors"

type PromotionMetrics struct {
	IndependentEvidence int
	SuccessRate         float64
	RegressionRate      float64
	PolicyViolations    int
	LatencyImprovement  float64
	TokenImprovement    float64
}

type PromotionPolicy struct {
	MinIndependentEvidence int
	MinSuccessRate          float64
	MaxRegressionRate       float64
	RequireNoPolicyViolation bool
}

func (p PromotionPolicy) Evaluate(m PromotionMetrics) error {
	if p.MinIndependentEvidence <= 0 {
		return errors.New("promotion policy requires positive independent evidence threshold")
	}
	if m.IndependentEvidence < p.MinIndependentEvidence {
		return errors.New("insufficient independent evidence")
	}
	if m.SuccessRate < p.MinSuccessRate {
		return errors.New("success rate below promotion threshold")
	}
	if m.RegressionRate > p.MaxRegressionRate {
		return errors.New("regression rate exceeds promotion threshold")
	}
	if p.RequireNoPolicyViolation && m.PolicyViolations > 0 {
		return errors.New("policy violation blocks promotion")
	}
	return nil
}
