package develop

import (
	"testing"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

func applicabilityFixture(t *testing.T) PlanningBaseline {
	t.Helper()
	goal := goals.GoalBaseline{ID: "goal-security", Version: "1", OriginalIntent: "secure", RefinedOutcome: "secure service", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationReviewAll}
	baseline, err := MaterializeBaseline(goal, []DevelopmentArtifact{{ID: "adr", Role: RoleADR, Digest: "sha256:adr"}, {ID: "plan", Role: RoleMasterPlan, Digest: "sha256:plan"}}, "plan")
	if err != nil {
		t.Fatal(err)
	}
	baseline.RequirementsDigest = "sha256:req"
	baseline.Dependencies = []ArtifactDependency{{Upstream: "adr", Downstream: "plan"}}
	return baseline
}

func TestApplicabilityFailsClosedForMissingEvidenceAndIdentity(t *testing.T) {
	baseline := applicabilityFixture(t)
	missing, err := EvaluateApplicability(baseline, WorkspaceSnapshot{GoalBaselineID: baseline.GoalBaselineID, GoalBaselineVersion: baseline.GoalBaselineVersion, RequirementsDigest: baseline.RequirementsDigest, EvidenceDigests: map[string]string{"adr": "sha256:adr"}}, ApplicabilityRequest{SliceID: "plan", GoverningArtifacts: []string{"plan"}})
	if err != nil || missing.Decision != ApplicabilityDelta {
		t.Fatalf("missing plan evidence must invalidate the dependent slice: %+v (err=%v)", missing, err)
	}
	wrongIdentity, err := EvaluateApplicability(baseline, WorkspaceSnapshot{GoalBaselineID: "other", GoalBaselineVersion: "1"}, ApplicabilityRequest{SliceID: "plan"})
	if err != nil || wrongIdentity.Decision != ApplicabilityReplan {
		t.Fatalf("changed baseline identity must replan: %+v (err=%v)", wrongIdentity, err)
	}
	wrongDependency := baseline
	wrongDependency.Dependencies = []ArtifactDependency{{Upstream: "unknown", Downstream: "plan"}}
	if _, err := EvaluateApplicability(wrongDependency, WorkspaceSnapshot{GoalBaselineID: baseline.GoalBaselineID, GoalBaselineVersion: baseline.GoalBaselineVersion}, ApplicabilityRequest{SliceID: "plan"}); err == nil {
		t.Fatal("unknown dependency endpoint must fail closed")
	}
}
