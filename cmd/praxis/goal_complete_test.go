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

// TestGoalCompletionIsOwnerEvaluatedAfterProvisionalClaim proves that the
// public `complete` operation refuses without a provisional claim, requires
// the owner's typed confirmation, records exactly one authoritative
// decision bound to the claim, replays truthfully, and that inspect renders
// the provisional and authoritative state with the literal next command.
func TestGoalCompletionIsOwnerEvaluatedAfterProvisionalClaim(t *testing.T) {
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
	if workSet["state"] != "runnable" || workSet["next_unit"] != "unit:complete" || workSet["goal_complete"] != false {
		t.Fatalf("fresh generation: %v", workSet)
	}
	// No claim yet: the operation refuses.
	var out bytes.Buffer
	if err := runGoalCompleteWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2"}, governed, strings.NewReader("COMPLETE-GOAL goal:pending-surface/2\n"), &out); err == nil || !strings.Contains(err.Error(), "no provisional completion claim") {
		t.Fatalf("complete without a claim must refuse: %v", err)
	}
	// The controller's durable unit completion and provisional claim.
	db, err := state.OpenSQLite(ctx, governed("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "praxis-goal-drive", Kind: "controller"}}
	goal := st["goal"].(map[string]any)
	digest := goal["digest"].(string)
	completion := goaldrive.UnitCompletion{GoalID: goalID, GoalVersion: "2", UnitID: "unit:complete", InvocationID: "inv-c", TurnID: "inv-c:turn:1", EndHead: "abc123", Requirements: []string{"req:complete"}, Evidence: []string{"checkpoint:abc123"}, CompletedAt: time.Now().UTC()}
	if err := ledger.RecordCompletion(ctx, completion); err != nil {
		t.Fatal(err)
	}
	claim := goaldrive.GoalCompletionClaim{GoalID: goalID, GoalVersion: "2", GoalDigest: digest, InvocationID: "inv-c", TurnID: "inv-c:turn:1", FinalHead: "abc123", Units: []goaldrive.UnitCompletion{completion}, Assessment: goaldrive.GoalCompletionAssessment{AllUnitsComplete: true, Complete: true}, ClaimedAt: time.Now().UTC()}
	if err := ledger.RecordGoalCompletionClaim(ctx, claim); err != nil {
		t.Fatal(err)
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	workSet = st["work_set"].(map[string]any)
	completeWith, _ := workSet["complete_with"].(string)
	if workSet["goal_complete"] != false || completeWith != "praxis goals-lifecycle --operation=complete --goal-id=goal:pending-surface --goal-version=2" || workSet["goal_completion_claim"].(map[string]any)["authoritative"] != false {
		t.Fatalf("inspect must show the provisional claim and the literal evaluation command: %v", workSet)
	}
	// Wrong confirmation fails closed; incomplete needs a reason.
	out.Reset()
	if err := runGoalCompleteWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2"}, governed, strings.NewReader("yes\n"), &out); !errors.Is(err, errAuthorityBootstrapConfirmation) {
		t.Fatalf("wrong confirmation must fail closed: %v", err)
	}
	if !strings.Contains(out.String(), "Prove pending authority is actionable") || !strings.Contains(out.String(), "unit:complete: turn inv-c:turn:1, checkpoint abc123") {
		t.Fatalf("the owner must see the original Goal contract and the completed units: %s", out.String())
	}
	if err := runGoalCompleteWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2", "status": "incomplete"}, governed, strings.NewReader("x\n"), &out); err == nil || !strings.Contains(err.Error(), "--reason") {
		t.Fatalf("incomplete requires a reason: %v", err)
	}
	// The owner's typed decision records exactly one authoritative completion.
	out.Reset()
	if err := runGoalCompleteWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2"}, governed, strings.NewReader("COMPLETE-GOAL goal:pending-surface/2\n"), &out); err != nil {
		t.Fatalf("owner completion: %v\n%s", err, out.String())
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes()[strings.Index(out.String(), "{"):], &result); err != nil {
		t.Fatal(err)
	}
	if result["status"] != "complete" || result["decision"].(map[string]any)["claim_turn_id"] != "inv-c:turn:1" {
		t.Fatalf("decision must bind the claim: %v", result)
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	workSet = st["work_set"].(map[string]any)
	if workSet["goal_complete"] != true || workSet["complete_with"] != nil {
		t.Fatalf("inspect must show authoritative completion and no further evaluation command: %v", workSet)
	}
	out.Reset()
	if err := runGoalCompleteWithTerminal(ctx, map[string]string{"goal-id": goalID, "goal-version": "2"}, governed, strings.NewReader("COMPLETE-GOAL goal:pending-surface/2\n"), &out); err != nil || !strings.Contains(out.String(), `"replay": true`) {
		t.Fatalf("a decided Goal replays the decision: %v %s", err, out.String())
	}
}

func executeEmittedOK(t *testing.T, ctx context.Context, governed func(string) string, command string) {
	t.Helper()
	if _, err := executeEmitted(t, ctx, governed, command); err != nil {
		t.Fatalf("%s: %v", command, err)
	}
}
