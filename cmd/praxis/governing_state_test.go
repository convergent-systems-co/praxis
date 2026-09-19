package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// attachedGeneration drives the fixture Goal through the emitted lifecycle
// commands to an attached, drivable generation 2 and returns its digest.
func attachedGeneration(t *testing.T, ctx context.Context, governed func(string) string, dir, goalID string) string {
	t.Helper()
	proposeFixture(t, ctx, governed, dir, "complete")
	st := inspectGoal(t, ctx, governed, goalID, "1")
	executeEmittedOK(t, ctx, governed, emitted(t, st, "proposals", "review_accept_with"))
	st = inspectGoal(t, ctx, governed, goalID, "1")
	executeEmittedOK(t, ctx, governed, emitted(t, st, "reviews", "request_with"))
	st = inspectGoal(t, ctx, governed, goalID, "1")
	executeEmittedOK(t, ctx, governed, emitted(t, st, "authority_requests", "resolve_with"))
	st = inspectGoal(t, ctx, governed, goalID, "1")
	executeEmittedOK(t, ctx, governed, emitted(t, st, "authority_requests", "accept_with"))
	st = inspectGoal(t, ctx, governed, goalID, "1")
	executeEmittedOK(t, ctx, governed, emitted(t, st, "acceptances", "attach_with"))
	st = inspectGoal(t, ctx, governed, goalID, "2")
	return st["goal"].(map[string]any)["digest"].(string)
}

