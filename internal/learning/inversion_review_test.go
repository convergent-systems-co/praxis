package learning

import (
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

func TestReviewCandidateOwnershipPreservesBlindBoundary(t *testing.T) {
	req := architecturereview.Request{
		Capability: "handoff", ProposedOwner: "core", ReusableAcrossScopes: true,
		GoalEvidence:      []architecturereview.EvidenceRef{{ID: "goal", Kind: "goal", Digest: "sha256:goal"}},
		InvariantEvidence: []architecturereview.EvidenceRef{{ID: "invariant", Kind: "invariant", Digest: "sha256:invariant"}},
		MechanismEvidence: []architecturereview.EvidenceRef{{ID: "mechanism", Kind: "mechanism", Digest: "sha256:mechanism"}},
		PolicyEvidence:    []architecturereview.EvidenceRef{{ID: "policy", Kind: "policy", Digest: "sha256:policy"}},
	}
	record, err := ReviewCandidateOwnership("candidate-1", "sha256:blind", req)
	if err != nil || record.ID == "" || record.Review.Kind != architecturereview.UniversalMechanism || record.BlindDerivationDigest != "sha256:blind" {
		t.Fatalf("unexpected review record: %#v, %v", record, err)
	}
	if err := validateInversionReviewRecord(record); err != nil {
		t.Fatalf("content-addressed review record did not verify: %v", err)
	}
	tampered := record
	tampered.Review.Reasons = append([]string(nil), record.Review.Reasons...)
	tampered.Review.Reasons[0] = "changed but structurally valid reason"
	if err := validateInversionReviewRecord(tampered); err == nil {
		t.Fatal("valid-shape review content tampering retained the original identity")
	}
	if _, err := ReviewCandidateOwnership("candidate-1", "", req); !errors.Is(err, ErrMissingBlindDerivation) {
		t.Fatalf("missing blind digest did not fail closed: %v", err)
	}
}
