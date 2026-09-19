package goaldrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// CompletionTrailer is the Git commit trailer through which a worker
// proposes that the selected WorkPlan unit is complete. It is a proposal
// only: the controller validates the checkpoint, evaluates the completion
// predicates, and records completion durably (#158).
const CompletionTrailer = "Praxis-Unit-Complete"

const unitCompletedEventType = "goal_drive.unit_completed"

// UnitCompletion is the durable, controller-owned record that a WorkPlan
// unit of one Goal generation is complete. Eligibility of dependent units
// derives from these records, never from worker assertion or from the
// immutable plan's proposal-time metadata alone.
type UnitCompletion struct {
	GoalID       string    `json:"goal_id"`
	GoalVersion  string    `json:"goal_version"`
	UnitID       string    `json:"unit_id"`
	InvocationID string    `json:"invocation_id"`
	TurnID       string    `json:"turn_id"`
	EndHead      string    `json:"end_head"`
	Requirements []string  `json:"requirements,omitempty"`
	Evidence     []string  `json:"evidence"`
	CompletedAt  time.Time `json:"completed_at"`
}

func (c UnitCompletion) validate() error {
	if c.GoalID == "" || c.GoalVersion == "" || c.UnitID == "" || c.InvocationID == "" || c.TurnID == "" || c.EndHead == "" {
		return errors.New("unit completion requires Goal generation, unit, turn, and checkpoint identity")
	}
	if len(c.Evidence) == 0 {
		return errors.New("unit completion requires evidence")
	}
	return nil
}

// CompletionClaimRepository reports the unit completion proposals a worker
// left in the commits between two checkpoints.
type CompletionClaimRepository interface {
	CompletionClaims(ctx context.Context, startHead, endHead string) ([]string, error)
}

// ParseCompletionTrailers extracts the unit identities named by
// Praxis-Unit-Complete trailers from trailer-only git log output.
func ParseCompletionTrailers(output string) []string {
	seen := map[string]struct{}{}
	var units []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, dup := seen[line]; dup {
			continue
		}
		seen[line] = struct{}{}
		units = append(units, line)
	}
	return units
}

func completionAggregate(goalID, version string) string {
	return "goal-drive-completion:" + goalID + ":" + version
}

// LoadCompletions returns the durable unit completions of a Goal generation
// in the order they were recorded.
func (l Ledger) LoadCompletions(ctx context.Context, goalID, goalVersion string) ([]UnitCompletion, error) {
	if l.Store == nil {
		return nil, errors.New("Goal drive event store is required")
	}
	events, err := l.Store.LoadAggregate(ctx, completionAggregate(goalID, goalVersion), 0)
	if err != nil {
		return nil, err
	}
	out := make([]UnitCompletion, 0, len(events))
	for _, event := range events {
		if event.Type != unitCompletedEventType {
			return nil, fmt.Errorf("unexpected completion event %q", event.Type)
		}
		var completion UnitCompletion
		if err := json.Unmarshal(event.Payload, &completion); err != nil {
			return nil, fmt.Errorf("decode unit completion: %w", err)
		}
		if err := completion.validate(); err != nil {
			return nil, err
		}
		if completion.GoalID != goalID || completion.GoalVersion != goalVersion {
			return nil, errors.New("unit completion identity mismatch")
		}
		out = append(out, completion)
	}
	return out, nil
}

// RecordCompletion appends one unit completion. A unit completes at most
// once per Goal generation.
func (l Ledger) RecordCompletion(ctx context.Context, completion UnitCompletion) error {
	if l.Store == nil {
		return errors.New("Goal drive event store is required")
	}
	if err := completion.validate(); err != nil {
		return err
	}
	current, err := l.LoadCompletions(ctx, completion.GoalID, completion.GoalVersion)
	if err != nil {
		return err
	}
	for _, existing := range current {
		if existing.UnitID == completion.UnitID {
			return fmt.Errorf("unit %s is already complete (turn %s)", completion.UnitID, existing.TurnID)
		}
	}
	payload, err := json.Marshal(completion)
	if err != nil {
		return fmt.Errorf("encode unit completion: %w", err)
	}
	aggregate := completionAggregate(completion.GoalID, completion.GoalVersion)
	_, err = l.Store.Append(ctx, aggregate, int64(len(current)), []eventstore.Event{{
		ID: aggregate + ":unit:" + completion.UnitID, AggregateType: "goal_drive_completion", Type: unitCompletedEventType, Version: "1",
		Actor: l.Actor, CommandID: "goal-drive:completion:" + completion.TurnID + ":" + completion.UnitID, CorrelationID: aggregate, Trust: contracts.TrustObserved,
		Payload: payload, CreatedAt: completion.CompletedAt,
	}})
	return err
}

