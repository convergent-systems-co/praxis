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
	// Materialization is set only when the completion was derived after the
	// turn ended by deterministic re-materialization (#164).
	Materialization *CompletionMaterialization `json:"materialization,omitempty"`
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

// RecoveryCompletionClaimRepository isolates worker-authored proposals from
// commits already present on the authoritative remote and from merge commits.
type RecoveryCompletionClaimRepository interface {
	RecoveryCompletionClaims(ctx context.Context, remoteHead, endHead string) ([]string, error)
}

// ErrAmbiguousCompletionClaim reports a claim line that starts with the
// completion key but does not carry exactly one unit identity. Praxis fails
// closed: it neither guesses the unit nor treats the line as prose (#164).
var ErrAmbiguousCompletionClaim = errors.New("ambiguous completion proposal")

// ParseCompletionProposals extracts the unit identities a commit message
// proposes complete. The contract is exactly what Praxis tells the worker: a
// line of the form `Praxis-Unit-Complete: <unit>` standing alone on a line
// anywhere after the subject line (the key compared case-insensitively, as
// Git compares trailer keys). Git's own trailer-block heuristic (final
// paragraph only) is not the contract: the live Weather II turn 6
// checkpoint carried the claim one paragraph above the Co-Authored-By
// trailer and was invisible to `%(trailers:key=...)` (#164).
//
// A line that starts with the key but is not exactly `key: <one token>` is
// ambiguous and returns ErrAmbiguousCompletionClaim. The key appearing
// inside prose (not at the start of a line) is not a proposal. The subject
// line is a title, never a proposal. Duplicate identical claims collapse
// to one; distinct units are all returned in message order so the caller
// can refuse a conflicting set.
func ParseCompletionProposals(message string) ([]string, error) {
	lines := strings.Split(message, "\n")
	seen := map[string]struct{}{}
	var units []string
	for i, raw := range lines {
		line := strings.TrimSpace(raw)
		if i == 0 || line == "" {
			continue
		}
		key, rest, found := strings.Cut(line, ":")
		if !found || !strings.EqualFold(strings.TrimSpace(key), CompletionTrailer) {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) != 1 {
			return nil, fmt.Errorf("%w: %q is not exactly `%s: <unit>`", ErrAmbiguousCompletionClaim, line, CompletionTrailer)
		}
		unit := fields[0]
		if _, dup := seen[unit]; dup {
			continue
		}
		seen[unit] = struct{}{}
		units = append(units, unit)
	}
	return units, nil
}

// ParseCompletionClaims applies ParseCompletionProposals to every commit
// message of a turn span, in log order, collapsing duplicates across commits.
func ParseCompletionClaims(messages []string) ([]string, error) {
	seen := map[string]struct{}{}
	var units []string
	for _, message := range messages {
		proposals, err := ParseCompletionProposals(message)
		if err != nil {
			return nil, err
		}
		for _, unit := range proposals {
			if _, dup := seen[unit]; dup {
				continue
			}
			seen[unit] = struct{}{}
			units = append(units, unit)
		}
	}
	return units, nil
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
		Actor: l.Actor, CommandID: aggregate + ":" + completion.TurnID + ":" + completion.UnitID, CorrelationID: aggregate, Trust: contracts.TrustObserved,
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

// GoalCompletionAssessment is STRUCTURAL: every WorkPlan unit is durably
// complete, and each success criterion is or is not referenced by a
// requirement of a completed unit. Coverage is evidence that the accepted
// decomposition addressed a criterion; it is never criterion satisfaction
// (#160). Satisfaction is established only by a GoalCompletionEvaluation.
type GoalCompletionAssessment struct {
	AllUnitsComplete  bool                `json:"all_units_complete"`
	IncompleteUnits   []string            `json:"incomplete_units,omitempty"`
	Coverage          []CriterionCoverage `json:"coverage,omitempty"`
	UncoveredCriteria []string            `json:"uncovered_criteria,omitempty"`
	// StructurallyComplete means all units complete and every criterion
	// covered. It is a candidate condition, not Goal completion.
	StructurallyComplete bool `json:"structurally_complete"`
}

// CriterionCoverage names which completed units reference a success
// criterion (structural coverage only).
type CriterionCoverage struct {
	Ref          string   `json:"ref"`
	Criterion    string   `json:"criterion"`
	Units        []string `json:"units,omitempty"`
	Requirements []string `json:"requirements,omitempty"`
}

// AssessGoalCompletion computes the structural assessment from the
// immutable generation and the durable unit completions.
func AssessGoalCompletion(baseline goals.GoalBaseline, completions []UnitCompletion) (GoalCompletionAssessment, error) {
	if baseline.WorkPlan == nil {
		return GoalCompletionAssessment{}, ErrNoAcceptedGoalWorkPlan
	}
	candidates := ApplyCompletions(baseline.WorkPlan.Candidates, completions)
	assessment := GoalCompletionAssessment{AllUnitsComplete: true}
	coverage := make([]CriterionCoverage, len(baseline.SuccessCriteria))
	for i, criterion := range baseline.SuccessCriteria {
		coverage[i] = CriterionCoverage{Ref: "success_criteria/" + strconv.Itoa(i+1), Criterion: criterion}
	}
	for _, candidate := range candidates {
		if !candidate.Completed {
			assessment.AllUnitsComplete = false
			assessment.IncompleteUnits = append(assessment.IncompleteUnits, candidate.ID)
			continue
		}
		for _, requirement := range candidate.Requirements {
			if index, ok := successCriterionIndex(requirement.SourceRef); ok && index <= len(coverage) {
				coverage[index-1].Units = append(coverage[index-1].Units, candidate.ID)
				coverage[index-1].Requirements = append(coverage[index-1].Requirements, requirement.ID)
			}
		}
	}
	for _, item := range coverage {
		if len(item.Units) == 0 {
			assessment.UncoveredCriteria = append(assessment.UncoveredCriteria, item.Ref)
		}
	}
	assessment.Coverage = coverage
	sort.Strings(assessment.UncoveredCriteria)
	assessment.StructurallyComplete = assessment.AllUnitsComplete && len(assessment.UncoveredCriteria) == 0
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
