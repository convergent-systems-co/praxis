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
	BaselineDigest            string             `json:"baseline_digest"`
	AuthorityRef              string             `json:"authority_ref"`
	AuthorityDigest           string             `json:"authority_digest"`
	AcceptanceRef             string             `json:"acceptance_ref"`
	AcceptanceDigest          string             `json:"acceptance_digest"`
	AcceptedBy                PrincipalRef       `json:"accepted_by"`
	ProposalDigest            string             `json:"proposal_digest"`
	AuthorityRequestID        string             `json:"authority_request_id,omitempty"`
	AuthorityRequestVersion   string             `json:"authority_request_version,omitempty"`
	AuthorityDecisionRef      string             `json:"authority_decision_ref,omitempty"`
	AuthorityDecisionVersion  string             `json:"authority_decision_version,omitempty"`
	AuthorityVersion          string             `json:"authority_version,omitempty"`
	AuthorityGenerationDigest string             `json:"authority_generation_digest,omitempty"`
	Candidates                []WorkCandidate    `json:"candidates,omitempty"`
	Relationships             []WorkRelationship `json:"relationships,omitempty"`
}

// WorkPlanProposal is advisory decomposition. It may contain model-derived
// candidates and edges, but it is never selector input.
type WorkPlanProposal struct {
	ID                 string             `json:"id"`
	GoalID             string             `json:"goal_id"`
	GoalVersion        string             `json:"goal_version"`
	BaselineDigest     string             `json:"baseline_digest"`
	ProposedBy         PrincipalRef       `json:"proposed_by"`
	ProposerGeneration string             `json:"proposer_generation"`
	Candidates         []WorkCandidate    `json:"candidates,omitempty"`
	Relationships      []WorkRelationship `json:"relationships,omitempty"`
}

type WorkPlanAcceptance struct {
	ProposalDigest            string       `json:"proposal_digest"`
	BaselineDigest            string       `json:"baseline_digest"`
	AuthorityRef              string       `json:"authority_ref"`
	AuthorityDigest           string       `json:"authority_digest"`
	AcceptanceRef             string       `json:"acceptance_ref"`
	AcceptanceDigest          string       `json:"acceptance_digest"`
	AcceptedBy                PrincipalRef `json:"accepted_by"`
	AuthorityScope            string       `json:"authority_scope"`
	ReviewRef                 string       `json:"review_ref"`
	ReviewVersion             string       `json:"review_version"`
	ReviewDigest              string       `json:"review_digest"`
	AuthorityRequestID        string       `json:"authority_request_id,omitempty"`
	AuthorityRequestVersion   string       `json:"authority_request_version,omitempty"`
	AuthorityDecisionRef      string       `json:"authority_decision_ref,omitempty"`
	AuthorityDecisionVersion  string       `json:"authority_decision_version,omitempty"`
	AuthorityVersion          string       `json:"authority_version,omitempty"`
	AuthorityGenerationDigest string       `json:"authority_generation_digest,omitempty"`
	Mode                      string       `json:"mode"` // human or policy
}

var ErrUnacceptedWorkPlan = errors.New("work plan is not an accepted authoritative decomposition")

