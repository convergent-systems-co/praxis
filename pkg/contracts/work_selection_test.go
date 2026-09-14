package contracts

import (
	"errors"
	"testing"
)

func candidate(id string, priority, sequence int, completed bool) WorkCandidate {
	return WorkCandidate{ID: id, Priority: priority, Sequence: sequence, Completed: completed, SourceRef: "docs/PLAN/003-post-release-roadmap.md#" + id, SourceDigest: "sha256:roadmap", Provenance: ProvenancePLAN}
}

func TestSelectRunnableWorkUsesAuthoritativeReadinessAndStableOrder(t *testing.T) {
	selected, err := SelectRunnableWork([]WorkCandidate{candidate("ready-late", 2, 2, false), candidate("done", 0, 0, true), candidate("ready-first", 1, 3, false)}, []WorkRelationship{{Dependent: "ready-late", Prerequisite: "done", Kind: RelationshipHardDependency, SourceRef: "docs/PLAN/003-post-release-roadmap.md", SourceDigest: "sha256:roadmap", Provenance: ProvenancePLAN}})
	if err != nil || selected.ID != "ready-first" {
		t.Fatalf("unexpected deterministic selection: %+v err=%v", selected, err)
	}
}

func TestSelectRunnableWorkFailsClosedForAmbiguousOrInferredAuthority(t *testing.T) {
	_, err := SelectRunnableWork([]WorkCandidate{candidate("a", 1, 1, false), candidate("b", 1, 1, false)}, nil)
	if !errors.Is(err, ErrAmbiguousWorkChoice) {
		t.Fatalf("equal runnable candidates must fail closed: %v", err)
	}
	inferred := candidate("model", 0, 0, false)
	inferred.Provenance = ProvenanceModelProposal
	if _, err := SelectRunnableWork([]WorkCandidate{inferred}, nil); !errors.Is(err, ErrInferredWorkSelection) {
		t.Fatalf("model-proposed candidate must not mint selection authority: %v", err)
	}
}

func TestSelectRunnableWorkReportsNoRunnableCandidate(t *testing.T) {
	_, err := SelectRunnableWork([]WorkCandidate{candidate("blocked", 0, 0, false)}, []WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: RelationshipHardDependency, SourceRef: "docs/PLAN/003-post-release-roadmap.md", SourceDigest: "sha256:roadmap", Provenance: ProvenancePLAN}})
	if !errors.Is(err, ErrNoRunnableWork) {
		t.Fatalf("blocked work must not be selected: %v", err)
	}
}
