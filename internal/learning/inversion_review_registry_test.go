package learning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	candidate, err := NewBehaviorGeneration(seed.ID, []AdvisoryInstruction{{ID: "instruction", Text: "retain evidence", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	record, err := ReviewCandidateOwnership(candidate.ID, "sha256:blind", inversionReviewRequest())
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
	got, ok := restarted.InversionReview(candidate.ID)
	if !ok || got.BlindDerivationDigest != "sha256:blind" || got.Review.Kind != architecturereview.UniversalMechanism {
		t.Fatalf("review evidence did not survive restart: %#v ok=%v", got, ok)
	}
	if err := restarted.RecordInversionReview(record); err != nil {
		t.Fatalf("identical review should be idempotent: %v", err)
	}
}

func TestBehaviorRegistryRejectsOrphanAndConflictingInversionReviews(t *testing.T) {
	seed, err := NewBehaviorGeneration("", []AdvisoryInstruction{{ID: "instruction", Text: "retain evidence", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := OpenBehaviorRegistry(filepath.Join(t.TempDir(), "learning.json"), seed)
	if err != nil {
		t.Fatal(err)
	}
	record, err := ReviewCandidateOwnership("missing-candidate", "sha256:blind", inversionReviewRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordInversionReview(record); err == nil {
		t.Fatal("orphan inversion review was persisted")
	}

	candidate, err := NewBehaviorGeneration(seed.ID, []AdvisoryInstruction{{ID: "instruction", Text: "retain evidence", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	record, err = ReviewCandidateOwnership(candidate.ID, "sha256:blind", inversionReviewRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordInversionReview(record); err != nil {
		t.Fatal(err)
	}
	conflict := record
	conflict.BlindDerivationDigest = "sha256:different"
	conflict, err = freezeInversionReviewRecord(conflict)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordInversionReview(conflict); err == nil {
		t.Fatal("conflicting inversion review replacement was accepted")
	}
	got, ok := registry.InversionReview(candidate.ID)
	if !ok || got.BlindDerivationDigest != record.BlindDerivationDigest {
		t.Fatalf("conflict changed original inversion review: %#v ok=%v", got, ok)
	}
}

func TestBehaviorRegistryRejectsTamperedPersistedInversionReview(t *testing.T) {
	seed, err := NewBehaviorGeneration("", []AdvisoryInstruction{{ID: "instruction", Text: "retain evidence", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "learning.json")
	registry, err := OpenBehaviorRegistry(path, seed)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := NewBehaviorGeneration(seed.ID, []AdvisoryInstruction{{ID: "instruction", Text: "retain evidence", Tier: TierD2, Learnable: true, Active: true}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(candidate); err != nil {
		t.Fatal(err)
	}
	record, err := ReviewCandidateOwnership(candidate.ID, "sha256:blind", inversionReviewRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.RecordInversionReview(record); err != nil {
		t.Fatal(err)
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot behaviorSnapshot
	if err := json.Unmarshal(bytes, &snapshot); err != nil {
		t.Fatal(err)
	}
	tampered := snapshot.InversionReviews[candidate.ID]
	tampered.Review.Reasons = append([]string(nil), tampered.Review.Reasons...)
	tampered.Review.Reasons[0] = "different but structurally valid advisory reason"
	snapshot.InversionReviews[candidate.ID] = tampered
	bytes, err = json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(bytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenBehaviorRegistry(path, seed); err == nil {
		t.Fatal("tampered persisted inversion review was accepted")
	} else if !strings.Contains(err.Error(), "content digest mismatch") {
		t.Fatalf("tampered persisted inversion review returned wrong error: %v", err)
	}
}

func inversionReviewRequest() architecturereview.Request {
	return architecturereview.Request{
		Capability: "handoff", ProposedOwner: "core", ReusableAcrossScopes: true,
		GoalEvidence:      []architecturereview.EvidenceRef{{ID: "goal", Kind: "goal", Digest: "sha256:goal"}},
		InvariantEvidence: []architecturereview.EvidenceRef{{ID: "invariant", Kind: "invariant", Digest: "sha256:invariant"}},
		MechanismEvidence: []architecturereview.EvidenceRef{{ID: "mechanism", Kind: "mechanism", Digest: "sha256:mechanism"}},
		PolicyEvidence:    []architecturereview.EvidenceRef{{ID: "policy", Kind: "policy", Digest: "sha256:policy"}},
	}
}
