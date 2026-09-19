package goaldrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	aggregateType = "goal_drive"
	eventType     = "goal_drive.turn_recorded"
	eventVersion  = "1"
)

type ExecutionMode string

const (
	ModeSupervised ExecutionMode = "supervised"
	ModeContinuous ExecutionMode = "continuous"
)

type Outcome string

const (
	OutcomeContinue             Outcome = "CONTINUE"
	OutcomeComplete             Outcome = "COMPLETE"
	OutcomeBlocked              Outcome = "BLOCKED"
	OutcomeNoProgress           Outcome = "NO_PROGRESS"
	OutcomeUserDecisionRequired Outcome = "USER_DECISION_REQUIRED"
)

// TurnRecord is the durable, controller-owned result of one bounded worker
// turn. Worker prose is not stored as authority; these fields are verified
// control-plane facts and references to independently stored evidence.
type TurnRecord struct {
	GoalID              string        `json:"goal_id"`
	GoalVersion         string        `json:"goal_version"`
	InvocationID        string        `json:"invocation_id"`
	Mode                ExecutionMode `json:"mode"`
	TurnID              string        `json:"turn_id"`
	ChildObjective      string        `json:"child_objective"`
	GraphID             string        `json:"graph_id"`
	GraphVersion        string        `json:"graph_version"`
	AgentID             string        `json:"agent_id,omitempty"`
	ExecutorID          string        `json:"executor_id,omitempty"`
	StartHead           string        `json:"start_head,omitempty"`
	EndHead             string        `json:"end_head,omitempty"`
	Outcome             Outcome       `json:"outcome"`
	Progress            bool          `json:"progress"`
	CheckpointPublished bool          `json:"checkpoint_published"`
	CheckpointEvidence  []string      `json:"checkpoint_evidence,omitempty"`
	RetryOf             string        `json:"retry_of,omitempty"`
	Blocker             string        `json:"blocker,omitempty"`
	// Consequence* bind what a BLOCKED turn left in the checkout (uncommitted
	// paths, unpublished commits) so a later --recover-turn can admit exactly
	// that state and nothing else.
	ConsequenceFingerprint string   `json:"consequence_fingerprint,omitempty"`
	ConsequenceFiles       []string `json:"consequence_files,omitempty"`
	ConsequenceCommits     []string `json:"consequence_commits,omitempty"`
	// CompletionClaim is the unit the worker proposed complete (commit
	// trailer); UnitCompleted says the controller accepted the proposal and
	// recorded the unit's completion durably (#158).
	CompletionClaim string `json:"completion_claim,omitempty"`
	UnitCompleted   bool   `json:"unit_completed,omitempty"`
	// GoalCandidate says this turn completed the last unit and recorded the
	// GOAL_COMPLETION_CANDIDATE; GoalEvaluation is the deterministic
	// verifier's outcome (satisfied|unsatisfied|unknown), evidence only.
	GoalCandidate  bool      `json:"goal_candidate,omitempty"`
	GoalEvaluation string    `json:"goal_evaluation,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// MarshalJSON renders an unknown (zero) CreatedAt as absent rather than as
// the year-0001 instant, so a record never claims a time it does not have
// (#156).
func (r TurnRecord) MarshalJSON() ([]byte, error) {
	type plain TurnRecord
	out := struct {
		plain
		CreatedAt *time.Time `json:"created_at,omitempty"`
	}{plain: plain(r)}
	if !r.CreatedAt.IsZero() {
		created := r.CreatedAt
		out.CreatedAt = &created
	}
	return json.Marshal(out)
}

func (r TurnRecord) validate() error {
	if r.GoalID == "" || r.GoalVersion == "" || r.InvocationID == "" || r.TurnID == "" || r.ChildObjective == "" || r.GraphID == "" || r.GraphVersion == "" {
		return errors.New("Goal turn identity, objective, graph, and versions are required")
	}
	if r.Mode != ModeSupervised && r.Mode != ModeContinuous {
		return fmt.Errorf("unsupported Goal-drive execution mode %q", r.Mode)
	}
	switch r.Outcome {
	case OutcomeContinue, OutcomeComplete, OutcomeBlocked, OutcomeNoProgress, OutcomeUserDecisionRequired:
	default:
		return fmt.Errorf("unsupported Goal turn outcome %q", r.Outcome)
	}
	if r.Outcome == OutcomeNoProgress && r.Progress {
		return errors.New("NO_PROGRESS turn cannot claim progress")
	}
	if r.CheckpointPublished && !r.Progress {
		return errors.New("checkpoint publication requires validated progress")
	}
	if r.Outcome == OutcomeComplete && !r.Progress {
		return errors.New("COMPLETE turn requires progress evidence")
	}
	return nil
}

type Ledger struct {
	Store eventstore.Store
	Actor contracts.PrincipalRef
}

// Record stamps the turn's creation instant on the caller's record and
// persists exactly that record, so the value the caller goes on to return
// or render carries the same timestamp the ledger holds (#156).
func (l Ledger) Record(ctx context.Context, expectedVersion int64, record *TurnRecord) (eventstore.Event, error) {
	if record == nil {
		return eventstore.Event{}, errors.New("Goal turn record is required")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	return l.Append(ctx, expectedVersion, *record)
}

func (l Ledger) Append(ctx context.Context, expectedVersion int64, record TurnRecord) (eventstore.Event, error) {
	if l.Store == nil {
		return eventstore.Event{}, errors.New("Goal drive event store is required")
	}
	if err := l.Actor.Validate(); err != nil {
		return eventstore.Event{}, fmt.Errorf("Goal drive actor: %w", err)
	}
	if err := record.validate(); err != nil {
		return eventstore.Event{}, err
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return eventstore.Event{}, fmt.Errorf("encode Goal turn: %w", err)
	}
	aggregateID := aggregateID(record.GoalID, record.GoalVersion)
	appended, err := l.Store.Append(ctx, aggregateID, expectedVersion, []eventstore.Event{{
		ID: aggregateID + ":turn:" + record.TurnID, AggregateType: aggregateType, Type: eventType, Version: eventVersion,
		Actor: l.Actor, CommandID: "goal-drive:turn:" + record.TurnID, CorrelationID: aggregateID, Trust: contracts.TrustObserved,
		Payload: payload, CreatedAt: record.CreatedAt,
	}})
	if err != nil {
		return eventstore.Event{}, err
	}
	return appended[0], nil
}

func (l Ledger) Load(ctx context.Context, goalID, goalVersion string) ([]TurnRecord, error) {
	if l.Store == nil {
		return nil, errors.New("Goal drive event store is required")
	}
	if goalID == "" || goalVersion == "" {
		return nil, errors.New("Goal identity and version are required")
	}
	events, err := l.Store.LoadAggregate(ctx, aggregateID(goalID, goalVersion), 0)
	if err != nil {
		return nil, err
	}
	turns := make([]TurnRecord, 0, len(events))
	for _, event := range events {
		if event.Type != eventType || event.Version != eventVersion || event.Trust != contracts.TrustObserved {
			return nil, errors.New("unsupported or untrusted Goal turn event")
		}
		var turn TurnRecord
		if err := json.Unmarshal(event.Payload, &turn); err != nil {
			return nil, fmt.Errorf("decode Goal turn: %w", err)
		}
		if err := turn.validate(); err != nil || turn.GoalID != goalID || turn.GoalVersion != goalVersion {
			return nil, errors.New("Goal turn payload identity or semantics are invalid")
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func aggregateID(goalID, version string) string { return "goal-drive:" + goalID + ":" + version }
