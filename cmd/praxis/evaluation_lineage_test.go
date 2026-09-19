package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func latestEvaluationDigest(t *testing.T, st map[string]any) string {
	t.Helper()
	return st["work_set"].(map[string]any)["goal_evaluation"].(map[string]any)["evaluation_digest"].(string)
}

func evaluationDoc(digest, basedOn, judgment string) []byte {
	based := ""
	if basedOn != "" {
		based = `"based_on":"` + basedOn + `",`
	}
	return []byte(`{"evaluator":{"id":"reviewer","kind":"human"},"evaluator_kind":"human",` + based + `"goal_digest":"` + digest + `","candidate_turn_id":"inv-c:turn:1","final_head":"abc123","findings":[{"ref":"integrated","result":"satisfied","evidence":["exercised the integrated result at abc123"],"judgment":"` + judgment + `"}]}`)
}

// TestEvaluationCompositionRequiresExplicitLineage is the #169 contract:
// an evaluation declares the exact evaluation it composes over; a missing
// or stale base is refused naming the current latest; inspect renders the
// chain; the settlement challenge shows the chain it binds. (Weather II:
// the T9 evaluation was composed on the blind evaluation only because
// nothing else had been recorded in between.)
func TestEvaluationCompositionRequiresExplicitLineage(t *testing.T) {
	ctx := context.Background()
	governed, _, _, dir := lifecycleFixture(t, ctx)
	const goalID = "goal:pending-surface"
	digest := attachedGeneration(t, ctx, governed, dir, goalID)
	seedCandidate(t, ctx, governed, goalID, digest)
	st := inspectGoal(t, ctx, governed, goalID, "2")
	d0 := latestEvaluationDigest(t, st)
	options := map[string]string{"goal-id": goalID, "goal-version": "2"}
	var out bytes.Buffer
	if err := runGoalEvaluate(ctx, options, evaluationDoc(digest, "", "no lineage"), governed, &out); err == nil || !strings.Contains(err.Error(), "based_on") || !strings.Contains(err.Error(), d0) {
		t.Fatalf("RED #169: an evaluation without based_on must be refused naming the current latest %s: %v", d0, err)
	}
	stale := "sha256:" + strings.Repeat("0", 64)
	if err := runGoalEvaluate(ctx, options, evaluationDoc(digest, stale, "stale base"), governed, &out); err == nil || !strings.Contains(err.Error(), d0) {
		t.Fatalf("a stale based_on must be refused naming the current latest: %v", err)
	}
	if st := inspectGoal(t, ctx, governed, goalID, "2"); latestEvaluationDigest(t, st) != d0 || st["work_set"].(map[string]any)["goal_evaluation"].(map[string]any)["evaluations"] != float64(1) {
		t.Fatalf("refused evaluations must record nothing: %v", st["work_set"])
	}
	if !strings.Contains(st["work_set"].(map[string]any)["evaluate_with"].(string), "based_on "+d0) {
		t.Fatalf("evaluate_with must name the base to compose over: %v", st["work_set"].(map[string]any)["evaluate_with"])
	}
	out.Reset()
	if err := runGoalEvaluate(ctx, options, evaluationDoc(digest, d0, "first judgment"), governed, &out); err != nil {
		t.Fatalf("evaluate over the current latest: %v", err)
	}
	st = inspectGoal(t, ctx, governed, goalID, "2")
	d1 := latestEvaluationDigest(t, st)
	chain, ok := st["work_set"].(map[string]any)["evaluation_chain"].([]any)
	if !ok || len(chain) != 2 || chain[0].(map[string]any)["digest"] != d0 || chain[1].(map[string]any)["digest"] != d1 || chain[1].(map[string]any)["based_on"] != d0 {
		t.Fatalf("inspect must render the evaluation chain deterministic → latest: %v", st["work_set"].(map[string]any)["evaluation_chain"])
	}
	// A second evaluator composing over the superseded base is fenced.
	if err := runGoalEvaluate(ctx, options, evaluationDoc(digest, d0, "second, stale"), governed, &out); err == nil || !strings.Contains(err.Error(), d1) {
		t.Fatalf("composition over a superseded evaluation must be refused naming the current latest %s: %v", d1, err)
	}
	// The settlement challenge shows the chain it binds.
	out.Reset()
	if err := runGoalCompleteWithTerminal(ctx, options, governed, strings.NewReader("yes\n"), &out); err == nil {
		t.Fatal("wrong confirmation must fail closed")
	}
	if !strings.Contains(out.String(), "Evaluation chain") || !strings.Contains(out.String(), d0+" -> "+d1) {
		t.Fatalf("the settler must see the evaluation chain: %s", out.String())
	}
}
