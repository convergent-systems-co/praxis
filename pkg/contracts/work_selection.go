package contracts

import (
	"errors"
	"fmt"
	"sort"
)

// WorkCandidate is an authoritative durable work record eligible for
// selection. Lower Priority and Sequence values win; equal values are
// ambiguous and fail closed rather than depending on input order.
type WorkCandidate struct {
	ID           string                 `json:"id"`
	Completed    bool                   `json:"completed"`
	Priority     int                    `json:"priority"`
	Sequence     int                    `json:"sequence"`
	SourceRef    string                 `json:"source_ref"`
	SourceDigest string                 `json:"source_digest"`
	Provenance   RelationshipProvenance `json:"provenance"`
}

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
	case ProvenanceADR, ProvenanceSPEC, ProvenancePLAN, ProvenanceContract, ProvenanceIssue:
		return nil
	case ProvenanceModelProposal:
		return ErrInferredWorkSelection
	default:
		return fmt.Errorf("%w: unknown candidate provenance %q", ErrInvalidWorkRelationship, c.Provenance)
	}
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
