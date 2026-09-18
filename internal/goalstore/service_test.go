package goalstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func finalizedFixture(t *testing.T, reviewRequired bool) (goals.GoalBaseline, goals.BaselineReviewReceipt) {
	t.Helper()
	b := goals.GoalBaseline{ID: "g1", Version: "1", OriginalIntent: "think through an idea", RefinedOutcome: "produce a clear decision-ready idea baseline", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll}
	digest, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	b.Digest = digest
	req := architecturereview.Request{
		Capability: "handoff", ProposedOwner: "core", ReusableAcrossScopes: true,
		GoalEvidence:      []architecturereview.EvidenceRef{{ID: "baseline:g1@1", Kind: "goal_baseline", Digest: digest}},
		InvariantEvidence: []architecturereview.EvidenceRef{{ID: "invariant", Kind: "invariant", Digest: "sha256:invariant"}},
	}
	if !reviewRequired {
		req.MechanismEvidence = []architecturereview.EvidenceRef{{ID: "mechanism", Kind: "mechanism", Digest: "sha256:mechanism"}}
		req.PolicyEvidence = []architecturereview.EvidenceRef{{ID: "policy", Kind: "policy", Digest: "sha256:policy"}}
	}
	receipt, err := goals.ReviewBaselineOwnership(b, req)
	if err != nil {
		t.Fatal(err)
	}
	return b, receipt
}

func TestFinalizeEphemeralRequiresExactResolvedReceiptWithoutStore(t *testing.T) {
	b, receipt := finalizedFixture(t, false)
	result, err := (Repository{}).Finalize(context.Background(), FinalizeRequest{Baseline: b, Review: receipt, Persist: false})
	if err != nil {
		t.Fatal(err)
	}
	if result.Persisted || result.Baseline.Digest != b.Digest {
		t.Fatalf("unexpected ephemeral result: %+v", result)
	}
}

func TestFinalizeDurableRequiresReceiptAndStore(t *testing.T) {
	b, receipt := finalizedFixture(t, false)
	if _, err := (Repository{}).Finalize(context.Background(), FinalizeRequest{Baseline: b, Review: receipt, Persist: true, CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("durable request without persistence boundary must fail")
	}
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	result, err := repo.Finalize(context.Background(), FinalizeRequest{Baseline: b, Review: receipt, Persist: true, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Persisted || result.Baseline.Digest != b.Digest {
		t.Fatalf("reviewed durable candidate was not persisted: %+v", result)
	}
}

func TestFinalizeRejectsAbsentUnresolvedStaleAndMutatedReview(t *testing.T) {
	b, receipt := finalizedFixture(t, false)
	r := Repository{}
	if _, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: b}); err == nil {
		t.Fatal("absent review receipt was accepted")
	}

	reviewCandidate, unresolved := finalizedFixture(t, true)
	if _, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: reviewCandidate, Review: unresolved}); !errors.Is(err, goals.ErrUnresolvedBaselineReview) {
		t.Fatalf("unresolved review was accepted: %v", err)
	}

	stale := receipt
	stale.BaselineDigest = "sha256:stale"
	if _, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: b, Review: stale}); !errors.Is(err, goals.ErrBaselineReviewEvidence) {
		t.Fatalf("stale review was accepted: %v", err)
	}

	b.RefinedOutcome = "changed after review"
	if _, err := r.Finalize(context.Background(), FinalizeRequest{Baseline: b, Review: receipt}); !errors.Is(err, goals.ErrBaselineDigestMismatch) {
		t.Fatalf("post-review mutation was accepted: %v", err)
	}
}

func TestFinalizeAcceptsExplicitReviewResolution(t *testing.T) {
	b, receipt := finalizedFixture(t, true)
	resolved, err := goals.ResolveBaselineReview(b, receipt, "decision:architecture-review/1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Repository{}).Finalize(context.Background(), FinalizeRequest{Baseline: b, Review: resolved}); err != nil {
		t.Fatal(err)
	}
}
