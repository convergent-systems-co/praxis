package goals

import (
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

func TestReviewBaselineOwnershipRequiresExactPersistedDigest(t *testing.T) {
	b := GoalBaseline{ID: "goal-96", Version: "1", OriginalIntent: "review architecture", RefinedOutcome: "bound review", Rigor: RigorRigorous, RecommendationMode: RecommendationReviewAll}
	digest, err := b.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	req := architecturereview.Request{
		Capability: "handoff", ProposedOwner: "core", ReusableAcrossScopes: true,
		GoalEvidence:      []architecturereview.EvidenceRef{{ID: "baseline:goal-96@1", Kind: "goal_baseline", Digest: digest}},
		InvariantEvidence: []architecturereview.EvidenceRef{{ID: "invariant", Kind: "invariant", Digest: "sha256:invariant"}},
		MechanismEvidence: []architecturereview.EvidenceRef{{ID: "mechanism", Kind: "mechanism", Digest: "sha256:mechanism"}},
		PolicyEvidence:    []architecturereview.EvidenceRef{{ID: "policy", Kind: "policy", Digest: "sha256:policy"}},
	}
	b.Digest = digest
	if _, err := ReviewBaselineOwnership(b, req); err != nil {
		t.Fatalf("exact baseline evidence rejected: %v", err)
	}
	req.GoalEvidence[0].Digest = "sha256:stale"
	if _, err := ReviewBaselineOwnership(b, req); !errors.Is(err, ErrBaselineReviewEvidence) {
		t.Fatalf("stale baseline evidence did not fail closed: %v", err)
	}
}
