package contracts

import (
	"errors"
	"fmt"
)

// WorkPlan is the accepted executable decomposition of a durable Goal
// baseline. A model may propose candidates, but only a plan persisted with an
// authority reference can be materialized for controller selection.
type WorkPlan struct {
	AuthorityRef    string             `json:"authority_ref"`
	AuthorityDigest string             `json:"authority_digest"`
	Candidates      []WorkCandidate    `json:"candidates,omitempty"`
	Relationships   []WorkRelationship `json:"relationships,omitempty"`
}

var ErrUnacceptedWorkPlan = errors.New("work plan is not an accepted authoritative decomposition")

func (p WorkPlan) Validate() error {
	if p.AuthorityRef == "" || p.AuthorityDigest == "" {
		return fmt.Errorf("%w: authority reference and digest are required", ErrUnacceptedWorkPlan)
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

// MaterializeWorkPlan returns only an accepted, validated decomposition. It
// deliberately has no path for model-proposed work to become runnable.
func MaterializeWorkPlan(plan WorkPlan) ([]WorkCandidate, []WorkRelationship, error) {
	if err := plan.Validate(); err != nil {
		return nil, nil, err
	}
	return append([]WorkCandidate(nil), plan.Candidates...), append([]WorkRelationship(nil), plan.Relationships...), nil
}
