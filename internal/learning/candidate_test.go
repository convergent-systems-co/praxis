package learning

import "testing"

func TestCandidateCannotSkipEvaluationToStabilized(t *testing.T) {
	if CanTransition(CandidateProposed, CandidateStabilized) {
		t.Fatal("candidate must not skip evaluation/governance")
	}
}

func TestPromotedCandidateCanDemote(t *testing.T) {
	if err := ValidateTransition(CandidatePromoted, CandidateDemoted); err != nil {
		t.Fatalf("promoted candidate must be demotable: %v", err)
	}
}

func TestCandidateRequiresRollbackPlan(t *testing.T) {
	c := Candidate{ID: "c1", Target: "graph:g1", SourceObservations: []string{"o1"}, RequiredEvaluation: "suite:1"}
	if err := c.Validate(); err == nil {
		t.Fatal("candidate without rollback reference must fail")
	}
}
