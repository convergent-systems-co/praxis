package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestGoalCompletionIsEvaluatedThenSettledThenSucceeded proves the public
// surface keeps the four states distinct: candidate (evidence), evaluation
// (evidence, composable, fail-closed on stale binding), settlement
// (authority, cannot turn unknown into satisfied, binds the evaluation),
// and succession (a successor generation preserving the predecessor).
func TestGoalCompletionIsEvaluatedThenSettledThenSucceeded(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"
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
	workSet := st["work_set"].(map[string]any)
	if workSet["state"] != "runnable" || workSet["goal_complete"] != false {
		t.Fatalf("fresh generation: %v", workSet)
	}
	digest := st["goal"].(map[string]any)["digest"].(string)
	options := map[string]string{"goal-id": goalID, "goal-version": "2"}
	var out bytes.Buffer
	// No candidate: settlement and evaluation refuse.
	if err := runGoalCompleteWithTerminal(ctx, options, governed, strings.NewReader("COMPLETE-GOAL goal:pending-surface/2\n"), &out); err == nil || !strings.Contains(err.Error(), "no completion candidate") {
		t.Fatalf("complete without a candidate must refuse: %v", err)
	}
	db, err := state.OpenSQLite(ctx, governed("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	completion := goaldrive.UnitCompletion{GoalID: goalID, GoalVersion: "2", UnitID: "unit:complete", InvocationID: "inv-c", TurnID: "inv-c:turn:1", EndHead: "abc123", Requirements: []string{"req:complete"}, Evidence: []string{"checkpoint:abc123"}, CompletedAt: time.Now().UTC()}
	if err := ledger.RecordCompletion(ctx, completion); err != nil {
		t.Fatal(err)
	}
	candidate := goaldrive.GoalCompletionCandidate{GoalID: goalID, GoalVersion: "2", GoalDigest: digest, InvocationID: "inv-c", TurnID: "inv-c:turn:1", FinalHead: "abc123", Units: []goaldrive.UnitCompletion{completion}, Assessment: goaldrive.GoalCompletionAssessment{AllUnitsComplete: true, StructurallyComplete: true}, CandidateAt: time.Now().UTC()}
	if err := ledger.RecordGoalCompletionCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	// Candidate without evaluation: settlement refuses (evidence required).
	if err := runGoalCompleteWithTerminal(ctx, options, governed, strings.NewReader("COMPLETE-GOAL goal:pending-surface/2\n"), &out); err == nil || !strings.Contains(err.Error(), "no evaluation") {
		t.Fatalf("settlement without evaluation must refuse: %v", err)
	}
	// The deterministic verifier's evaluation: one unknown item (the fixture
	// Goal has no criteria; the integrated validation is unknown without a
	// repository).
	verifier := goaldrive.GoalCompletionEvaluation{GoalID: goalID, GoalVersion: "2", GoalDigest: digest, CandidateTurnID: "inv-c:turn:1", FinalHead: "abc123", Evaluator: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}, EvaluatorKind: goaldrive.EvaluatorDeterministic, Items: []goaldrive.PredicateEvaluation{{Kind: goaldrive.ItemIntegratedValidator, Ref: goaldrive.IntegratedValidator, Text: "final integrated consequence", Predicate: "requires judgment: the repository declares no validation", Result: goaldrive.ResultUnknown}}, EvaluatedAt: time.Now().UTC()}
	if _, err := ledger.RecordGoalEvaluation(ctx, verifier); err != nil {
		t.Fatal(err)
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	workSet = st["work_set"].(map[string]any)
	evaluation := workSet["goal_evaluation"].(map[string]any)
	if evaluation["outcome"] != "unknown" || workSet["complete_with"] != nil || workSet["reject_with"] == nil || workSet["goal_completion_candidate"].(map[string]any)["authoritative"] != false {
		t.Fatalf("inspect must show an unknown evaluation, no complete_with, and the candidate as non-authoritative: %v", workSet)
	}
	// G: COMPLETE cannot be settled on an unknown evaluation.
	if err := runGoalCompleteWithTerminal(ctx, options, governed, strings.NewReader("COMPLETE-GOAL goal:pending-surface/2\n"), &out); err == nil || !strings.Contains(err.Error(), "is unknown") {
		t.Fatalf("settlement must not turn unknown into satisfied: %v", err)
	}
	// F: an evaluation document with a stale checkpoint fails closed.
	stale := []byte(`{"evaluator":{"id":"reviewer","kind":"human"},"evaluator_kind":"human","goal_digest":"` + digest + `","candidate_turn_id":"inv-c:turn:1","final_head":"zzz","findings":[{"ref":"integrated","result":"satisfied","evidence":["ran it"],"judgment":"ok"}]}`)
	if err := runGoalEvaluate(ctx, options, stale, governed, &out); err == nil || !strings.Contains(err.Error(), "binds") {
		t.Fatalf("stale evaluation must fail closed: %v", err)
	}
	// A human evaluator resolves the unknown with evidence and judgment.
	out.Reset()
	doc := []byte(`{"evaluator":{"id":"reviewer","kind":"human"},"evaluator_kind":"human","goal_digest":"` + digest + `","candidate_turn_id":"inv-c:turn:1","final_head":"abc123","findings":[{"ref":"integrated","result":"satisfied","evidence":["exercised the integrated result at abc123"],"judgment":"the contract is met"}]}`)
	if err := runGoalEvaluate(ctx, options, doc, governed, &out); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	var evaluated map[string]any
	if err := json.Unmarshal(out.Bytes()[strings.Index(out.String(), "{"):], &evaluated); err != nil {
		t.Fatal(err)
	}
	if evaluated["outcome"] != "satisfied" {
		t.Fatalf("composed evaluation: %v", evaluated)
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	workSet = st["work_set"].(map[string]any)
	if workSet["complete_with"] != completeCommand(goalID, "2") || workSet["goal_evaluation"].(map[string]any)["evaluator_kind"] != "human" {
		t.Fatalf("a satisfied evaluation exposes the literal settlement command: %v", workSet)
	}
	// Wrong confirmation fails closed and the owner sees the evaluation.
	out.Reset()
	if err := runGoalCompleteWithTerminal(ctx, options, governed, strings.NewReader("yes\n"), &out); !errors.Is(err, errAuthorityBootstrapConfirmation) {
		t.Fatalf("wrong confirmation must fail closed: %v", err)
	}
	if !strings.Contains(out.String(), "Prove pending authority is actionable") || !strings.Contains(out.String(), "integrated [SATISFIED]") || !strings.Contains(out.String(), "judgment: the contract is met") {
		t.Fatalf("the settler must see the contract and the evaluation: %s", out.String())
	}
	// Settlement: INCOMPLETE with reason, then succession preserves the predecessor.
	out.Reset()
	incomplete := map[string]string{"goal-id": goalID, "goal-version": "2", "status": "incomplete", "reason": "the plan never addressed operability"}
	if err := runGoalCompleteWithTerminal(ctx, incomplete, governed, strings.NewReader("INCOMPLETE-GOAL goal:pending-surface/2\n"), &out); err != nil {
		t.Fatalf("incomplete settlement: %v\n%s", err, out.String())
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	workSet = st["work_set"].(map[string]any)
	if workSet["goal_complete"] != false || workSet["succeed_with"] == nil || workSet["goal_completion_decision"].(map[string]any)["status"] != "incomplete" {
		t.Fatalf("incomplete settlement must be durable and name succession: %v", workSet)
	}
	if err := runGoalSucceedWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2"}, governed, strings.NewReader("x\n"), &out); err == nil || !strings.Contains(err.Error(), "--reason") {
		t.Fatalf("succession requires a reason: %v", err)
	}
	out.Reset()
	if err := runGoalSucceedWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2", "reason": "replan for operability"}, governed, strings.NewReader("SUCCEED-GOAL goal:pending-surface/2\n"), &out); err != nil {
		t.Fatalf("succeed: %v\n%s", err, out.String())
	}
	var succeeded map[string]any
	if err := json.Unmarshal(out.Bytes()[strings.Index(out.String(), "{"):], &succeeded); err != nil {
		t.Fatal(err)
	}
	if succeeded["successor_version"] != "3" || succeeded["predecessor_digest"] != digest {
		t.Fatalf("succession: %v", succeeded)
	}
	successor := inspectGoal(t, ctx, governed, goalID, "3")
	predecessor := successor["predecessor_completion"].(map[string]any)
	if successor["drivable"] != false || predecessor["goal_version"] != "2" || predecessor["decision"].(map[string]any)["status"] != "incomplete" || len(predecessor["completed_units"].([]any)) != 1 {
		t.Fatalf("the successor must show its predecessor's completed units, evaluation, and gap, and await a plan: %v", successor)
	}
	// Generation 2 is untouched; replay of settlement and succession is truthful.
	st = inspectGoal(t, ctx, governed, goalID, "2")
	if st["goal"].(map[string]any)["digest"] != digest || st["work_set"].(map[string]any)["succession"] == nil {
		t.Fatalf("predecessor must be preserved and reference its successor: %v", st["work_set"])
	}
	out.Reset()
	if err := runGoalSucceedWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2", "reason": "again"}, governed, strings.NewReader("SUCCEED-GOAL goal:pending-surface/2\n"), &out); err != nil || !strings.Contains(out.String(), `"replay": true`) {
		t.Fatalf("a second succession replays: %v %s", err, out.String())
	}
}

func executeEmittedOK(t *testing.T, ctx context.Context, governed func(string) string, command string) {
	t.Helper()
	if _, err := executeEmitted(t, ctx, governed, command); err != nil {
		t.Fatalf("%s: %v", command, err)
	}
}
