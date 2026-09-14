package develop

import (
	"testing"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

func TestMaterializePlanningBaselineBindsExactGoalDigest(t *testing.T) {
	goal := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "build it", RefinedOutcome: "deliver secure service", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationReviewAll}
	b, err := MaterializeBaseline(goal, []DevelopmentArtifact{{ID: "adr-001", Role: RoleADR, SourceGoal: "decision-1"}, {ID: "spec-001", Role: RoleSpecification, SourceGoal: "spec-1"}}, "plan-001")
	if err != nil {
		t.Fatal(err)
	}
	if b.GoalBaselineDigest == "" {
		t.Fatal("planning baseline must bind Goal Baseline digest")
	}
}

func TestMaterializationRejectsMutatedGoalBaseline(t *testing.T) {
	goal := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "build it", RefinedOutcome: "deliver secure service", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationReviewAll}
	d, _ := goal.ComputeDigest()
	goal.Digest = d
	goal.RefinedOutcome = "silently changed"
	if _, err := MaterializeBaseline(goal, nil, ""); err == nil {
		t.Fatal("mutated Goal Baseline must not materialize")
	}
}

func TestSliceMustBindPlanningBaselineAndKnownArtifacts(t *testing.T) {
	goal := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "build it", RefinedOutcome: "deliver secure service", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationReviewAll}
	b, err := MaterializeBaseline(goal, []DevelopmentArtifact{{ID: "adr-001", Role: RoleADR}, {ID: "spec-001", Role: RoleSpecification}}, "plan-001")
	if err != nil {
		t.Fatal(err)
	}
	s := SliceContract{ID: "slice-1", PlanningBaselineID: b.GoalBaselineID, GoalBaselineDigest: b.GoalBaselineDigest, GoverningArtifacts: []string{"adr-001", "spec-001"}, Scope: "auth service", AcceptanceEvidence: []string{"tests pass"}}
	if err := s.Validate(b); err != nil {
		t.Fatal(err)
	}
	s.GoverningArtifacts = []string{"missing"}
	if err := s.Validate(b); err == nil {
		t.Fatal("slice cannot reference unknown planning artifact")
	}
}

func TestSliceCannotUseDifferentGoalDigest(t *testing.T) {
	goal := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "build it", RefinedOutcome: "deliver secure service", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll}
	b, err := MaterializeBaseline(goal, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	s := SliceContract{ID: "slice-1", PlanningBaselineID: b.GoalBaselineID, GoalBaselineDigest: "sha256:other", Scope: "auth service"}
	if err := s.Validate(b); err == nil {
		t.Fatal("slice with a different Goal Baseline digest must fail")
	}
}

func TestPlanningBaselineReusesAndInvalidatesOnlyDependentSlices(t *testing.T) {
	goal := goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "build it", RefinedOutcome: "deliver secure service", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationReviewAll}
	baseline, err := MaterializeBaseline(goal, []DevelopmentArtifact{
		{ID: "adr-auth", Role: RoleADR, Digest: "sha256:adr-v1"},
		{ID: "spec-auth", Role: RoleSpecification, Digest: "sha256:spec-v1"},
		{ID: "plan-auth", Role: RoleMasterPlan, Digest: "sha256:plan-v1"},
		{ID: "adr-docs", Role: RoleADR, Digest: "sha256:docs-v1"},
	}, "plan-1")
	if err != nil {
		t.Fatal(err)
	}
	baseline.RequirementsDigest = "sha256:req-v1"
	baseline.Dependencies = []ArtifactDependency{
		{Upstream: "adr-auth", Downstream: "spec-auth"},
		{Upstream: "spec-auth", Downstream: "plan-auth"},
	}
	if err := baseline.Validate(); err != nil {
		t.Fatal(err)
	}
	current := WorkspaceSnapshot{GoalBaselineID: "goal-1", GoalBaselineVersion: "1", RequirementsDigest: "sha256:req-v1", EvidenceDigests: map[string]string{
		"adr-auth": "sha256:adr-v1", "spec-auth": "sha256:spec-v1", "plan-auth": "sha256:plan-v1", "adr-docs": "sha256:docs-v1",
	}}
	reused, err := EvaluateApplicability(baseline, current, ApplicabilityRequest{SliceID: "auth-tests", GoverningArtifacts: []string{"plan-auth"}})
	if err != nil || reused.Decision != ApplicabilityReuse {
		t.Fatalf("expected reusable baseline, got %+v (err=%v)", reused, err)
	}
	current.EvidenceDigests["adr-auth"] = "sha256:adr-v2"
	delta, err := EvaluateApplicability(baseline, current, ApplicabilityRequest{SliceID: "auth-tests", GoverningArtifacts: []string{"plan-auth"}})
	if err != nil || delta.Decision != ApplicabilityDelta {
		t.Fatalf("expected dependency-aware delta, got %+v (err=%v)", delta, err)
	}
	if got, want := delta.InvalidatedArtifacts, []string{"adr-auth", "plan-auth", "spec-auth"}; !equalStrings(got, want) {
		t.Fatalf("unexpected invalidation closure: got %v want %v", got, want)
	}
	unrelated, err := EvaluateApplicability(baseline, current, ApplicabilityRequest{SliceID: "docs", GoverningArtifacts: []string{"adr-docs"}})
	if err != nil || unrelated.Decision != ApplicabilityReuse {
		t.Fatalf("unrelated slice should reuse baseline, got %+v (err=%v)", unrelated, err)
	}
}

