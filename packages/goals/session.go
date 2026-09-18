package goals

import (
	"encoding/json"
	"errors"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

type SessionStage string

const (
	StageIntent                     SessionStage = "intent"
	StageFrame                      SessionStage = "frame"
	StageDiscover                   SessionStage = "discover"
	StageVariance                   SessionStage = "variance"
	StageCalibrate                  SessionStage = "calibrate"
	StageDecide                     SessionStage = "decide"
	StageModel                      SessionStage = "model"
	StageSpecify                    SessionStage = "specify"
	StagePlan                       SessionStage = "plan"
	StageCandidateBaseline          SessionStage = "candidate_baseline"
	StageArchitectureReview         SessionStage = "architecture_review"
	StageArchitectureReviewDecision SessionStage = "architecture_review_decision"
	StageBaseline                   SessionStage = "baseline"
	StageComplete                   SessionStage = "complete"
	StageFailed                     SessionStage = "failed"
)

var ErrInvalidSessionTransition = errors.New("invalid Goals session transition")
var ErrSessionComplete = errors.New("Goals session is already complete")

// Session retains the candidate and its review receipt as separate durable
// state. The receipt is evidence, not authority.
type Session struct {
	ID            string                  `json:"id"`
	Stage         SessionStage            `json:"stage"`
	Outcome       string                  `json:"outcome,omitempty"`
	StageOutcomes map[SessionStage]string `json:"stage_outcomes,omitempty"`
	Baseline      GoalBaseline            `json:"baseline"`
	ReviewReceipt BaselineReviewReceipt   `json:"review_receipt,omitempty"`
}

func NewSession(id, originalIntent string) (Session, error) {
	if id == "" || originalIntent == "" {
		return Session{}, errors.New("Goals session id and original intent are required")
	}
	return Session{ID: id, Stage: StageIntent, StageOutcomes: map[SessionStage]string{}, Baseline: GoalBaseline{ID: id, Version: "1", OriginalIntent: originalIntent}}, nil
}

func (s *Session) Advance(outcome string) error {
	if s == nil || s.ID == "" || outcome == "" {
		return ErrInvalidSessionTransition
	}
	if s.Stage == StageComplete {
		return ErrSessionComplete
	}
	if outcome == "blocked" {
		s.recordOutcome(outcome)
		s.Stage = StageFailed
		return nil
	}
	if !validOutcome(s.Stage, outcome) {
		return ErrInvalidSessionTransition
	}

	switch s.Stage {
	case StageCandidateBaseline:
		if err := s.digestCandidate(); err != nil {
			return err
		}
	case StageArchitectureReview:
		if err := VerifyBaselineReviewReceipt(s.Baseline, s.ReviewReceipt, outcome == "ready"); err != nil {
			return err
		}
		if outcome == "ready" && s.ReviewReceipt.Result.Kind == architecturereview.ReviewRequired {
			return ErrInvalidSessionTransition
		}
		if outcome == "review_required" && (s.ReviewReceipt.Result.Kind != architecturereview.ReviewRequired || s.ReviewReceipt.ResolutionEvidence != "") {
			return ErrInvalidSessionTransition
		}
	case StageArchitectureReviewDecision:
		if err := VerifyBaselineReviewReceipt(s.Baseline, s.ReviewReceipt, true); err != nil {
			return err
		}
	case StageBaseline:
		if err := s.verifyCompletion(); err != nil {
			return err
		}
	}

	s.recordOutcome(outcome)
	if s.Stage == StageFrame {
		s.Outcome = outcome
	}
	var next SessionStage
	switch s.Stage {
	case StageIntent:
		next = StageFrame
	case StageFrame:
		if outcome == string(RigorDirect) {
			next = StageSpecify
		} else {
			next = StageDiscover
		}
	case StageDiscover:
		next = StageVariance
	case StageVariance:
		if outcome == "calibrate" {
			next = StageCalibrate
		} else {
			next = StageDecide
		}
	case StageCalibrate:
		next = StageDecide
	case StageDecide:
		if outcome == "needs_human" {
			next = StageCalibrate
		} else {
			next = StageModel
		}
	case StageModel:
		next = StageSpecify
	case StageSpecify:
		next = StagePlan
	case StagePlan:
		next = StageCandidateBaseline
	case StageCandidateBaseline:
		next = StageArchitectureReview
	case StageArchitectureReview:
		if outcome == "review_required" {
			next = StageArchitectureReviewDecision
		} else {
			next = StageBaseline
		}
	case StageArchitectureReviewDecision:
		next = StageBaseline
	case StageBaseline:
		next = StageComplete
	default:
		return ErrInvalidSessionTransition
	}
	s.Stage = next
	return nil
}

func (s *Session) recordOutcome(outcome string) {
	if s.StageOutcomes == nil {
		s.StageOutcomes = map[SessionStage]string{}
	}
	s.StageOutcomes[s.Stage] = outcome
}

func (s *Session) digestCandidate() error {
	if err := s.Baseline.Validate(); err != nil {
		return err
	}
	digest, err := s.Baseline.ComputeDigest()
	if err != nil {
		return err
	}
	if s.Baseline.Digest != "" && s.Baseline.Digest != digest {
		return ErrBaselineDigestMismatch
	}
	s.Baseline.Digest = digest
	return nil
}

func (s *Session) verifyCompletion() error {
	if err := verifyCandidateBaseline(s.Baseline); err != nil {
		return err
	}
	return VerifyBaselineReviewReceipt(s.Baseline, s.ReviewReceipt, true)
}

func validOutcome(stage SessionStage, outcome string) bool {
	allowed := map[SessionStage]map[string]bool{
		StageIntent:                     {"captured": true},
		StageFrame:                      {string(RigorDirect): true, string(RigorStructured): true, string(RigorRigorous): true},
		StageDiscover:                   {"ready": true},
		StageVariance:                   {"calibrate": true, "decide": true},
		StageCalibrate:                  {"review_all": true, "delegate_clear": true},
		StageDecide:                     {"ready": true, "needs_human": true},
		StageModel:                      {"ready": true, "skip": true},
		StageSpecify:                    {"ready": true},
		StagePlan:                       {"ready": true, "no_execution": true},
		StageCandidateBaseline:          {"digested": true},
		StageArchitectureReview:         {"ready": true, "review_required": true},
		StageArchitectureReviewDecision: {"resolved": true},
		StageBaseline:                   {"stored": true, "ephemeral": true},
	}
	return allowed[stage][outcome]
}

func (s Session) Snapshot() ([]byte, error) {
	if s.ID == "" || s.Stage == "" || s.Baseline.ID != s.ID {
		return nil, ErrInvalidSessionTransition
	}
	if err := s.validateDurableStage(); err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

func RestoreSession(data []byte) (Session, error) {
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, err
	}
	if s.ID == "" || s.Baseline.ID != s.ID {
		return Session{}, ErrInvalidSessionTransition
	}
	if s.StageOutcomes == nil {
		s.StageOutcomes = map[SessionStage]string{}
	}
	switch s.Stage {
	case StageIntent, StageFrame, StageDiscover, StageVariance, StageCalibrate, StageDecide, StageModel, StageSpecify, StagePlan, StageCandidateBaseline, StageArchitectureReview, StageArchitectureReviewDecision, StageBaseline, StageComplete, StageFailed:
	default:
		return Session{}, ErrInvalidSessionTransition
	}
	if err := s.validateDurableStage(); err != nil {
		return Session{}, err
	}
	return s, nil
}

func (s Session) validateDurableStage() error {
	switch s.Stage {
	case StageArchitectureReview:
		if err := verifyCandidateBaseline(s.Baseline); err != nil {
			return err
		}
		if s.ReviewReceipt.ID != "" {
			return VerifyBaselineReviewReceipt(s.Baseline, s.ReviewReceipt, false)
		}
	case StageArchitectureReviewDecision:
		if err := VerifyBaselineReviewReceipt(s.Baseline, s.ReviewReceipt, false); err != nil {
			return err
		}
		if s.ReviewReceipt.Result.Kind != architecturereview.ReviewRequired {
			return ErrInvalidSessionTransition
		}
	case StageBaseline, StageComplete:
		return s.verifyCompletion()
	}
	return nil
}

// SetBaseline attaches the complete candidate after Plan and before its digest
// is frozen. No post-review candidate replacement is accepted.
func (s *Session) SetBaseline(b GoalBaseline) error {
	if s == nil || s.Stage != StageCandidateBaseline || b.ID != s.ID || b.OriginalIntent != s.Baseline.OriginalIntent {
		return ErrInvalidSessionTransition
	}
	if err := b.Validate(); err != nil {
		return err
	}
	if b.Digest != "" {
		if err := b.VerifyDigest(); err != nil {
			return err
		}
	}
	s.Baseline = b
	return nil
}

// ReviewArchitecture runs the review only at the post-digest review stage.
func (s *Session) ReviewArchitecture(req architecturereview.Request) error {
	if s == nil || s.Stage != StageArchitectureReview {
		return ErrInvalidSessionTransition
	}
	receipt, err := ReviewBaselineOwnership(s.Baseline, req)
	if err != nil {
		return err
	}
	s.ReviewReceipt = receipt
	return nil
}

func (s *Session) SetArchitectureReview(receipt BaselineReviewReceipt) error {
	if s == nil || s.Stage != StageArchitectureReview {
		return ErrInvalidSessionTransition
	}
	if err := VerifyBaselineReviewReceipt(s.Baseline, receipt, false); err != nil {
		return err
	}
	s.ReviewReceipt = receipt
	return nil
}

func (s *Session) ResolveArchitectureReview(evidence string) error {
	if s == nil || s.Stage != StageArchitectureReviewDecision {
		return ErrInvalidSessionTransition
	}
	receipt, err := ResolveBaselineReview(s.Baseline, s.ReviewReceipt, evidence)
	if err != nil {
		return err
	}
	s.ReviewReceipt = receipt
	return nil
}
