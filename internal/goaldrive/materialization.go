package goaldrive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var (
	// ErrAlreadyMaterialized reports that the unit the turn proposed is
	// already durably complete; materialization is idempotent and refuses.
	ErrAlreadyMaterialized = errors.New("unit completion is already materialized")
	// ErrTurnNotQualified reports a historical turn that never satisfied
	// the completion predicates (validated progress, published checkpoint,
	// declared validation passed); it has nothing to materialize.
	ErrTurnNotQualified = errors.New("historical turn is not a qualified, published checkpoint")
	// ErrConsequenceChanged reports that the exact checkpoint the turn
	// published is no longer the published consequence.
	ErrConsequenceChanged = errors.New("the turn's consequence is no longer the published one")
	// ErrNoCompletionProposal reports a qualified consequence that carries
	// no proposal: there is nothing to materialize.
	ErrNoCompletionProposal = errors.New("the turn's consequence carries no completion proposal")
)

// CompletionMaterialization is the provenance of a unit completion derived
// after its turn ended, by `praxis supervise materialize`, from the exact
// durable consequence the turn published (#164, #165). It is deterministic
// re-materialization, not settlement: no judgment is exercised and no new
// evidence is created. At is the instant of materialization, never the
// turn's.
type CompletionMaterialization struct {
	By             contracts.PrincipalRef `json:"by"`
	At             time.Time              `json:"at"`
	TurnRecordedAt time.Time              `json:"turn_recorded_at,omitempty"`
	Reason         string                 `json:"reason"`
}

// Materialization is the result of MaterializeTurnCompletion: the untouched
// historical turn, the completion derived from it, and whether that
// completion derived the Goal completion candidate (and its deterministic
// evaluation) exactly as the turn would have.
type Materialization struct {
	Turn           TurnRecord     `json:"turn"`
	Completion     UnitCompletion `json:"completion"`
	GoalCandidate  bool           `json:"goal_candidate"`
	GoalEvaluation string         `json:"goal_evaluation,omitempty"`
}

// PublishedCheckpointRepository verifies that a turn's exact consequence
// (start..end) is still what the branch publishes.
type PublishedCheckpointRepository interface {
	CheckpointPublished(ctx context.Context, startHead, endHead string) error
}

