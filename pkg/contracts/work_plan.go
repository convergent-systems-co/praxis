package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// WorkPlan is the accepted executable decomposition of a durable Goal
// baseline. A model may propose candidates, but only a plan persisted with an
// authority reference can be materialized for controller selection.
type WorkPlan struct {
	AuthorityRef     string             `json:"authority_ref"`
	AuthorityDigest  string             `json:"authority_digest"`
	AcceptanceRef    string             `json:"acceptance_ref"`
	AcceptanceDigest string             `json:"acceptance_digest"`
	AcceptedBy       PrincipalRef       `json:"accepted_by"`
	ProposalDigest   string             `json:"proposal_digest"`
	Candidates       []WorkCandidate    `json:"candidates,omitempty"`
	Relationships    []WorkRelationship `json:"relationships,omitempty"`
}

// WorkPlanProposal is advisory decomposition. It may contain model-derived
// candidates and edges, but it is never selector input.
type WorkPlanProposal struct {
	ID             string             `json:"id"`
	GoalID         string             `json:"goal_id"`
	GoalVersion    string             `json:"goal_version"`
	BaselineDigest string             `json:"baseline_digest"`
	ProposedBy     PrincipalRef       `json:"proposed_by"`
	Candidates     []WorkCandidate    `json:"candidates,omitempty"`
	Relationships  []WorkRelationship `json:"relationships,omitempty"`
}

type WorkPlanAcceptance struct {
	ProposalDigest   string       `json:"proposal_digest"`
	BaselineDigest   string       `json:"baseline_digest"`
	AuthorityRef     string       `json:"authority_ref"`
	AuthorityDigest  string       `json:"authority_digest"`
	AcceptanceRef    string       `json:"acceptance_ref"`
	AcceptanceDigest string       `json:"acceptance_digest"`
	AcceptedBy       PrincipalRef `json:"accepted_by"`
	ReviewDigest     string       `json:"review_digest"`
	Mode             string       `json:"mode"` // human or policy
}

var ErrUnacceptedWorkPlan = errors.New("work plan is not an accepted authoritative decomposition")

