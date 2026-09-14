package architecturereview

import (
	"errors"
	"testing"
)

func evidence(id, kind string) EvidenceRef {
	return EvidenceRef{ID: id, Kind: kind, Digest: "sha256:" + id}
}

func base() Request {
	return Request{
		Capability:                     "handoff",
		ProposedOwner:                  "core",
		GoalEvidence:                   []EvidenceRef{evidence("goal", "goal")},
		InvariantEvidence:              []EvidenceRef{evidence("invariant", "invariant")},
		ImplementationLocationEvidence: []EvidenceRef{evidence("location", "implementation")},
	}
}

func TestReviewIdentifiesUniversalMechanismWithoutGrantingAuthority(t *testing.T) {
	req := base()
	req.ReusableAcrossScopes = true
	req.MechanismEvidence = []EvidenceRef{evidence("mechanism", "mechanism")}
	req.PolicyEvidence = []EvidenceRef{evidence("policy", "policy")}
	got, err := Review(req)
	if err != nil || got.Kind != UniversalMechanism || !got.AdvisoryOnly {
		t.Fatalf("unexpected universal result: %#v, %v", got, err)
	}
}

func TestReviewRetainsGenuinelyDomainSpecificCounterexample(t *testing.T) {
	req := base()
	req.DomainSpecific = true
	req.Scope = "research-protocol"
	req.CounterexampleEvidence = []EvidenceRef{evidence("counterexample", "counterexample")}
	got, err := Review(req)
	if err != nil || got.Kind != DomainSpecific {
		t.Fatalf("unexpected domain-specific result: %#v, %v", got, err)
	}
}

func TestReviewDoesNotInferOwnershipFromImplementationLocation(t *testing.T) {
	req := base()
	req.ReusableAcrossScopes = true
	got, err := Review(req)
	if err != nil || got.Kind != ReviewRequired {
		t.Fatalf("location-only universal claim was not held for review: %#v, %v", got, err)
	}
}

func TestReviewFailsClosedForMissingAndDuplicateEvidence(t *testing.T) {
	req := base()
	req.ReusableAcrossScopes = true
	req.GoalEvidence = nil
	if _, err := Review(req); !errors.Is(err, ErrMissingEvidence) {
		t.Fatalf("missing evidence did not fail closed: %v", err)
	}
	req = base()
	req.ReusableAcrossScopes = true
	req.MechanismEvidence = []EvidenceRef{evidence("same", "mechanism")}
	req.PolicyEvidence = []EvidenceRef{evidence("same", "policy")}
	if _, err := Review(req); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("duplicate evidence did not fail closed: %v", err)
	}
}

func TestReviewRejectsUnclassifiedClaim(t *testing.T) {
	if _, err := Review(base()); !errors.Is(err, ErrUnknownClaim) {
		t.Fatalf("unclassified claim did not fail closed: %v", err)
	}
}

func TestReviewRejectsAmbiguousClaim(t *testing.T) {
	req := base()
	req.ReusableAcrossScopes = true
	req.DomainSpecific = true
	if _, err := Review(req); !errors.Is(err, ErrAmbiguousClaim) {
		t.Fatalf("ambiguous claim did not fail closed: %v", err)
	}
}