// MaterializeTurnCompletion derives the UnitCompletion a qualified,
// published historical turn earned but never recorded (#164). It binds the
// exact Goal generation, turn, and start/end checkpoint; requires the
// durable turn record's own qualification predicates; requires the
// published branch to still contain the exact checkpoint; re-runs the
// completion recognizer over exactly that consequence; requires the
// proposal to name the turn's selected unit; records the completion with
// materialization provenance, idempotently; and derives the Goal completion
// candidate and deterministic evaluation the turn would have derived. It
// never rewrites the historical turn.
func (c Controller) MaterializeTurnCompletion(ctx context.Context, goalID, version, turnID string, baseline goals.GoalBaseline, repo RepositoryAdapter, by contracts.PrincipalRef) (Materialization, error) {
	if goalID == "" || version == "" || turnID == "" {
		return Materialization{}, errors.New("materialization requires the exact Goal generation and turn")
	}
	if baseline.ID != goalID || baseline.Version != version {
		return Materialization{}, fmt.Errorf("baseline %s/%s is not Goal %s/%s", baseline.ID, baseline.Version, goalID, version)
	}
	if safety, safetyErr := c.safetyBearing(ctx, &baseline); safetyErr != nil {
		return Materialization{}, safetyErr
	} else if safety {
		return Materialization{}, errors.New("safety-bearing completion cannot be reconstructed without its original conformance run; a fresh governed turn is required")
	}
	if by.ID == "" {
		return Materialization{}, errors.New("materialization requires the acting principal")
	}
	turns, err := c.Ledger.Load(ctx, goalID, version)
	if err != nil {
		return Materialization{}, fmt.Errorf("load Goal-drive ledger: %w", err)
	}
	var record *TurnRecord
	for i := range turns {
		if turns[i].TurnID == turnID {
			record = &turns[i]
		}
	}
	if record == nil {
		return Materialization{}, fmt.Errorf("turn %s is not recorded for Goal %s/%s", turnID, goalID, version)
	}
	if record.Outcome != OutcomeContinue || record.ChildObjective == "" || record.EndHead == "" {
		return Materialization{}, fmt.Errorf("%w: turn %s ended %s for %q", ErrTurnNotQualified, turnID, record.Outcome, record.ChildObjective)
	}
	if missing := unitCompletionPredicates(*record, false); len(missing) > 0 {
		return Materialization{}, fmt.Errorf("%w: turn %s lacks %s", ErrTurnNotQualified, turnID, strings.Join(missing, ", "))
	}
	if record.UnitCompleted {
		return Materialization{}, fmt.Errorf("%w: turn %s recorded completion of %s", ErrAlreadyMaterialized, turnID, record.ChildObjective)
	}
	completions, err := c.Ledger.LoadCompletions(ctx, goalID, version)
	if err != nil {
		return Materialization{}, fmt.Errorf("load unit completions: %w", err)
	}
	for _, existing := range completions {
		if existing.UnitID == record.ChildObjective {
			return Materialization{}, fmt.Errorf("%w: %s completed by turn %s", ErrAlreadyMaterialized, existing.UnitID, existing.TurnID)
		}
	}
	published, ok := repo.(PublishedCheckpointRepository)
	if !ok {
		return Materialization{}, errors.New("repository adapter cannot verify checkpoint publication")
	}
	if err := published.CheckpointPublished(ctx, record.StartHead, record.EndHead); err != nil {
		return Materialization{}, fmt.Errorf("%w: %v", ErrConsequenceChanged, err)
	}
	claims, ok := repo.(CompletionClaimRepository)
	if !ok {
		return Materialization{}, errors.New("repository adapter cannot read completion proposals")
	}
	units, err := claims.CompletionClaims(ctx, record.StartHead, record.EndHead)
	if err != nil {
		return Materialization{}, err
	}
	if len(units) == 0 {
		return Materialization{}, fmt.Errorf("%w: turn %s (%s..%s)", ErrNoCompletionProposal, turnID, record.StartHead, record.EndHead)
	}
	for _, unit := range units {
		if unit != record.ChildObjective {
			return Materialization{}, fmt.Errorf("turn %s proposes completion of %s, which is not the selected unit %s; nothing is materialized", turnID, unit, record.ChildObjective)
		}
	}
	candidates, relationships, err := MaterializeGoalWork(baseline)
	if err != nil {
		return Materialization{}, err
	}
	var requirements []string
	for _, candidate := range candidates {
		if candidate.ID == record.ChildObjective {
			for _, requirement := range candidate.Requirements {
				requirements = append(requirements, requirement.ID)
			}
		}
	}
	now := time.Now().UTC()
	completion := UnitCompletion{GoalID: goalID, GoalVersion: version, UnitID: record.ChildObjective, InvocationID: record.InvocationID, TurnID: record.TurnID, EndHead: record.EndHead, Requirements: requirements,
		Evidence:    append(append([]string{"checkpoint:" + record.EndHead}, record.CheckpointEvidence...), "materialized:supervise.materialize"),
		CompletedAt: now, Materialization: &CompletionMaterialization{By: by, At: now, TurnRecordedAt: record.CreatedAt, Reason: "completion proposal in the qualified checkpoint was not recognized when the turn ended (#164)"}}
	// The durable completion is the compare-and-set: concurrent
	// materializations of one consequence yield exactly one record.
	if err := c.Ledger.RecordCompletion(ctx, completion); err != nil {
		current, loadErr := c.Ledger.LoadCompletions(ctx, goalID, version)
		if loadErr == nil {
			for _, existing := range current {
				if existing.UnitID == completion.UnitID {
					return Materialization{}, fmt.Errorf("%w: %s completed by turn %s", ErrAlreadyMaterialized, existing.UnitID, existing.TurnID)
				}
			}
		}
		return Materialization{}, fmt.Errorf("record materialized unit completion: %w", err)
	}
	req := TurnRequest{GoalID: goalID, GoalVersion: version, InvocationID: record.InvocationID, TurnID: record.TurnID, ChildObjective: record.ChildObjective, GraphID: record.GraphID, GraphVersion: record.GraphVersion, Mode: record.Mode, ProviderID: "materialization", GoalBaseline: &baseline, WorkCandidates: candidates, WorkRelationships: relationships}
	provenance := map[string]string{"scope": "unit", "unit": completion.UnitID, "end_head": completion.EndHead, "materialized": "true", "materialized_by": by.ID, "materialized_at": now.Format(time.RFC3339Nano)}
	if err := c.emit(ctx, ActivityCompletionClaimed, req, provenance); err != nil {
		return Materialization{}, err
	}
	provenance["requirements"] = strings.Join(requirements, ",")
	if err := c.emit(ctx, ActivityUnitCompleted, req, provenance); err != nil {
		return Materialization{}, err
	}
	derived, err := c.deriveGoalCandidate(ctx, req, repo, *record)
	if err != nil {
		return Materialization{}, err
	}
	return Materialization{Turn: *record, Completion: completion, GoalCandidate: derived.GoalCandidate, GoalEvaluation: derived.GoalEvaluation}, nil
}