func (p WorkPlan) Validate() error {
	if p.BaselineDigest == "" || p.AuthorityRef == "" || p.AuthorityDigest == "" || p.AcceptanceRef == "" || p.AcceptanceDigest == "" || p.ProposalDigest == "" {
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
		if len(candidate.Requirements) == 0 {
			return fmt.Errorf("%w: proposal candidate %q lacks requirement provenance", ErrUnacceptedWorkPlan, candidate.ID)
		}
		for _, requirement := range candidate.Requirements {
			if err := requirement.Validate(); err != nil {
				return err
			}
		}
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
		if len(candidate.Requirements) == 0 {
			return fmt.Errorf("%w: proposal candidate %q lacks requirement provenance", ErrUnacceptedWorkPlan, candidate.ID)
		}
		for _, requirement := range candidate.Requirements {
			if err := requirement.Validate(); err != nil {
				return err
			}
		}
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
		// A proposal may describe a hard dependency for independent review;
		// model provenance keeps it advisory and prevents selection until an
		// authority-backed acceptance promotes it. WorkRelationship.Validate
		// intentionally rejects that same edge in executable state.
		if relationship.Provenance == ProvenanceModelProposal {
			switch relationship.Kind {
			case RelationshipHardDependency, RelationshipConsumer, RelationshipInteraction, RelationshipAdvisory:
				continue
			default:
				return fmt.Errorf("%w: unknown proposal relationship kind %q", ErrUnacceptedWorkPlan, relationship.Kind)
			}
		}
		if err := relationship.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type WorkPlanReviewStatus string

const (
	ReviewAcceptableForAuthority WorkPlanReviewStatus = "acceptable_for_authority_decision"
	ReviewRevisionRequired       WorkPlanReviewStatus = "revision_required"
	ReviewInsufficientEvidence   WorkPlanReviewStatus = "insufficient_evidence"
	ReviewAuthorityConflict      WorkPlanReviewStatus = "authority_conflict"
)

// WorkPlanProposalReview is independent evidence about a proposal. It never
// creates a WorkPlan or attaches one to a Goal Baseline.
type WorkPlanProposalReview struct {
	ProposalDigest      string               `json:"proposal_digest"`
	BaselineDigest      string               `json:"baseline_digest"`
	ReviewRef           string               `json:"review_ref"`
	ReviewDigest        string               `json:"review_digest"`
	ReviewedBy          PrincipalRef         `json:"reviewed_by"`
	ReviewerGeneration  string               `json:"reviewer_generation"`
	ReviewerProvider    string               `json:"reviewer_provider,omitempty"`
	Status              WorkPlanReviewStatus `json:"status"`
	CoveredRequirements []string             `json:"covered_requirements,omitempty"`
	MissingRequirements []string             `json:"missing_requirements,omitempty"`
	InventedScope       []string             `json:"invented_scope,omitempty"`
	Findings            []string             `json:"findings,omitempty"`
}

func (r WorkPlanProposalReview) Validate(proposal WorkPlanProposal) error {
	proposalDigest, err := proposal.Digest()
	if err != nil {
		return err
	}
	if r.ProposalDigest != proposalDigest || r.BaselineDigest != proposal.BaselineDigest || r.ReviewRef == "" || r.ReviewDigest == "" {
		return fmt.Errorf("%w: review must bind exact proposal, baseline, and review identity", ErrUnacceptedWorkPlan)
	}
	if err := r.ReviewedBy.Validate(); err != nil {
		return err
	}
	if proposal.ProposerGeneration == "" || r.ReviewedBy.ID == proposal.ProposedBy.ID || r.ReviewerGeneration == "" || r.ReviewerGeneration == proposal.ProposerGeneration {
		return fmt.Errorf("%w: reviewer identity and generation must be independent", ErrUnacceptedWorkPlan)
	}
	switch r.Status {
	case ReviewAcceptableForAuthority, ReviewRevisionRequired, ReviewInsufficientEvidence, ReviewAuthorityConflict:
	default:
		return fmt.Errorf("%w: unknown proposal review status %q", ErrUnacceptedWorkPlan, r.Status)
	}
	requirements := make(map[string]struct{})
	for _, candidate := range proposal.Candidates {
		for _, requirement := range candidate.Requirements {
			requirements[requirement.ID] = struct{}{}
		}
	}
	seen := make(map[string]struct{})
	for _, id := range append(append(append([]string{}, r.CoveredRequirements...), r.MissingRequirements...), r.InventedScope...) {
		if id == "" {
			return fmt.Errorf("%w: review finding identity is required", ErrUnacceptedWorkPlan)
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("%w: duplicate review finding %q", ErrUnacceptedWorkPlan, id)
		}
		seen[id] = struct{}{}
	}
	for _, id := range r.CoveredRequirements {
		if _, ok := requirements[id]; !ok {
			return fmt.Errorf("%w: review covers unknown requirement %q", ErrUnacceptedWorkPlan, id)
		}
	}
	for _, id := range r.MissingRequirements {
		if _, ok := requirements[id]; !ok {
			return fmt.Errorf("%w: review marks unknown requirement missing %q", ErrUnacceptedWorkPlan, id)
		}
	}
	if r.Status == ReviewAcceptableForAuthority && (len(r.MissingRequirements) != 0 || len(r.InventedScope) != 0 || len(r.CoveredRequirements) != len(requirements)) {
		return fmt.Errorf("%w: acceptable review requires complete requirement coverage and no invented scope", ErrUnacceptedWorkPlan)
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

// MaterializeAcceptedPlanCandidate converts the advisory portions of a
// proposal into a candidate for authority-backed acceptance. It does not
// grant authority: callers must still pass the result through AcceptWorkPlan
// (normally via the durable authority-decision bridge in goalstore).
//
// A planner may introduce candidates and relationships with
// model_proposal provenance so they can be reviewed without becoming
// selector input. Once an exact request binding that proposal is approved,
// those advisory records are re-sourced to that request and become plan
// provenance. Already-authoritative source records retain their provenance.
func MaterializeAcceptedPlanCandidate(proposal WorkPlanProposal, authorityRef, authorityDigest string) (WorkPlan, error) {
	if err := proposal.Validate(); err != nil {
		return WorkPlan{}, err
	}
	if authorityRef == "" {
		return WorkPlan{}, fmt.Errorf("%w: accepted plan candidate requires an authority source", ErrUnacceptedWorkPlan)
	}
	if err := ValidateSHA256Digest(authorityDigest); err != nil {
		return WorkPlan{}, fmt.Errorf("%w: accepted plan candidate authority digest: %v", ErrUnacceptedWorkPlan, err)
	}
	plan := WorkPlan{
		Candidates:    append([]WorkCandidate(nil), proposal.Candidates...),
		Relationships: append([]WorkRelationship(nil), proposal.Relationships...),
	}
	for i := range plan.Candidates {
		plan.Candidates[i].Requirements = append([]RequirementRef(nil), plan.Candidates[i].Requirements...)
		if plan.Candidates[i].Provenance == ProvenanceModelProposal {
			plan.Candidates[i].Provenance = ProvenancePLAN
			plan.Candidates[i].SourceRef = authorityRef + "#candidate:" + plan.Candidates[i].ID
			plan.Candidates[i].SourceDigest = authorityDigest
		}
	}
	for i := range plan.Relationships {
		if plan.Relationships[i].Provenance == ProvenanceModelProposal {
			plan.Relationships[i].Provenance = ProvenancePLAN
			plan.Relationships[i].SourceRef = authorityRef + "#relationship:" + plan.Relationships[i].Dependent + ":" + plan.Relationships[i].Prerequisite
			plan.Relationships[i].SourceDigest = authorityDigest
		}
	}
	return plan, nil
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
	if decision.AuthorityRef == "" || decision.AuthorityDigest == "" || decision.AuthorityScope == "" || decision.AcceptanceRef == "" || decision.AcceptanceDigest == "" || decision.ReviewRef == "" || decision.ReviewVersion == "" || decision.ReviewDigest == "" {
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
		proposed, ok := proposalIDs[candidate.ID]
		if !ok {
			return WorkPlan{}, fmt.Errorf("%w: accepted candidate %q was not proposed", ErrUnacceptedWorkPlan, candidate.ID)
		}
		if !sameRequirements(candidate.Requirements, proposed.Requirements) {
			return WorkPlan{}, fmt.Errorf("%w: accepted candidate %q changed requirement provenance", ErrUnacceptedWorkPlan, candidate.ID)
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
	accepted.BaselineDigest = proposal.BaselineDigest
	accepted.AuthorityRef, accepted.AuthorityDigest = decision.AuthorityRef, decision.AuthorityDigest
	accepted.AcceptanceRef, accepted.AcceptanceDigest = decision.AcceptanceRef, decision.AcceptanceDigest
	accepted.AcceptedBy, accepted.ProposalDigest = decision.AcceptedBy, proposalDigest
	accepted.AuthorityRequestID = decision.AuthorityRequestID
	accepted.AuthorityRequestVersion = decision.AuthorityRequestVersion
	accepted.AuthorityDecisionRef = decision.AuthorityDecisionRef
	accepted.AuthorityDecisionVersion = decision.AuthorityDecisionVersion
	accepted.AuthorityVersion = decision.AuthorityVersion
	accepted.AuthorityGenerationDigest = decision.AuthorityGenerationDigest
	if err := accepted.Validate(); err != nil {
		return WorkPlan{}, err
	}
	return accepted, nil
}

func sameRequirements(a, b []RequirementRef) bool {
	if len(a) != len(b) {
		return false
	}
	left, right := append([]RequirementRef(nil), a...), append([]RequirementRef(nil), b...)
	sort.Slice(left, func(i, j int) bool { return left[i].ID < left[j].ID })
	sort.Slice(right, func(i, j int) bool { return right[i].ID < right[j].ID })
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// MaterializeWorkPlan returns only an accepted, validated decomposition. It
// deliberately has no path for model-proposed work to become runnable.
func MaterializeWorkPlan(plan WorkPlan) ([]WorkCandidate, []WorkRelationship, error) {
	if err := plan.Validate(); err != nil {
		return nil, nil, err
	}
	return append([]WorkCandidate(nil), plan.Candidates...), append([]WorkRelationship(nil), plan.Relationships...), nil
}
