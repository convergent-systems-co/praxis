package main

import (
	"github.com/convergent-systems-co/praxis/internal/goaldrive"
)

// Governing states of a Goal generation as inspect reports them. They are
// derived from durable lifecycle and settlement state, never from the mere
// presence of a WorkPlan (#171): goal-drive refuses a settled generation
// and one whose completion candidate awaits evaluation or settlement, so
// inspect must not call either drivable.
const (
	governingUnattached                 = "unattached"
	governingDrivable                   = "drivable"
	governingCandidatePendingEvaluation = "candidate-pending-evaluation"
	governingCandidatePendingSettlement = "candidate-pending-settlement"
	governingComplete                   = "complete"
	governingIncomplete                 = "incomplete"
	governingSuperseded                 = "superseded"
)

// governingState derives the generation's governing state and whether
// goal-drive would admit a turn on it.
func governingState(hasPlan bool, goalState goaldrive.GoalCompletionState) (string, bool) {
	switch {
	case !hasPlan:
		return governingUnattached, false
	case goalState.Succession != nil:
		return governingSuperseded, false
	case goalState.Decision != nil:
		if goalState.Decision.Status == goaldrive.GoalComplete {
			return governingComplete, false
		}
		return governingIncomplete, false
	case goalState.Candidate != nil:
		if latest := goalState.Latest(); latest != nil && latest.Outcome == goaldrive.ResultSatisfied {
			return governingCandidatePendingSettlement, false
		}
		return governingCandidatePendingEvaluation, false
	}
	return governingDrivable, true
}

// turnRecordEntries renders the generation's durable turn records with the
// facts a supervisor otherwise reconstructs from raw events: unit, outcome,
// heads, publication, the proposal seen at turn time, and the completion
// the turn earned (at turn time or by later materialization).
func turnRecordEntries(turns []goaldrive.TurnRecord, completions []goaldrive.UnitCompletion) []map[string]any {
	byTurn := map[string]goaldrive.UnitCompletion{}
	for _, completion := range completions {
		byTurn[completion.TurnID] = completion
	}
	entries := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		entry := map[string]any{"turn_id": turn.TurnID, "invocation_id": turn.InvocationID, "mode": turn.Mode, "unit": turn.ChildObjective, "outcome": turn.Outcome, "progress": turn.Progress, "checkpoint_published": turn.CheckpointPublished, "start_head": turn.StartHead, "end_head": turn.EndHead, "checkpoint_evidence": turn.CheckpointEvidence, "completion": "none"}
		if turn.CompletionClaim != "" {
			entry["completion_claim"] = turn.CompletionClaim
		}
		if turn.Blocker != "" {
			entry["blocker"] = turn.Blocker
		}
		if !turn.CreatedAt.IsZero() {
			entry["recorded_at"] = turn.CreatedAt
		}
		if turn.GoalCandidate {
			entry["goal_candidate"] = true
		}
		if completion, ok := byTurn[turn.TurnID]; ok {
			entry["completed_unit"] = completion.UnitID
			entry["completed_at"] = completion.CompletedAt
			if completion.Materialization != nil {
				entry["completion"] = "materialized"
				entry["materialization"] = completion.Materialization
			} else {
				entry["completion"] = "turn-time"
			}
		}
		entries = append(entries, entry)
	}
	return entries
}
