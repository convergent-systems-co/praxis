package contracts

import "testing"

func acceptedPlan() WorkPlan {
	return WorkPlan{
		AuthorityRef:    "docs/PLAN/example.md#unit",
		AuthorityDigest: "sha256:accepted",
		Candidates:      []WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: ProvenancePLAN}},
	}
}

func TestMaterializeWorkPlanRequiresAcceptedAuthority(t *testing.T) {
	plan := acceptedPlan()
	plan.Candidates[0].Provenance = ProvenanceModelProposal
	if _, _, err := MaterializeWorkPlan(plan); err == nil {
		t.Fatal("model proposal was materialized as runnable work")
	}
}

func TestMaterializeWorkPlanRejectsRelationshipsOutsideAcceptedSet(t *testing.T) {
	plan := acceptedPlan()
	plan.Relationships = []WorkRelationship{{
		Dependent: "unit", Prerequisite: "missing", Kind: RelationshipHardDependency,
		SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: ProvenancePLAN,
	}}
	if _, _, err := MaterializeWorkPlan(plan); err == nil {
		t.Fatal("relationship to an unmaterialized prerequisite was accepted")
	}
}

func TestMaterializeWorkPlanReturnsAcceptedCandidatesOnly(t *testing.T) {
	plan := acceptedPlan()
	candidates, relationships, err := MaterializeWorkPlan(plan)
	if err != nil || len(candidates) != 1 || candidates[0].ID != "unit" || len(relationships) != 0 {
		t.Fatalf("unexpected materialized work plan: candidates=%+v relationships=%+v err=%v", candidates, relationships, err)
	}
}