// ApplyCompletions overlays durable completion state on the immutable
// plan's candidates: this is the "durable completion state supplied to the
// evaluator" that SPEC-025 requires.
func ApplyCompletions(candidates []contracts.WorkCandidate, completions []UnitCompletion) []contracts.WorkCandidate {
	complete := map[string]struct{}{}
	for _, completion := range completions {
		complete[completion.UnitID] = struct{}{}
	}
	out := make([]contracts.WorkCandidate, len(candidates))
	for i, candidate := range candidates {
		out[i] = candidate
		if _, ok := complete[candidate.ID]; ok {
			out[i].Completed = true
		}
	}
	return out
}

// GoalCompletionAssessment says whether a Goal generation is complete: every
// WorkPlan unit is durably complete and every success criterion of the
// generation is covered by a requirement of a completed unit.
type GoalCompletionAssessment struct {
	AllUnitsComplete  bool     `json:"all_units_complete"`
	IncompleteUnits   []string `json:"incomplete_units,omitempty"`
	UncoveredCriteria []string `json:"uncovered_criteria,omitempty"`
	Complete          bool     `json:"complete"`
}

// AssessGoalCompletion evaluates the Goal-level completion predicate from
// the immutable generation and the durable unit completions.
func AssessGoalCompletion(baseline goals.GoalBaseline, completions []UnitCompletion) (GoalCompletionAssessment, error) {
	if baseline.WorkPlan == nil {
		return GoalCompletionAssessment{}, ErrNoAcceptedGoalWorkPlan
	}
	candidates := ApplyCompletions(baseline.WorkPlan.Candidates, completions)
	assessment := GoalCompletionAssessment{AllUnitsComplete: true}
	covered := map[int]struct{}{}
	for _, candidate := range candidates {
		if !candidate.Completed {
			assessment.AllUnitsComplete = false
			assessment.IncompleteUnits = append(assessment.IncompleteUnits, candidate.ID)
			continue
		}
		for _, requirement := range candidate.Requirements {
			if index, ok := successCriterionIndex(requirement.SourceRef); ok {
				covered[index] = struct{}{}
			}
		}
	}
	for i := range baseline.SuccessCriteria {
		if _, ok := covered[i+1]; !ok {
			assessment.UncoveredCriteria = append(assessment.UncoveredCriteria, "success_criteria/"+strconv.Itoa(i+1))
		}
	}
	sort.Strings(assessment.UncoveredCriteria)
	assessment.Complete = assessment.AllUnitsComplete && len(assessment.UncoveredCriteria) == 0
	return assessment, nil
}

// successCriterionIndex reads the 1-based criterion index from a requirement
// source reference of the form <goal>/<version>#success_criteria/<n>.
func successCriterionIndex(sourceRef string) (int, bool) {
	const marker = "#success_criteria/"
	at := strings.LastIndex(sourceRef, marker)
	if at < 0 {
		return 0, false
	}
	index, err := strconv.Atoi(sourceRef[at+len(marker):])
	if err != nil || index < 1 {
		return 0, false
	}
	return index, true
}

// GoalCompletionClaim is the controller's provisional evidence that a Goal
// generation may be complete: every WorkPlan unit is durably complete and
// every success criterion is covered. It is not authoritative. The WorkPlan
// itself may have been incomplete, so the installation owner re-evaluates
// the original Goal contract before Goal completion is recorded (#158).
type GoalCompletionClaim struct {
	GoalID       string                   `json:"goal_id"`
	GoalVersion  string                   `json:"goal_version"`
	GoalDigest   string                   `json:"goal_digest"`
	InvocationID string                   `json:"invocation_id"`
	TurnID       string                   `json:"turn_id"`
	FinalHead    string                   `json:"final_head"`
	Units        []UnitCompletion         `json:"units"`
	Assessment   GoalCompletionAssessment `json:"assessment"`
	ClaimedAt    time.Time                `json:"claimed_at"`
}

// GoalCompletionStatus is the owner's authoritative re-evaluation outcome.
type GoalCompletionStatus string

const (
	GoalComplete   GoalCompletionStatus = "complete"
	GoalIncomplete GoalCompletionStatus = "incomplete"
)

// GoalCompletionDecision is the owner's authoritative evaluation of a claim
// against the original Goal contract.
type GoalCompletionDecision struct {
	GoalID      string                 `json:"goal_id"`
	GoalVersion string                 `json:"goal_version"`
	GoalDigest  string                 `json:"goal_digest"`
	ClaimTurnID string                 `json:"claim_turn_id"`
	FinalHead   string                 `json:"final_head"`
	Status      GoalCompletionStatus   `json:"status"`
	DecidedBy   contracts.PrincipalRef `json:"decided_by"`
	Reason      string                 `json:"reason,omitempty"`
	DecidedAt   time.Time              `json:"decided_at"`
}

// GoalCompletionState is the durable Goal-level completion state of a
// generation: the provisional claim, if any, and the owner's decision.
type GoalCompletionState struct {
	Claim    *GoalCompletionClaim    `json:"claim,omitempty"`
	Decision *GoalCompletionDecision `json:"decision,omitempty"`
}

