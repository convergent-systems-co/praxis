package goalstore

import (
	"context"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

func TestFinalizeEphemeralNeedsNoStoreOrCrypto(t *testing.T) {
	r := Repository{}
	b := goals.GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "think through an idea", RefinedOutcome: "produce a clear decision-ready idea baseline", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll}
	result, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: b, Persist: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Persisted {
		t.Fatal("ephemeral baseline must not be persisted")
	}
	if result.Baseline.Digest == "" {
		t.Fatal("ephemeral baseline still needs exact digest for handoff")
	}
}

func TestFinalizeDurableRequiresStore(t *testing.T) {
	r := Repository{}
	b := goals.GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll}
	if _, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: b, Persist: true, CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("durable request without persistence boundary must fail")
	}
}

func TestFinalizeRejectsMutationAgainstExistingDigest(t *testing.T) {
	r := Repository{}
	b := goals.GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "goal", RefinedOutcome: "outcome", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll}
	d, _ := b.ComputeDigest()
	b.Digest = d
	b.RefinedOutcome = "changed"
	if _, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: b, Persist: false}); err == nil {
		t.Fatal("ephemeral mode must not bypass baseline integrity")
	}
}
