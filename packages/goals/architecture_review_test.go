package goals

import (
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

func reviewedBaselineFixture(t *testing.T, id, version, predecessor string) GoalBaseline {
	t.Helper()
	b := GoalBaseline{ID: id, Version: version, PredecessorDigest: predecessor, OriginalIntent: "review architecture", RefinedOutcome: "bound review", Rigor: RigorRigorous, RecommendationMode: RecommendationReviewAll}
	digest, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	b.Digest = digest
	return b
}

func reviewRequestFixture(b GoalBaseline, requiresReview bool) architecturereview.Request {
	req := architecturereview.Request{
		Capability: "handoff", ProposedOwner: "core", ReusableAcrossScopes: true,
		GoalEvidence:      []architecturereview.EvidenceRef{{ID: "baseline:" + b.ID + "@" + b.Version, Kind: "goal_baseline", Digest: b.Digest}},
		InvariantEvidence: []architecturereview.EvidenceRef{{ID: "invariant", Kind: "invariant", Digest: "sha256:invariant"}},
	}
	if !requiresReview {
		req.MechanismEvidence = []architecturereview.EvidenceRef{{ID: "mechanism", Kind: "mechanism", Digest: "sha256:mechanism"}}
		req.PolicyEvidence = []architecturereview.EvidenceRef{{ID: "policy", Kind: "policy", Digest: "sha256:policy"}}
	}
	return req
}

func reviewReceiptFixture(t *testing.T, b GoalBaseline, requiresReview bool) BaselineReviewReceipt {
	t.Helper()
	receipt, err := ReviewBaselineOwnership(b, reviewRequestFixture(b, requiresReview))
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestReviewBaselineOwnershipContentBindsExactCandidate(t *testing.T) {
	b := reviewedBaselineFixture(t, "goal-96", "1", "")
	receipt := reviewReceiptFixture(t, b, false)
	if receipt.ID == "" || receipt.BaselineDigest != b.Digest {
		t.Fatalf("review was not content-bound: %+v", receipt)
	}
	if err := VerifyBaselineReviewReceipt(b, receipt, true); err != nil {
		t.Fatal(err)
	}

	stale := reviewRequestFixture(b, false)
	stale.GoalEvidence[0].Digest = "sha256:stale"
	if _, err := ReviewBaselineOwnership(b, stale); !errors.Is(err, ErrBaselineReviewEvidence) {
		t.Fatalf("stale candidate evidence did not fail closed: %v", err)
	}

	tampered := receipt
	tampered.Result.Reasons = append([]string(nil), receipt.Result.Reasons...)
	tampered.Result.Reasons[0] = "tampered"
	if err := VerifyBaselineReviewReceipt(b, tampered, true); !errors.Is(err, ErrBaselineReviewReceipt) {
		t.Fatalf("tampered receipt did not fail closed: %v", err)
	}
}

func TestReviewReceiptRejectsPostReviewMutationAndPredecessorBinding(t *testing.T) {
	predecessor := reviewedBaselineFixture(t, "goal-96", "1", "")
	successor := reviewedBaselineFixture(t, "goal-96", "2", predecessor.Digest)
	receipt := reviewReceiptFixture(t, successor, false)

	mutated := successor
	mutated.RefinedOutcome = "changed after review"
	if err := VerifyBaselineReviewReceipt(mutated, receipt, true); !errors.Is(err, ErrBaselineDigestMismatch) {
		t.Fatalf("post-review mutation was accepted: %v", err)
	}

	predecessorReceipt := reviewReceiptFixture(t, predecessor, false)
	if err := VerifyBaselineReviewReceipt(successor, predecessorReceipt, true); !errors.Is(err, ErrBaselineReviewEvidence) {
		t.Fatalf("predecessor receipt was accepted for successor: %v", err)
	}
	req := reviewRequestFixture(successor, false)
	req.GoalEvidence[0] = architecturereview.EvidenceRef{ID: "baseline:" + predecessor.ID + "@" + predecessor.Version, Kind: "goal_baseline", Digest: predecessor.Digest}
	if _, err := ReviewBaselineOwnership(successor, req); !errors.Is(err, ErrBaselineReviewEvidence) {
		t.Fatalf("predecessor evidence became candidate binding: %v", err)
	}
}

func TestReviewRequiredReceiptMustBeExplicitlyResolved(t *testing.T) {
	b := reviewedBaselineFixture(t, "goal-review", "1", "")
	receipt := reviewReceiptFixture(t, b, true)
	if err := VerifyBaselineReviewReceipt(b, receipt, true); !errors.Is(err, ErrUnresolvedBaselineReview) {
		t.Fatalf("unresolved review was accepted: %v", err)
	}
	resolved, err := ResolveBaselineReview(b, receipt, "decision:architecture-review/1")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID == receipt.ID || resolved.ResolutionEvidence == "" {
		t.Fatalf("resolution did not produce new content identity: %+v", resolved)
	}
	if err := VerifyBaselineReviewReceipt(b, resolved, true); err != nil {
		t.Fatal(err)
	}
}
