// Package architecturereview evaluates ownership evidence without making an
// architectural or execution-authority decision on behalf of its caller.
package architecturereview

import (
	"errors"
	"fmt"
)

type ResultKind string

const (
	UniversalMechanism ResultKind = "universal_mechanism"
	DomainSpecific     ResultKind = "domain_specific"
	ReviewRequired     ResultKind = "review_required"
)

type EvidenceRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Digest string `json:"digest"`
}

type Request struct {
	Capability                     string
	ProposedOwner                  string
	Scope                          string
	ReusableAcrossScopes           bool
	DomainSpecific                 bool
	GoalEvidence                   []EvidenceRef
	InvariantEvidence              []EvidenceRef
	ImplementationLocationEvidence []EvidenceRef
	MechanismEvidence              []EvidenceRef
	PolicyEvidence                 []EvidenceRef
	CounterexampleEvidence         []EvidenceRef
}

type Result struct {
	Kind         ResultKind    `json:"kind"`
	Reasons      []string      `json:"reasons"`
	Evidence     []EvidenceRef `json:"evidence"`
	AdvisoryOnly bool          `json:"advisory_only"`
}

var (
	ErrMissingCapability = errors.New("architecture review capability and proposed owner are required")
	ErrMissingEvidence   = errors.New("architecture review requires goal and invariant evidence")
	ErrInvalidEvidence   = errors.New("architecture review evidence must have unique id, kind, and digest")
	ErrUnknownClaim      = errors.New("architecture review claim is neither reusable nor domain-specific")
	ErrAmbiguousClaim    = errors.New("architecture review claim cannot be both reusable and domain-specific")
)

func Review(req Request) (Result, error) {
	if req.Capability == "" || req.ProposedOwner == "" {
		return Result{}, ErrMissingCapability
	}
	all := appendEvidence(req.GoalEvidence, req.InvariantEvidence, req.ImplementationLocationEvidence, req.MechanismEvidence, req.PolicyEvidence, req.CounterexampleEvidence)
	if err := validateEvidence(all); err != nil {
		return Result{}, err
	}
	if len(req.GoalEvidence) == 0 || len(req.InvariantEvidence) == 0 {
		return Result{}, ErrMissingEvidence
	}
	if req.ReusableAcrossScopes && req.DomainSpecific {
		return Result{}, ErrAmbiguousClaim
	}
	if !req.ReusableAcrossScopes && !req.DomainSpecific {
		return Result{}, ErrUnknownClaim
	}

	result := Result{AdvisoryOnly: true, Evidence: all}
	if req.ReusableAcrossScopes {
		if len(req.MechanismEvidence) == 0 || len(req.PolicyEvidence) == 0 {
			result.Kind = ReviewRequired
			result.Reasons = append(result.Reasons, "reusable claim does not separate mechanism evidence from policy evidence")
			return result, nil
		}
		result.Kind = UniversalMechanism
		result.Reasons = append(result.Reasons, "scope-reusable mechanism has separately represented policy evidence")
		return result, nil
	}

	if req.DomainSpecific {
		if req.Scope == "" || len(req.CounterexampleEvidence) == 0 {
			result.Kind = ReviewRequired
			result.Reasons = append(result.Reasons, "domain-specific claim lacks scope or counterexample evidence")
			return result, nil
		}
		result.Kind = DomainSpecific
		result.Reasons = append(result.Reasons, fmt.Sprintf("behavior is bounded to declared scope %q and supported by counterexample evidence", req.Scope))
		return result, nil
	}
	return Result{}, ErrUnknownClaim
}

func appendEvidence(groups ...[]EvidenceRef) []EvidenceRef {
	var all []EvidenceRef
	for _, group := range groups {
		all = append(all, group...)
	}
	return all
}

func validateEvidence(refs []EvidenceRef) error {
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if ref.ID == "" || ref.Kind == "" || ref.Digest == "" {
			return ErrInvalidEvidence
		}
		if _, ok := seen[ref.ID]; ok {
			return fmt.Errorf("%w: duplicate %q", ErrInvalidEvidence, ref.ID)
		}
		seen[ref.ID] = struct{}{}
	}
	return nil
}
