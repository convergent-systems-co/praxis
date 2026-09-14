package goals

import (
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestBuildWorkPlanProposalBindsExplicitRequirementsWithoutAuthority(t *testing.T) {
	baseline := baselineFixture()
	digest, err := baseline.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	baseline.Digest = digest
	candidates := []contracts.WorkCandidate{{ID: "unit", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: contracts.ProvenanceModelProposal, Requirements: []contracts.RequirementRef{{ID: "req-1", SourceRef: "goal:requirement/1", SourceDigest: "sha256:req"}}}}
	proposal, err := BuildWorkPlanProposal(baseline, "proposal-1", contracts.PrincipalRef{ID: "planner", Kind: "model"}, "generation-1", candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.BaselineDigest != digest || proposal.Candidates[0].Provenance != contracts.ProvenanceModelProposal {
		t.Fatalf("proposal lost baseline/provenance boundary: %+v", proposal)
	}
	if _, err := BuildWorkPlanProposal(baseline, "proposal-2", contracts.PrincipalRef{ID: "planner", Kind: "model"}, "generation-1", []contracts.WorkCandidate{{ID: "unbound", SourceRef: "model:proposal", SourceDigest: "sha256:model", Provenance: contracts.ProvenanceModelProposal}}, nil); err == nil {
		t.Fatal("child without requirement provenance became a proposal")
	}
	if _, err := BuildWorkPlanProposal(baseline, "proposal-3", contracts.PrincipalRef{ID: "planner", Kind: "model"}, "generation-1", candidates, nil); err != nil {
		t.Fatal(err)
	}
}
