package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// RequirementRef binds a proposed child to an authoritative requirement
// record. It is traceability, not execution authority.
type RequirementRef struct {
	ID            string `json:"id"`
	SourceRef     string `json:"source_ref"`
	SourceDigest  string `json:"source_digest"`
	Specification []byte `json:"specification,omitempty"`
}

func (r RequirementRef) ValidateSpecification() error {
	if len(r.Specification) == 0 {
		return fmt.Errorf("requirement %q has no preserved specification bytes", r.ID)
	}
	sum := sha256.Sum256(r.Specification)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	if digest != r.SourceDigest || !strings.HasSuffix(r.ID, ":"+digest) {
		return fmt.Errorf("requirement %q content-derived identity or digest mismatch", r.ID)
	}
	return nil
}

func (r RequirementRef) Validate() error {
	if r.ID == "" || r.SourceRef == "" || r.SourceDigest == "" {
		return fmt.Errorf("%w: requirement identity and provenance are required", ErrInvalidWorkRelationship)
	}
	return nil
}

// WorkCandidate is an authoritative durable work record eligible for
// selection. Lower Priority and Sequence values win; equal values are
// ambiguous and fail closed rather than depending on input order.
type WorkCandidate struct {
	ID                      string                 `json:"id"`
	Kind                    WorkCandidateKind      `json:"kind,omitempty"`
	Completed               bool                   `json:"completed"`
	Priority                int                    `json:"priority"`
	Sequence                int                    `json:"sequence"`
	SourceRef               string                 `json:"source_ref"`
	SourceDigest            string                 `json:"source_digest"`
	Provenance              RelationshipProvenance `json:"provenance"`
	Requirements            []RequirementRef       `json:"requirements,omitempty"`
	QualificationPredicates []string               `json:"qualification_predicates,omitempty"`
	Responsibility          string                 `json:"responsibility,omitempty"`
	Exclusions              []string               `json:"exclusions,omitempty"`
	// Specification is the exact source byte sequence accepted for this
	// candidate. Safety-kernel plans never re-read a mutable path at dispatch.
	Specification []byte `json:"specification,omitempty"`
}

type WorkCandidateKind string

const (
	WorkCandidateOrdinary      WorkCandidateKind = "work"
	WorkCandidateAuthorityGate WorkCandidateKind = "authority_gate"
)

var (
	ErrNoRunnableWork        = errors.New("no authoritative runnable work candidate")
	ErrAmbiguousWorkChoice   = errors.New("authoritative runnable work selection is ambiguous")
	ErrInferredWorkSelection = errors.New("model-derived candidate cannot authorize work selection")
)

type WorkSetState string

const (
	WorkSetRunnable WorkSetState = "runnable"
	WorkSetBlocked  WorkSetState = "blocked"
	WorkSetComplete WorkSetState = "complete"
)

type WorkCandidateAssessment struct {
	Candidate WorkCandidate `json:"candidate"`
	Readiness WorkReadiness `json:"readiness"`
	BlockedBy []string      `json:"blocked_by,omitempty"`
}

type WorkSetAssessment struct {
	State      WorkSetState              `json:"state"`
	Candidates []WorkCandidateAssessment `json:"candidates"`
	Selected   *WorkCandidate            `json:"selected,omitempty"`
}

func (c WorkCandidate) Validate() error {
	if c.ID == "" || c.SourceRef == "" || c.SourceDigest == "" {
		return fmt.Errorf("%w: candidate identity and provenance are required", ErrInvalidWorkRelationship)
	}
	switch c.Provenance {
	case ProvenanceADR, ProvenanceSPEC, ProvenancePLAN, ProvenanceContract, ProvenanceIssue, ProvenanceAuthorityGate:
		if c.Kind == WorkCandidateAuthorityGate && c.Provenance != ProvenanceAuthorityGate {
			return fmt.Errorf("%w: authority gate %q lacks authority_gate provenance", ErrInvalidWorkRelationship, c.ID)
		}
		if c.Provenance == ProvenanceAuthorityGate && c.Kind != WorkCandidateAuthorityGate {
			return fmt.Errorf("%w: authority_gate provenance requires authority_gate kind", ErrInvalidWorkRelationship)
		}
		return nil
	case ProvenanceModelProposal:
		return ErrInferredWorkSelection
	case ProvenanceModelGateProposal:
		return ErrInferredWorkSelection
	default:
		return fmt.Errorf("%w: unknown candidate provenance %q", ErrInvalidWorkRelationship, c.Provenance)
	}
}

