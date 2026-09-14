package goals

import (
	"testing"

	"github.com/convergent-systems-co/praxis/internal/kernel"
)

func TestGoalsGraphValidates(t *testing.T) {
	if err := Graph().Validate(); err != nil {
		t.Fatalf("goals graph invalid: %v", err)
	}
	if err := Invocation().Validate(); err != nil {
		t.Fatalf("goals invocation invalid: %v", err)
	}
}

func TestGoalGraphIncludesBoundedArchitectureReviewStage(t *testing.T) {
	g := Graph()
	if g.Version != "0.2.0" {
		t.Fatalf("architecture-review graph generation was not versioned: %s", g.Version)
	}
	hasReview, hasDecision := false, false
	for _, node := range g.Nodes {
		if node.ID == "architecture_review" {
			hasReview = true
		}
		if node.ID == "architecture_review_decision" {
			hasDecision = true
		}
	}
	if !hasReview {
		t.Fatal("Goals graph must expose deterministic architecture review stage")
	}
	if !hasDecision {
		t.Fatal("Goals graph must expose human resolution stage")
	}
	wants := []kernel.TransitionDef{
		{From: "model", Outcome: "ready", To: "architecture_review"},
		{From: "architecture_review", Outcome: "ready", To: "specify"},
		{From: "architecture_review", Outcome: "review_required", To: "architecture_review_decision"},
		{From: "architecture_review_decision", Outcome: "resolved", To: "specify"},
	}
	for _, want := range wants {
		found := false
		for _, got := range g.Transitions {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing architecture review transition: %+v", want)
		}
	}
}

func TestGoalDriveInvocationIsRegistryBoundAndProviderNeutral(t *testing.T) {
	inv := GoalDriveInvocation()
	if err := inv.Validate(); err != nil {
		t.Fatal(err)
	}
	if inv.EntryPointID != "goal-drive" || len(inv.Aliases) != 1 || inv.Aliases[0] != "goal-drive" {
		t.Fatalf("unexpected Goal-drive identity: %+v", inv)
	}
	for _, required := range []string{"goal", "goal-file", "goal-id", "provider", "no-progress-limit", "ledger"} {
		found := false
		for _, option := range inv.Options {
			if option.Name == required {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing Goal-drive option %q", required)
		}
	}
}

func TestGoalBaselinePreservesOriginalIntent(t *testing.T) {
	b := GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "I want mornings to stop being chaotic", RefinedOutcome: "Create a predictable low-conflict household morning routine", Rigor: RigorStructured, RecommendationMode: RecommendationReviewAll}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if b.OriginalIntent == b.RefinedOutcome {
		t.Fatal("original user intent must remain distinct from refined outcome")
	}
}

func TestDelegatedModeCanAutoAcceptClearRecommendation(t *testing.T) {
	b := GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: RigorRigorous, RecommendationMode: RecommendationDelegated, Decisions: []Decision{{ID: "d1", Statement: "choose representation", Status: DecisionResolved, Recommendation: "use dependency graph", Rationale: "dominant fit", Reversible: true, AutoAccepted: true}}}
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRecommendationDelegationCannotAutoAcceptHumanRequiredDecision(t *testing.T) {
	b := GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: RigorRigorous, RecommendationMode: RecommendationDelegated, Decisions: []Decision{{ID: "d1", Statement: "publish private information", Status: DecisionResolved, Recommendation: "publish", HumanRequired: true, AutoAccepted: true}}}
	if err := b.Validate(); err == nil {
		t.Fatal("human-required decision must never be auto-accepted")
	}
}

func TestAutoAcceptedDecisionRequiresDelegatedMode(t *testing.T) {
	b := GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: RigorStructured, RecommendationMode: RecommendationReviewAll, Decisions: []Decision{{ID: "d1", Statement: "choice", Status: DecisionResolved, Recommendation: "a", Reversible: true, AutoAccepted: true}}}
	if err := b.Validate(); err == nil {
		t.Fatal("review-all mode cannot silently auto-accept")
	}
}
