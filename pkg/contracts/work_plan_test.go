package contracts

import "testing"

func acceptedPlan() WorkPlan {
	return WorkPlan{
		BaselineDigest: "sha256:baseline",
		AuthorityRef:   "docs/PLAN/example.md#unit", AuthorityDigest: "sha256:accepted",
		AcceptanceRef: "acceptance:unit", AcceptanceDigest: "sha256:acceptance",
		AcceptedBy: PrincipalRef{ID: "reviewer", Kind: "human"}, ProposalDigest: "sha256:proposal",
		Candidates: []WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: ProvenancePLAN, Requirements: []RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}},
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

func TestAcceptWorkPlanSeparatesProposalAndAcceptanceAndBindsBaseline(t *testing.T) {
	proposal := WorkPlanProposal{ID: "proposal-1", GoalID: "goal", GoalVersion: "2", BaselineDigest: "sha256:baseline", ProposedBy: PrincipalRef{ID: "model-1", Kind: "model"}, Candidates: []WorkCandidate{{ID: "unit", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: ProvenanceModelProposal, Requirements: []RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}}}
	proposalDigest, err := proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := AcceptWorkPlan(proposal, WorkPlan{Candidates: []WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: ProvenancePLAN, Requirements: proposal.Candidates[0].Requirements}}}, WorkPlanAcceptance{ProposalDigest: proposalDigest, BaselineDigest: "sha256:baseline", AuthorityRef: "docs/PLAN/example.md#unit", AuthorityDigest: "sha256:accepted", AcceptanceRef: "acceptance:unit", AcceptanceDigest: "sha256:acceptance", AcceptedBy: PrincipalRef{ID: "reviewer", Kind: "human"}, ReviewDigest: "sha256:review", Mode: "human"})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.ProposalDigest != proposalDigest || accepted.AcceptedBy.ID == proposal.ProposedBy.ID {
		t.Fatalf("acceptance did not preserve independent binding: %+v", accepted)
	}
}

func TestAcceptWorkPlanRejectsSelfAcceptanceAndStaleBaseline(t *testing.T) {
	proposal := WorkPlanProposal{ID: "proposal-1", GoalID: "goal", GoalVersion: "2", BaselineDigest: "sha256:baseline", ProposedBy: PrincipalRef{ID: "actor", Kind: "model"}, Candidates: []WorkCandidate{{ID: "unit", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: ProvenanceModelProposal, Requirements: []RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}}}
	proposalDigest, err := proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := WorkPlanAcceptance{ProposalDigest: proposalDigest, BaselineDigest: "sha256:stale", AuthorityRef: "docs/PLAN/example.md#unit", AuthorityDigest: "sha256:accepted", AcceptanceRef: "acceptance:unit", AcceptanceDigest: "sha256:acceptance", AcceptedBy: PrincipalRef{ID: "actor", Kind: "model"}, ReviewDigest: "sha256:review", Mode: "policy"}
	if _, err := AcceptWorkPlan(proposal, WorkPlan{Candidates: []WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: ProvenancePLAN}}}, decision); err == nil {
		t.Fatal("stale/self-accepted proposal was authorized")
	}
}

func TestAcceptWorkPlanRejectsModelAsIndependentPolicyAuthority(t *testing.T) {
	proposal := WorkPlanProposal{ID: "proposal-1", GoalID: "goal", GoalVersion: "2", BaselineDigest: "sha256:baseline", ProposedBy: PrincipalRef{ID: "model-1", Kind: "model"}, Candidates: []WorkCandidate{{ID: "unit", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: ProvenanceModelProposal, Requirements: []RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}}}
	proposalDigest, err := proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := WorkPlanAcceptance{ProposalDigest: proposalDigest, BaselineDigest: "sha256:baseline", AuthorityRef: "docs/PLAN/example.md#unit", AuthorityDigest: "sha256:accepted", AcceptanceRef: "acceptance:unit", AcceptanceDigest: "sha256:acceptance", AcceptedBy: PrincipalRef{ID: "model-2", Kind: "model"}, ReviewDigest: "sha256:review", Mode: "policy"}
	if _, err := AcceptWorkPlan(proposal, WorkPlan{Candidates: []WorkCandidate{{ID: "unit", SourceRef: "docs/PLAN/example.md#unit", SourceDigest: "sha256:accepted", Provenance: ProvenancePLAN}}}, decision); err == nil {
		t.Fatal("model was accepted as independent policy authority")
	}
}

func TestWorkPlanProposalReviewRequiresIndependentGenerationAndExactBinding(t *testing.T) {
	proposal := WorkPlanProposal{ID: "proposal-1", GoalID: "goal", GoalVersion: "2", BaselineDigest: "sha256:baseline", ProposedBy: PrincipalRef{ID: "model-1", Kind: "model"}, ProposerGeneration: "generation-1", Candidates: []WorkCandidate{{ID: "unit", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: ProvenanceModelProposal, Requirements: []RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}}}
	digest, err := proposal.Digest()
	if err != nil {
		t.Fatal(err)
	}
	review := WorkPlanProposalReview{ProposalDigest: digest, BaselineDigest: proposal.BaselineDigest, ReviewRef: "review-1", ReviewDigest: "sha256:review", ReviewedBy: PrincipalRef{ID: "reviewer", Kind: "agent"}, ReviewerGeneration: "generation-2", Status: ReviewAcceptableForAuthority, CoveredRequirements: []string{"req-1"}}
	if err := review.Validate(proposal); err != nil {
		t.Fatal(err)
	}
	review.ReviewerGeneration = "generation-1"
	if err := review.Validate(proposal); err == nil {
		t.Fatal("same generation was accepted as independent review")
	}
	review.ReviewerGeneration = "generation-2"
	review.ProposalDigest = "sha256:stale"
	if err := review.Validate(proposal); err == nil {
		t.Fatal("stale proposal review was accepted")
	}
}