// ValidateSpecification validates a proposal-time candidate: the preserved
// specification bytes must describe the executable record exactly.
func (c WorkCandidate) ValidateSpecification() error { return c.validateSpecification(false) }

// ValidateAcceptedSpecification validates a candidate of an accepted plan.
// Acceptance is the only transition that rewrites an executable record's
// provenance (model_proposal -> plan, model_gate_proposal -> authority_gate);
// the immutable specification bytes keep their proposal-time provenance. Every
// other field must still match exactly, and no other provenance pairing is
// admitted.
func (c WorkCandidate) ValidateAcceptedSpecification() error { return c.validateSpecification(true) }

func (c WorkCandidate) validateSpecification(accepted bool) error {
	if len(c.Specification) == 0 {
		return fmt.Errorf("candidate %q has no preserved specification bytes", c.ID)
	}
	sum := sha256.Sum256(c.Specification)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != c.SourceDigest {
		return fmt.Errorf("candidate %q specification digest mismatch: got %s want %s", c.ID, got, c.SourceDigest)
	}
	var spec struct {
		ID                      string                 `json:"id"`
		Kind                    WorkCandidateKind      `json:"kind"`
		Provenance              RelationshipProvenance `json:"provenance"`
		Priority                int                    `json:"priority"`
		Sequence                int                    `json:"sequence"`
		QualificationPredicates []string               `json:"qualification_predicates"`
		Responsibility          string                 `json:"responsibility"`
		Exclusions              []string               `json:"exclusions"`
		Requirements            []RequirementRef       `json:"requirements"`
	}
	if err := UnmarshalExactJSON(c.Specification, &spec, false); err != nil {
		return fmt.Errorf("candidate %q specification is not valid JSON: %w", c.ID, err)
	}
	if spec.ID != c.ID || spec.Kind != c.Kind || !specificationProvenanceMatches(spec.Provenance, c.Provenance, accepted) || spec.Priority != c.Priority || spec.Sequence != c.Sequence || spec.Responsibility != c.Responsibility || !slices.Equal(spec.Exclusions, c.Exclusions) || !slices.Equal(spec.QualificationPredicates, c.QualificationPredicates) || !sameRequirementIdentities(spec.Requirements, c.Requirements) {
		return fmt.Errorf("candidate %q specification content does not match executable record", c.ID)
	}
	if c.Kind == WorkCandidateOrdinary && len(c.QualificationPredicates) == 0 {
		return fmt.Errorf("candidate %q has no conformance qualification predicates", c.ID)
	}
	if c.Responsibility == "" || len(c.Exclusions) == 0 {
		return fmt.Errorf("candidate %q lacks bounded responsibility or exclusions", c.ID)
	}
	if c.Kind == WorkCandidateAuthorityGate {
		if _, err := ParseAuthorityGateContract(c.Specification); err != nil {
			return fmt.Errorf("authority gate %q: %w", c.ID, err)
		}
	}
	if _, err := ParseGovernedOutputContracts(c.Specification); err != nil {
		return fmt.Errorf("candidate %q governed outputs: %w", c.ID, err)
	}
	seen := map[string]struct{}{}
	for _, predicate := range c.QualificationPredicates {
		if predicate == "" {
			return fmt.Errorf("candidate %q has an empty qualification predicate", c.ID)
		}
		if _, duplicate := seen[predicate]; duplicate {
			return fmt.Errorf("candidate %q duplicates qualification predicate %q", c.ID, predicate)
		}
		seen[predicate] = struct{}{}
	}
	return nil
}