// seedCandidate records one durable turn, its unit completion, the
// completion candidate and the deterministic evaluation (unknown) for
// generation 2 of the fixture Goal, exactly as a controller would.
func seedCandidate(t *testing.T, ctx context.Context, governed func(string) string, goalID, digest string) {
	t.Helper()
	db, err := state.OpenSQLite(ctx, governed("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	turn := goaldrive.TurnRecord{GoalID: goalID, GoalVersion: "2", InvocationID: "inv-c", Mode: goaldrive.ModeContinuous, TurnID: "inv-c:turn:1", ChildObjective: "unit:complete", GraphID: "g", GraphVersion: "1", StartHead: "000000", EndHead: "abc123", Outcome: goaldrive.OutcomeContinue, Progress: true, CheckpointPublished: true, CheckpointEvidence: []string{"repository:declared-validation-passed", "repository:validated-local-commit"}, CompletionClaim: "unit:complete", UnitCompleted: true, CreatedAt: time.Now().UTC()}
	if _, err := ledger.Record(ctx, 0, &turn); err != nil {
		t.Fatal(err)
	}
	completion := goaldrive.UnitCompletion{GoalID: goalID, GoalVersion: "2", UnitID: "unit:complete", InvocationID: "inv-c", TurnID: "inv-c:turn:1", EndHead: "abc123", Requirements: []string{"req:complete"}, Evidence: []string{"checkpoint:abc123"}, CompletedAt: time.Now().UTC()}
	if err := ledger.RecordCompletion(ctx, completion); err != nil {
		t.Fatal(err)
	}
	candidate := goaldrive.GoalCompletionCandidate{GoalID: goalID, GoalVersion: "2", GoalDigest: digest, InvocationID: "inv-c", TurnID: "inv-c:turn:1", FinalHead: "abc123", Units: []goaldrive.UnitCompletion{completion}, Assessment: goaldrive.GoalCompletionAssessment{AllUnitsComplete: true, StructurallyComplete: true}, CandidateAt: time.Now().UTC()}
	if err := ledger.RecordGoalCompletionCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	verifier := goaldrive.GoalCompletionEvaluation{GoalID: goalID, GoalVersion: "2", GoalDigest: digest, CandidateTurnID: "inv-c:turn:1", FinalHead: "abc123", Evaluator: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}, EvaluatorKind: goaldrive.EvaluatorDeterministic, Items: []goaldrive.PredicateEvaluation{{Kind: goaldrive.ItemIntegratedValidator, Ref: goaldrive.IntegratedValidator, Text: "final integrated consequence", Predicate: "requires judgment: the repository declares no validation", Result: goaldrive.ResultUnknown}}, EvaluatedAt: time.Now().UTC()}
	if _, err := ledger.RecordGoalEvaluation(ctx, verifier); err != nil {
		t.Fatal(err)
	}
}

func satisfyAndSettle(t *testing.T, ctx context.Context, governed func(string) string, goalID, digest string) {
	t.Helper()
	options := map[string]string{"goal-id": goalID, "goal-version": "2"}
	var out bytes.Buffer
	doc := []byte(`{"evaluator":{"id":"reviewer","kind":"human"},"evaluator_kind":"human","goal_digest":"` + digest + `","candidate_turn_id":"inv-c:turn:1","final_head":"abc123","findings":[{"ref":"integrated","result":"satisfied","evidence":["exercised the integrated result at abc123"],"judgment":"the contract is met"}]}`)
	if err := runGoalEvaluate(ctx, options, doc, governed, &out); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	out.Reset()
	if err := runGoalCompleteWithTerminal(ctx, options, governed, strings.NewReader("COMPLETE-GOAL "+goalID+"/2\n"), &out); err != nil {
		t.Fatalf("complete: %v\n%s", err, out.String())
	}
}

// TestInspectReportsSettledGenerationAsNotDrivable is the #171 contract:
// governing state derives from durable settlement, not from the presence
// of a WorkPlan. A generation with a durable COMPLETE decision is not
// drivable and its governing state is "complete". (Live evidence: after
// decision event 454 inspect still rendered drivable:true for
// goal:weather-app-continuous/2.)
func TestInspectReportsSettledGenerationAsNotDrivable(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"
	digest := attachedGeneration(t, ctx, governed, dir, goalID)
	seedCandidate(t, ctx, governed, goalID, digest)
	satisfyAndSettle(t, ctx, governed, goalID, digest)
	st := inspectGoal(t, ctx, governed, goalID, "2")
	if st["drivable"] != false || st["governing_state"] != "complete" {
		t.Fatalf("RED #171: a settled generation must report drivable:false and governing_state:complete, got drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	if st["work_set"].(map[string]any)["goal_complete"] != true {
		t.Fatalf("goal_complete must agree: %v", st["work_set"])
	}
}

// TestInspectGoverningStateAcrossTheLifecycle covers the other governing
// states, the rendered turn records, succession and an unattached
// successor.
func TestInspectGoverningStateAcrossTheLifecycle(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"
	st := inspectGoal(t, ctx, governed, goalID, "1")
	if st["drivable"] != false || st["governing_state"] != "unattached" {
		t.Fatalf("generation without a plan: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	digest := attachedGeneration(t, ctx, governed, dir, goalID)
	st = inspectGoal(t, ctx, governed, goalID, "2")
	if st["drivable"] != true || st["governing_state"] != "drivable" {
		t.Fatalf("attached generation: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	seedCandidate(t, ctx, governed, goalID, digest)
	st = inspectGoal(t, ctx, governed, goalID, "2")
	if st["drivable"] != false || st["governing_state"] != "candidate-pending-evaluation" {
		t.Fatalf("candidate with an unknown evaluation: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	records, ok := st["turn_records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("turn records must be rendered: %v", st["turn_records"])
	}
	record := records[0].(map[string]any)
	if record["turn_id"] != "inv-c:turn:1" || record["unit"] != "unit:complete" || record["outcome"] != "CONTINUE" || record["end_head"] != "abc123" || record["start_head"] != "000000" || record["checkpoint_published"] != true || record["completion"] != "turn-time" || record["completed_unit"] != "unit:complete" {
		t.Fatalf("turn record must carry unit, outcome, heads, publication and completion provenance: %v", record)
	}
	options := map[string]string{"goal-id": goalID, "goal-version": "2"}
	var out bytes.Buffer
	doc := []byte(`{"evaluator":{"id":"reviewer","kind":"human"},"evaluator_kind":"human","goal_digest":"` + digest + `","candidate_turn_id":"inv-c:turn:1","final_head":"abc123","findings":[{"ref":"integrated","result":"satisfied","evidence":["exercised the integrated result at abc123"],"judgment":"the contract is met"}]}`)
	if err := runGoalEvaluate(ctx, options, doc, governed, &out); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	if st["drivable"] != false || st["governing_state"] != "candidate-pending-settlement" {
		t.Fatalf("satisfied evaluation awaiting the owner: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	out.Reset()
	incomplete := map[string]string{"goal-id": goalID, "goal-version": "2", "status": "incomplete", "reason": "the plan never addressed operability"}
	if err := runGoalCompleteWithTerminal(ctx, incomplete, governed, strings.NewReader("INCOMPLETE-GOAL "+goalID+"/2\n"), &out); err != nil {
		t.Fatalf("incomplete settlement: %v\n%s", err, out.String())
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	if st["drivable"] != false || st["governing_state"] != "incomplete" {
		t.Fatalf("incomplete settlement: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	out.Reset()
	if err := runGoalSucceedWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2", "reason": "replan for operability"}, governed, strings.NewReader("SUCCEED-GOAL "+goalID+"/2\n"), &out); err != nil {
		t.Fatalf("succeed: %v\n%s", err, out.String())
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	if st["drivable"] != false || st["governing_state"] != "superseded" {
		t.Fatalf("superseded generation: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
	st = inspectGoal(t, ctx, governed, goalID, "3")
	if st["drivable"] != false || st["governing_state"] != "unattached" {
		t.Fatalf("successor without a plan: drivable=%v governing_state=%v", st["drivable"], st["governing_state"])
	}
}
