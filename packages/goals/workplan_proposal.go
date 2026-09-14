package goals

import (
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// BuildWorkPlanProposal creates advisory decomposition from explicitly
// supplied candidates. It verifies the immutable baseline and requirement
// references but never derives children from prose, PlanRef, or repository
// shape, and never grants execution authority.
func BuildWorkPlanProposal(baseline GoalBaseline, id string, proposer contracts.PrincipalRef, proposerGeneration string, candidates []contracts.WorkCandidate, relationships []contracts.WorkRelationship) (contracts.WorkPlanProposal, error) {
	if err := baseline.VerifyDigest(); err != nil || baseline.Digest == "" || proposerGeneration == "" {
		return contracts.WorkPlanProposal{}, fmt.Errorf("proposal requires a verified Goal Baseline: %w", err)
	}
	proposal := contracts.WorkPlanProposal{ID: id, GoalID: baseline.ID, GoalVersion: baseline.Version, BaselineDigest: baseline.Digest, ProposedBy: proposer, ProposerGeneration: proposerGeneration, Candidates: candidates, Relationships: relationships}
	if err := proposal.Validate(); err != nil {
		return contracts.WorkPlanProposal{}, err
	}
	return proposal, nil
}