const (
	goalCompletionClaimedEventType = "goal_drive.goal_completion_claimed"
	goalCompletionDecidedEventType = "goal_drive.goal_completion_decided"
)

func goalCompletionAggregate(goalID, version string) string {
	return "goal-drive-goal-completion:" + goalID + ":" + version
}

// LoadGoalCompletion returns the generation's provisional claim and the
// owner's decision, when they exist.
func (l Ledger) LoadGoalCompletion(ctx context.Context, goalID, goalVersion string) (GoalCompletionState, error) {
	if l.Store == nil {
		return GoalCompletionState{}, errors.New("Goal drive event store is required")
	}
	events, err := l.Store.LoadAggregate(ctx, goalCompletionAggregate(goalID, goalVersion), 0)
	if err != nil {
		return GoalCompletionState{}, err
	}
	var state GoalCompletionState
	for _, event := range events {
		switch event.Type {
		case goalCompletionClaimedEventType:
			var claim GoalCompletionClaim
			if err := json.Unmarshal(event.Payload, &claim); err != nil {
				return GoalCompletionState{}, fmt.Errorf("decode Goal completion claim: %w", err)
			}
			state.Claim = &claim
		case goalCompletionDecidedEventType:
			var decision GoalCompletionDecision
			if err := json.Unmarshal(event.Payload, &decision); err != nil {
				return GoalCompletionState{}, fmt.Errorf("decode Goal completion decision: %w", err)
			}
			state.Decision = &decision
		default:
			return GoalCompletionState{}, fmt.Errorf("unexpected Goal completion event %q", event.Type)
		}
	}
	return state, nil
}

// RecordGoalCompletionClaim persists the provisional claim once.
func (l Ledger) RecordGoalCompletionClaim(ctx context.Context, claim GoalCompletionClaim) error {
	if claim.GoalID == "" || claim.GoalVersion == "" || claim.TurnID == "" || claim.FinalHead == "" || !claim.Assessment.Complete {
		return errors.New("Goal completion claim requires the generation, the claiming turn, the final checkpoint, and a complete assessment")
	}
	state, err := l.LoadGoalCompletion(ctx, claim.GoalID, claim.GoalVersion)
	if err != nil {
		return err
	}
	if state.Claim != nil {
		return fmt.Errorf("Goal %s/%s completion was already claimed by turn %s", claim.GoalID, claim.GoalVersion, state.Claim.TurnID)
	}
	return l.appendGoalCompletion(ctx, claim.GoalID, claim.GoalVersion, 0, goalCompletionClaimedEventType, "goal-drive:goal-completion-claim:"+claim.TurnID, claim, claim.ClaimedAt)
}

// RecordGoalCompletionDecision persists the owner's evaluation of the
// claim. It requires a claim, binds the decision to the claim's turn and
// final checkpoint, and admits exactly one decision.
func (l Ledger) RecordGoalCompletionDecision(ctx context.Context, decision GoalCompletionDecision) error {
	if decision.Status != GoalComplete && decision.Status != GoalIncomplete {
		return fmt.Errorf("Goal completion status must be %q or %q", GoalComplete, GoalIncomplete)
	}
	if err := decision.DecidedBy.Validate(); err != nil {
		return fmt.Errorf("Goal completion decider: %w", err)
	}
	state, err := l.LoadGoalCompletion(ctx, decision.GoalID, decision.GoalVersion)
	if err != nil {
		return err
	}
	if state.Claim == nil {
		return fmt.Errorf("Goal %s/%s has no provisional completion claim to evaluate", decision.GoalID, decision.GoalVersion)
	}
	if state.Decision != nil {
		return fmt.Errorf("Goal %s/%s completion was already decided (%s) by %s", decision.GoalID, decision.GoalVersion, state.Decision.Status, state.Decision.DecidedBy.ID)
	}
	if decision.ClaimTurnID != state.Claim.TurnID || decision.FinalHead != state.Claim.FinalHead || decision.GoalDigest != state.Claim.GoalDigest {
		return errors.New("Goal completion decision must bind the exact claim turn, final checkpoint, and generation digest")
	}
	return l.appendGoalCompletion(ctx, decision.GoalID, decision.GoalVersion, 1, goalCompletionDecidedEventType, "goal-drive:goal-completion-decision:"+decision.ClaimTurnID, decision, decision.DecidedAt)
}

func (l Ledger) appendGoalCompletion(ctx context.Context, goalID, goalVersion string, expected int64, eventType, commandID string, payload any, at time.Time) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	aggregate := goalCompletionAggregate(goalID, goalVersion)
	_, err = l.Store.Append(ctx, aggregate, expected, []eventstore.Event{{
		ID: aggregate + ":" + eventType, AggregateType: "goal_drive_goal_completion", Type: eventType, Version: "1",
		Actor: l.Actor, CommandID: commandID, CorrelationID: aggregate, Trust: contracts.TrustObserved,
		Payload: encoded, CreatedAt: at,
	}})
	return err
}
