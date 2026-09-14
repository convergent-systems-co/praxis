package goaldrive

import (
	"testing"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestMaterializeGoalWorkDoesNotInferChildrenFromBaselineText(t *testing.T) {
	baseline := goals.GoalBaseline{ID: "g", Version: "1", OriginalIntent: "complete issue 103", RefinedOutcome: "ship controller", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll}
	if _, _, err := MaterializeGoalWork(baseline); err != ErrNoAcceptedGoalWorkPlan {
		t.Fatalf("expected missing accepted work plan, got %v", err)
	}
}

func TestMaterializeGoalWorkUsesOnlyAcceptedPlan(t *testing.T) {
	baseline := goals.GoalBaseline{ID: "g", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll,
		WorkPlan: &contracts.WorkPlan{AuthorityRef: "docs/PLAN/example.md#unit", AuthorityDigest: "sha256:accepted", AcceptanceRef: "acceptance:unit", AcceptanceDigest: "sha256:acceptance", AcceptedBy: contracts.PrincipalRef{ID: "reviewer", Kind: "human"}, ProposalDigest: "sha256:proposal", Candidates: []contracts.WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: contracts.ProvenancePLAN}}}}
	candidates, _, err := MaterializeGoalWork(baseline)
	if err != nil || len(candidates) != 1 || candidates[0].ID != "unit" {
		t.Fatalf("unexpected materialization: %+v, %v", candidates, err)
	}
}
