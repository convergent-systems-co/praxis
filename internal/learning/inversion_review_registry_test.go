package learning

import (
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/architecturereview"
)

func TestBehaviorRegistryPersistsAdvisoryInversionReviewAcrossRestart(t *testing.T) {
	seed, err := NewBehaviorGeneration("", []AdvisoryInstruction{{ID: "instruction", Text: "retain evidence", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "learning.json")
	registry, err := OpenBehaviorRegistry(path, seed)
	if err != nil {
		t.Fatal(err)
	}
	record, err := ReviewCandidateOwnership("candidate-1", "sha256:blind", architecturereview.Request{
		Capability: "handoff", ProposedOwner: "core", ReusableAcrossScopes: true,
		GoalEvidence:      []architecturereview.EvidenceRef{{ID: "goal", Kind: "goal", Digest: "sha256:goal"}},
		InvariantEvidence: []architecturereview.EvidenceRef{{ID: "invariant", Kind: "invariant", Digest: "sha256:invariant"}},
		MechanismEvidence: []architecturereview.EvidenceRef{{ID: "mechanism", Kind: "mechanism", Digest: "sha256:mechanism"}},
		PolicyEvidence:    []architecturereview.EvidenceRef{{ID: "policy", Kind: "policy", Digest: "sha256:policy"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordInversionReview(record); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenBehaviorRegistry(path, seed)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := restarted.InversionReview("candidate-1")
	if !ok || got.BlindDerivationDigest != "sha256:blind" || got.Review.Kind != architecturereview.UniversalMechanism {
		t.Fatalf("review evidence did not survive restart: %#v ok=%v", got, ok)
	}
	if err := restarted.RecordInversionReview(record); err != nil {
		t.Fatalf("identical review should be idempotent: %v", err)
	}
}
