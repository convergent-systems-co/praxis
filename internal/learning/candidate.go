package learning

import (
	"errors"
	"fmt"
)

type CandidateState string

const (
	CandidateProposed     CandidateState = "proposed"
	CandidateEvaluating   CandidateState = "evaluating"
	CandidateRejected     CandidateState = "rejected"
	CandidateExperimental CandidateState = "experimental"
	CandidatePromoted     CandidateState = "promoted"
	CandidateStabilized   CandidateState = "stabilized"
	CandidateDemoted      CandidateState = "demoted"
	CandidateRevoked      CandidateState = "revoked"
)

type Candidate struct {
	ID                 string
	Target             string
	SourceObservations []string
	ExpectedBenefit    string
	RiskClass          string
	RequiredEvaluation string
	RollbackRef        string
	State              CandidateState
}

func (c Candidate) Validate() error {
	if c.ID == "" || c.Target == "" || len(c.SourceObservations) == 0 || c.RequiredEvaluation == "" || c.RollbackRef == "" {
		return errors.New("candidate id, target, observations, evaluation, and rollback are required")
	}
	return nil
}

func CanTransition(from, to CandidateState) bool {
	if from == CandidateRevoked {
		return false
	}
	allowed := map[CandidateState]map[CandidateState]bool{
		CandidateProposed:     {CandidateEvaluating: true, CandidateRejected: true, CandidateRevoked: true},
		CandidateEvaluating:   {CandidateRejected: true, CandidateExperimental: true, CandidatePromoted: true, CandidateRevoked: true},
		CandidateExperimental: {CandidateEvaluating: true, CandidatePromoted: true, CandidateRejected: true, CandidateRevoked: true},
		CandidatePromoted:     {CandidateStabilized: true, CandidateDemoted: true, CandidateRevoked: true},
		CandidateStabilized:   {CandidateDemoted: true, CandidateRevoked: true},
		CandidateDemoted:      {CandidateEvaluating: true, CandidateRevoked: true},
		CandidateRejected:     {CandidateEvaluating: true, CandidateRevoked: true},
	}
	return allowed[from][to]
}

func ValidateTransition(from, to CandidateState) error {
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid learning candidate transition %q -> %q", from, to)
	}
	return nil
}
