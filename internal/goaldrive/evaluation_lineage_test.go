package goaldrive

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestEvaluationLineageIsFencedAtTheLedger: a composed evaluation must be
// based on the current latest evaluation; a second deterministic
// evaluation is refused; the chain verifies and detects a broken link.
func TestEvaluationLineageIsFencedAtTheLedger(t *testing.T) {
	ctx := context.Background()
	baseline := chainBaseline()
	ledger := Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	completion := UnitCompletion{GoalID: baseline.ID, GoalVersion: baseline.Version, UnitID: "unit:a", InvocationID: "i", TurnID: "i:turn:1", EndHead: "h", Evidence: []string{"checkpoint:h"}, CompletedAt: time.Now().UTC()}
	candidate := GoalCompletionCandidate{GoalID: baseline.ID, GoalVersion: baseline.Version, GoalDigest: baseline.Digest, InvocationID: "i", TurnID: "i:turn:1", FinalHead: "h", Units: []UnitCompletion{completion}, Assessment: GoalCompletionAssessment{AllUnitsComplete: true}, CandidateAt: time.Now().UTC()}
	if err := ledger.RecordGoalCompletionCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	deterministic := GoalCompletionEvaluation{GoalID: baseline.ID, GoalVersion: baseline.Version, GoalDigest: baseline.Digest, CandidateTurnID: "i:turn:1", FinalHead: "h", Evaluator: contracts.PrincipalRef{ID: "controller", Kind: "controller"}, EvaluatorKind: EvaluatorDeterministic, Items: []PredicateEvaluation{{Kind: ItemIntegratedValidator, Ref: IntegratedValidator, Predicate: "none", Result: ResultUnknown}}, EvaluatedAt: time.Now().UTC()}
	d0, err := ledger.RecordGoalEvaluation(ctx, deterministic)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.RecordGoalEvaluation(ctx, deterministic); err == nil {
		t.Fatal("RED #169: a second deterministic evaluation must be refused; the verifier is the root of the chain")
	}
	reviewer := contracts.PrincipalRef{ID: "r", Kind: "human"}
	finding := []Finding{{Ref: IntegratedValidator, Result: ResultSatisfied, Evidence: []string{"x"}, Judgment: "ok"}}
	first, err := ComposeEvaluation(deterministic, reviewer, EvaluatorHuman, finding)
	if err != nil {
		t.Fatal(err)
	}
	d1, err := ledger.RecordGoalEvaluation(ctx, first)
	if err != nil || first.BasedOn != d0 {
		t.Fatalf("composition over the latest is recorded with lineage: %v based_on=%s", err, first.BasedOn)
	}
	staleComposition, _ := ComposeEvaluation(deterministic, reviewer, EvaluatorHuman, finding)
	if _, err := ledger.RecordGoalEvaluation(ctx, staleComposition); err == nil || !strings.Contains(err.Error(), d1) {
		t.Fatalf("RED #169: a composition over a superseded evaluation must be refused naming the current latest %s: %v", d1, err)
	}
	state, err := ledger.LoadGoalCompletion(ctx, baseline.ID, baseline.Version)
	if err != nil || len(state.Evaluations) != 2 {
		t.Fatalf("exactly the fenced chain is durable: %v %d", err, len(state.Evaluations))
	}
	if err := VerifyEvaluationChain(state.Evaluations); err != nil {
		t.Fatalf("an unbroken chain verifies: %v", err)
	}
	broken := append([]GoalCompletionEvaluation(nil), state.Evaluations...)
	broken[1].BasedOn = "sha256:" + strings.Repeat("0", 64)
	if err := VerifyEvaluationChain(broken); err == nil {
		t.Fatal("a broken link must be detected")
	}
	unrooted := []GoalCompletionEvaluation{first}
	if err := VerifyEvaluationChain(unrooted); err == nil {
		t.Fatal("a chain not rooted at the deterministic verifier must be refused")
	}
}