func (p WorkPlan) Validate() error {
	if p.AuthorityRef == "" || p.AuthorityDigest == "" || p.AcceptanceRef == "" || p.AcceptanceDigest == "" || p.ProposalDigest == "" {
		return fmt.Errorf("%w: authority, acceptance, and proposal bindings are required", ErrUnacceptedWorkPlan)
	}
	if err := p.AcceptedBy.Validate(); err != nil {
		return fmt.Errorf("%w: accepted-by principal: %v", ErrUnacceptedWorkPlan, err)
	}
	if len(p.Candidates) == 0 {
		return fmt.Errorf("%w: at least one candidate is required", ErrUnacceptedWorkPlan)
	}
	seen := make(map[string]struct{}, len(p.Candidates))
	for _, candidate := range p.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if _, ok := seen[candidate.ID]; ok {
			return fmt.Errorf("%w: duplicate candidate %q", ErrUnacceptedWorkPlan, candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
	}
	for _, relationship := range p.Relationships {
		if err := relationship.Validate(); err != nil {
			return err
		}
		if _, ok := seen[relationship.Dependent]; !ok {
			return fmt.Errorf("%w: relationship dependent %q is not a candidate", ErrUnacceptedWorkPlan, relationship.Dependent)
		}
		if _, ok := seen[relationship.Prerequisite]; !ok {
			return fmt.Errorf("%w: relationship prerequisite %q is not a candidate", ErrUnacceptedWorkPlan, relationship.Prerequisite)
		}
	}
	return nil
}

func (p WorkPlanProposal) Validate() error {
	if p.ID == "" || p.GoalID == "" || p.GoalVersion == "" || p.BaselineDigest == "" {
		return fmt.Errorf("%w: proposal and baseline identity are required", ErrUnacceptedWorkPlan)
	}
	if err := p.ProposedBy.Validate(); err != nil {
		return fmt.Errorf("%w: proposer: %v", ErrUnacceptedWorkPlan, err)
	}
	if len(p.Candidates) == 0 {
		return fmt.Errorf("%w: proposal must contain candidates", ErrUnacceptedWorkPlan)
	}
	seen := make(map[string]struct{}, len(p.Candidates))
	for _, candidate := range p.Candidates {
		if candidate.ID == "" || candidate.SourceRef == "" || candidate.SourceDigest == "" {
			return fmt.Errorf("%w: proposal candidate identity and provenance are required", ErrUnacceptedWorkPlan)
		}
		if _, ok := seen[candidate.ID]; ok {
			return fmt.Errorf("%w: duplicate proposal candidate %q", ErrUnacceptedWorkPlan, candidate.ID)
		}
		seen[candidate.ID] = struct{}{}
		if candidate.Provenance != ProvenanceModelProposal {
			if err := candidate.Validate(); err != nil {
				return err
			}
		}
	}
	for _, relationship := range p.Relationships {
		if relationship.Dependent == "" || relationship.Prerequisite == "" || relationship.SourceRef == "" || relationship.SourceDigest == "" || relationship.Dependent == relationship.Prerequisite {
			return fmt.Errorf("%w: proposal relationship identity and provenance are required", ErrUnacceptedWorkPlan)
		}
		if _, ok := seen[relationship.Dependent]; !ok {
			return fmt.Errorf("%w: proposal relationship dependent %q is not a candidate", ErrUnacceptedWorkPlan, relationship.Dependent)
		}
		if _, ok := seen[relationship.Prerequisite]; !ok {
			return fmt.Errorf("%w: proposal relationship prerequisite %q is not a candidate", ErrUnacceptedWorkPlan, relationship.Prerequisite)
		}
	}
	return nil
}

func (p WorkPlanProposal) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	c := p
	c.Candidates = append([]WorkCandidate(nil), p.Candidates...)
	sort.Slice(c.Candidates, func(i, j int) bool { return c.Candidates[i].ID < c.Candidates[j].ID })
	c.Relationships = append([]WorkRelationship(nil), p.Relationships...)
	sort.Slice(c.Relationships, func(i, j int) bool {
		if c.Relationships[i].Dependent != c.Relationships[j].Dependent {
			return c.Relationships[i].Dependent < c.Relationships[j].Dependent
		}
		return c.Relationships[i].Prerequisite < c.Relationships[j].Prerequisite
	})
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// AcceptWorkPlan is the only contract operation that turns a proposal into
// accepted selector input. The acceptance authority must be distinct from the
// proposer, bind the exact proposal and baseline generation, and carry
// independent review evidence. Authenticity of the accepting principal is
// supplied by the durable authority/event provider, not by model output.
func AcceptWorkPlan(proposal WorkPlanProposal, accepted WorkPlan, decision WorkPlanAcceptance) (WorkPlan, error) {
	if err := proposal.Validate(); err != nil {
		return WorkPlan{}, err
	}
	proposalDigest, err := proposal.Digest()
	if err != nil {
		return WorkPlan{}, err
	}
	if decision.ProposalDigest != proposalDigest || decision.BaselineDigest != proposal.BaselineDigest {
		return WorkPlan{}, fmt.Errorf("%w: proposal or baseline digest does not match acceptance", ErrUnacceptedWorkPlan)
	}
	if decision.AuthorityRef == "" || decision.AuthorityDigest == "" || decision.AcceptanceRef == "" || decision.AcceptanceDigest == "" || decision.ReviewDigest == "" {
		return WorkPlan{}, fmt.Errorf("%w: acceptance authority and independent review are required", ErrUnacceptedWorkPlan)
	}
	if err := decision.AcceptedBy.Validate(); err != nil {
		return WorkPlan{}, err
	}
	if decision.AcceptedBy.ID == proposal.ProposedBy.ID {
		return WorkPlan{}, fmt.Errorf("%w: proposer cannot accept its own decomposition", ErrUnacceptedWorkPlan)
	}
	if decision.Mode != "human" && decision.Mode != "policy" {
		return WorkPlan{}, fmt.Errorf("%w: acceptance mode must be human or policy", ErrUnacceptedWorkPlan)
	}
	if decision.Mode == "human" && decision.AcceptedBy.Kind != "human" {
		return WorkPlan{}, fmt.Errorf("%w: human acceptance requires a human authority", ErrUnacceptedWorkPlan)
	}
	if decision.Mode == "policy" && decision.AcceptedBy.Kind != "policy" && decision.AcceptedBy.Kind != "controller" {
		return WorkPlan{}, fmt.Errorf("%w: policy acceptance requires policy/controller authority", ErrUnacceptedWorkPlan)
	}
	if len(accepted.Candidates) == 0 {
		return WorkPlan{}, fmt.Errorf("%w: accepted plan must contain candidates", ErrUnacceptedWorkPlan)
	}
	proposalIDs := make(map[string]WorkCandidate, len(proposal.Candidates))
	for _, candidate := range proposal.Candidates {
		proposalIDs[candidate.ID] = candidate
	}
	proposalRelationships := make(map[string]struct{}, len(proposal.Relationships))
	for _, relationship := range proposal.Relationships {
		proposalRelationships[relationship.Dependent+"\x00"+relationship.Prerequisite+"\x00"+string(relationship.Kind)] = struct{}{}
	}
	for _, candidate := range accepted.Candidates {
		if _, ok := proposalIDs[candidate.ID]; !ok {
			return WorkPlan{}, fmt.Errorf("%w: accepted candidate %q was not proposed", ErrUnacceptedWorkPlan, candidate.ID)
		}
		if candidate.Provenance == ProvenanceModelProposal {
			return WorkPlan{}, ErrInferredWorkSelection
		}
	}
	for _, relationship := range accepted.Relationships {
		key := relationship.Dependent + "\x00" + relationship.Prerequisite + "\x00" + string(relationship.Kind)
		if _, ok := proposalRelationships[key]; !ok {
			return WorkPlan{}, fmt.Errorf("%w: accepted relationship was not proposed", ErrUnacceptedWorkPlan)
		}
	}
	accepted.AuthorityRef, accepted.AuthorityDigest = decision.AuthorityRef, decision.AuthorityDigest
	accepted.AcceptanceRef, accepted.AcceptanceDigest = decision.AcceptanceRef, decision.AcceptanceDigest
	accepted.AcceptedBy, accepted.ProposalDigest = decision.AcceptedBy, proposalDigest
	if err := accepted.Validate(); err != nil {
		return WorkPlan{}, err
	}
	return accepted, nil
}

// MaterializeWorkPlan returns only an accepted, validated decomposition. It
// deliberately has no path for model-proposed work to become runnable.
func MaterializeWorkPlan(plan WorkPlan) ([]WorkCandidate, []WorkRelationship, error) {
	if err := plan.Validate(); err != nil {
		return nil, nil, err
	}
	return append([]WorkCandidate(nil), plan.Candidates...), append([]WorkRelationship(nil), plan.Relationships...), nil
}
