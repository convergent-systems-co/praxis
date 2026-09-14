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
	if len(candidates) == 0 {
		return WorkCandidate{}, ErrNoRunnableWork
	}
	completed := make(map[string]bool, len(candidates))
	seen := make(map[string]bool, len(candidates))
	for _, candidate := range candidates {
		if err := candidate.Validate(); err != nil {
			return WorkCandidate{}, err
		}
		if seen[candidate.ID] {
			return WorkCandidate{}, fmt.Errorf("%w: duplicate candidate %q", ErrAmbiguousWorkChoice, candidate.ID)
		}
		seen[candidate.ID] = true
		completed[candidate.ID] = candidate.Completed
	}
	ready := make([]WorkCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Completed {
			continue
		}
		state, _, err := EvaluateWorkReadiness(candidate.ID, relationships, completed)
		if err != nil {
			return WorkCandidate{}, err
		}
		if state == WorkReady {
			ready = append(ready, candidate)
		}
	}
	if len(ready) == 0 {
		return WorkCandidate{}, ErrNoRunnableWork
	}
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
		return WorkCandidate{}, fmt.Errorf("%w: %q and %q share priority and sequence", ErrAmbiguousWorkChoice, ready[0].ID, ready[1].ID)
	}
	return ready[0], nil
}