func TestPlanningLifecycleReusesBaselineAndSelectivelyReplansSlices(t *testing.T) {
	if got := ClassifyWork(WorkSignals{FilesLikelyAffected: 1, KnownAcceptanceTests: true, DeterministicFixKnown: true}); got.Outcome != "fast" {
		t.Fatalf("direct fast path was not selected: %+v", got)
	}
	if got := ClassifyWork(WorkSignals{ArchitectureChange: true}); got.Outcome != "goals" {
		t.Fatalf("material work without a baseline must build one: %+v", got)
	}
	goal := goals.GoalBaseline{ID: "goal-lifecycle", Version: "1", OriginalIntent: "ship a service", RefinedOutcome: "ship a reliable service", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationReviewAll}
	baseline, err := MaterializeBaseline(goal, []DevelopmentArtifact{
		{ID: "adr-runtime", Role: RoleADR, Digest: "sha256:runtime-v1"},
		{ID: "spec-runtime", Role: RoleSpecification, Digest: "sha256:spec-v1"},
		{ID: "plan-runtime", Role: RoleMasterPlan, Digest: "sha256:plan-v1"},
		{ID: "adr-docs", Role: RoleADR, Digest: "sha256:docs-v1"},
	}, "plan-runtime-v1")
	if err != nil {
		t.Fatal(err)
	}
	baseline.RequirementsDigest = "sha256:req-v1"
	baseline.Dependencies = []ArtifactDependency{{Upstream: "adr-runtime", Downstream: "spec-runtime"}, {Upstream: "spec-runtime", Downstream: "plan-runtime"}}
	snapshot := WorkspaceSnapshot{GoalBaselineID: goal.ID, GoalBaselineVersion: goal.Version, RequirementsDigest: baseline.RequirementsDigest, EvidenceDigests: map[string]string{
		"adr-runtime": "sha256:runtime-v1", "spec-runtime": "sha256:spec-v1", "plan-runtime": "sha256:plan-v1", "adr-docs": "sha256:docs-v1",
	}}
	first, err := EvaluateApplicability(baseline, snapshot, ApplicabilityRequest{SliceID: "runtime-tests", GoverningArtifacts: []string{"plan-runtime"}})
	if err != nil || first.Decision != ApplicabilityReuse {
		t.Fatalf("first slice should reuse the current baseline: %+v (err=%v)", first, err)
	}
	second, err := EvaluateApplicability(baseline, snapshot, ApplicabilityRequest{SliceID: "docs", GoverningArtifacts: []string{"adr-docs"}})
	if err != nil || second.Decision != ApplicabilityReuse {
		t.Fatalf("independent slice should reuse the current baseline: %+v (err=%v)", second, err)
	}
	snapshot.EvidenceDigests["adr-runtime"] = "sha256:runtime-v2"
	delta, err := EvaluateApplicability(baseline, snapshot, ApplicabilityRequest{SliceID: "runtime-tests", GoverningArtifacts: []string{"plan-runtime"}})
	if err != nil || delta.Decision != ApplicabilityDelta || !equalStrings(delta.InvalidatedArtifacts, []string{"adr-runtime", "plan-runtime", "spec-runtime"}) {
		t.Fatalf("runtime slice did not receive exact dependency-aware delta: %+v (err=%v)", delta, err)
	}
	unchanged, err := EvaluateApplicability(baseline, snapshot, ApplicabilityRequest{SliceID: "docs", GoverningArtifacts: []string{"adr-docs"}})
	if err != nil || unchanged.Decision != ApplicabilityReuse {
		t.Fatalf("unrelated docs slice was unnecessarily invalidated: %+v (err=%v)", unchanged, err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
