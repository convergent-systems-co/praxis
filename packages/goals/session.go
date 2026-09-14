package goals

import (
	"encoding/json"
	"errors"
)

// SessionStage is the durable boundary between interactive Goals work and
// the resulting derived baseline. The stage is data, not authority: callers
// still need the normal policy and persistence boundaries to finalize a
// baseline.
type SessionStage string

const (
	StageIntent    SessionStage = "intent"
	StageFrame     SessionStage = "frame"
	StageDiscover  SessionStage = "discover"
	StageVariance  SessionStage = "variance"
	StageCalibrate SessionStage = "calibrate"
	StageDecide    SessionStage = "decide"
	StageModel     SessionStage = "model"
	StageSpecify   SessionStage = "specify"
	StagePlan      SessionStage = "plan"
	StageBaseline  SessionStage = "baseline"
	StageComplete  SessionStage = "complete"
	StageFailed    SessionStage = "failed"
)

var ErrInvalidSessionTransition = errors.New("invalid Goals session transition")
var ErrSessionComplete = errors.New("Goals session is already complete")

// Session is the resumable, user-facing portion of the Goals graph. Pending
// decisions and the baseline under construction are retained in the snapshot
// so an interruption cannot turn a partial conversation into authority.
type Session struct {
	ID            string                  `json:"id"`
	Stage         SessionStage            `json:"stage"`
	Outcome       string                  `json:"outcome,omitempty"`
	StageOutcomes map[SessionStage]string `json:"stage_outcomes,omitempty"`
	Baseline      GoalBaseline            `json:"baseline"`
}

func NewSession(id, originalIntent string) (Session, error) {
	if id == "" || originalIntent == "" {
		return Session{}, errors.New("Goals session id and original intent are required")
	}
	return Session{ID: id, Stage: StageIntent, StageOutcomes: map[SessionStage]string{}, Baseline: GoalBaseline{ID: id, Version: "1", OriginalIntent: originalIntent}}, nil
}

// Advance records one completed graph responsibility. The outcome is
// intentionally caller-provided evidence; this type does not infer domain
// meaning or grant execution authority.
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
	if s.Stage == StageBaseline {
		if err := s.finalizeBaseline(); err != nil {
			return err
		}
		s.recordOutcome(outcome)
		s.Stage = StageComplete
		return nil
	}
	s.recordOutcome(outcome)
	if s.Stage == StageFrame {
		s.Outcome = outcome
		if outcome == string(RigorDirect) {
			s.Stage = StageBaseline
			return nil
		}
		if outcome != string(RigorStructured) && outcome != string(RigorRigorous) {
			return ErrInvalidSessionTransition
		}
	}
	var next SessionStage
	switch s.Stage {
	case StageIntent:
		next = StageFrame
	case StageFrame:
		next = StageDiscover
	case StageDiscover:
		next = StageVariance
	case StageVariance:
		next = StageCalibrate
	case StageCalibrate:
		next = StageDecide
	case StageDecide:
		next = StageModel
	case StageModel:
		next = StageSpecify
	case StageSpecify:
		next = StagePlan
	case StagePlan:
		next = StageBaseline
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

// finalizeBaseline makes completion contingent on a usable, integrity-bound
// baseline. Persistence is intentionally owned by goalstore; this only derives
// the content digest and never grants execution authority.
func (s *Session) finalizeBaseline() error {
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

func validOutcome(stage SessionStage, outcome string) bool {
	allowed := map[SessionStage]map[string]bool{
		StageIntent:    {"captured": true},
		StageFrame:     {string(RigorDirect): true, string(RigorStructured): true, string(RigorRigorous): true},
		StageDiscover:  {"ready": true},
		StageVariance:  {"calibrate": true, "decide": true},
		StageCalibrate: {"review_all": true, "delegate_clear": true},
		StageDecide:    {"ready": true, "needs_human": true},
		StageModel:     {"ready": true, "skip": true},
		StageSpecify:   {"ready": true},
		StagePlan:      {"ready": true, "no_execution": true},
		StageBaseline:  {"stored": true, "ephemeral": true},
	}
	return allowed[stage][outcome]
}

// Snapshot returns canonical JSON suitable for an authoritative provider to
// store as a session checkpoint. Restore validates identity and stage before
// the caller publishes it as current state.
func (s Session) Snapshot() ([]byte, error) {
	if s.ID == "" || s.Stage == "" || s.Baseline.ID != s.ID {
		return nil, ErrInvalidSessionTransition
	}
	if s.Stage == StageBaseline || s.Stage == StageComplete {
		if err := s.Baseline.Validate(); err != nil {
			return nil, err
		}
		if s.Stage == StageComplete && s.Baseline.Digest == "" {
			return nil, ErrBaselineDigestMismatch
		}
		if err := s.Baseline.VerifyDigest(); err != nil {
			return nil, err
		}
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
	case StageIntent, StageFrame, StageDiscover, StageVariance, StageCalibrate, StageDecide, StageModel, StageSpecify, StagePlan, StageBaseline, StageComplete, StageFailed:
	default:
		return Session{}, ErrInvalidSessionTransition
	}
	if s.Stage == StageComplete {
		if err := s.Baseline.Validate(); err != nil {
			return Session{}, err
		}
		if s.Baseline.Digest == "" {
			return Session{}, ErrBaselineDigestMismatch
		}
		if err := s.Baseline.VerifyDigest(); err != nil {
			return Session{}, err
		}
	}
	return s, nil
}

// SetBaseline attaches the derived result without changing the original
// intent captured at session creation.
func (s *Session) SetBaseline(b GoalBaseline) error {
	if s == nil || s.Stage != StageBaseline || b.ID != s.ID || b.OriginalIntent != s.Baseline.OriginalIntent {
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