func sameRequirementIdentities(a, b []RequirementRef) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].SourceRef != b[i].SourceRef || a[i].SourceDigest != b[i].SourceDigest {
			return false
		}
	}
	return true
}

// SelectRunnableWork evaluates only authoritative readiness and chooses one
// candidate using stable priority/sequence ordering. Input order, model
// proposals, weaker relationship kinds, and ambiguous ties never authorize a
// selection.
func SelectRunnableWork(candidates []WorkCandidate, relationships []WorkRelationship) (WorkCandidate, error) {
	assessment, err := AssessWorkCandidates(candidates, relationships)
	if err != nil {
		return WorkCandidate{}, err
	}
	if assessment.Selected == nil {
		return WorkCandidate{}, ErrNoRunnableWork
	}
	return *assessment.Selected, nil
}

// AssessWorkCandidates preserves child-level blocking information while
// deriving the aggregate state. A blocked child therefore cannot make a ready
// sibling appear globally blocked, and an empty/ambiguous assessment fails
// closed instead of inventing a selection.
func AssessWorkCandidates(candidates []WorkCandidate, relationships []WorkRelationship) (WorkSetAssessment, error) {
	if len(candidates) == 0 {
		return WorkSetAssessment{}, ErrNoRunnableWork
	}
	completed := make(map[string]bool, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return WorkSetAssessment{}, err
		}
		if seen[candidate.ID] {
			return WorkSetAssessment{}, fmt.Errorf("%w: duplicate candidate %q", ErrAmbiguousWorkChoice, candidate.ID)
		}
		seen[candidate.ID] = true
		completed[candidate.ID] = candidate.Completed
	}
	assessments := make([]WorkCandidateAssessment, 0, len(candidates))
	ready := make([]WorkCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Completed {
			assessments = append(assessments, WorkCandidateAssessment{Candidate: candidate, Readiness: WorkReady})
			continue
		}
		state, blockedBy, err := EvaluateWorkReadiness(candidate.ID, relationships, completed)
		if err != nil {
			return WorkSetAssessment{}, err
		}
		assessments = append(assessments, WorkCandidateAssessment{Candidate: candidate, Readiness: state, BlockedBy: blockedBy})
		if state == WorkReady {
			ready = append(ready, candidate)
		}
	}
	assessment := WorkSetAssessment{State: WorkSetBlocked, Candidates: assessments}
	allCompleted := true
	for _, candidate := range candidates {
		if !candidate.Completed {
			allCompleted = false
			break
		}
	}
	if allCompleted {
		assessment.State = WorkSetComplete
	}
	if len(ready) == 0 {
		return assessment, nil
	}
	assessment.State = WorkSetRunnable
	sort.SliceStable(ready, func(i, j int) bool {
		if ready[i].Priority != ready[j].Priority {
			return ready[i].Priority < ready[j].Priority
		}
		if ready[i].Sequence != ready[j].Sequence {
			return ready[i].Sequence < ready[j].Sequence
		}
		return ready[i].ID < ready[j].ID
	})
	if len(ready) > 1 && ready[0].Priority == ready[1].Priority && ready[0].Sequence == ready[1].Sequence {
		return WorkSetAssessment{}, fmt.Errorf("%w: %q and %q share priority and sequence", ErrAmbiguousWorkChoice, ready[0].ID, ready[1].ID)
	}
	selected := ready[0]
	assessment.Selected = &selected
	return assessment, nil
}

// specificationProvenanceMatches is exact for proposal-time records. For an
// accepted plan it additionally admits precisely the two provenance rewrites
// that acceptance performs, and nothing else.
func specificationProvenanceMatches(specified, executable RelationshipProvenance, accepted bool) bool {
	if specified == executable {
		return true
	}
	if !accepted {
		return false
	}
	return (specified == ProvenanceModelProposal && executable == ProvenancePLAN) ||
		(specified == ProvenanceModelGateProposal && executable == ProvenanceAuthorityGate)
}
