package goals

import "testing"

func TestGoalsGraphValidates(t *testing.T) {
	if err := Graph().Validate(); err != nil {
		t.Fatalf("goals graph invalid: %v", err)
	}
	if err := Invocation().Validate(); err != nil {
		t.Fatalf("goals invocation invalid: %v", err)
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
