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

func TestAssessWorkCandidatesPreservesPartialBlockersAndParentState(t *testing.T) {
	assessment, err := AssessWorkCandidates([]WorkCandidate{candidate("blocked", 0, 0, false), candidate("ready", 1, 0, false)}, []WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: RelationshipHardDependency, SourceRef: "docs/PLAN/003-post-release-roadmap.md", SourceDigest: "sha256:roadmap", Provenance: ProvenancePLAN}})
	if err != nil || assessment.State != WorkSetRunnable || assessment.Selected == nil || assessment.Selected.ID != "ready" {
		t.Fatalf("partial blocker incorrectly changed parent readiness: %+v err=%v", assessment, err)
	}
	if len(assessment.Candidates) != 2 || assessment.Candidates[0].Readiness != WorkBlocked || len(assessment.Candidates[0].BlockedBy) != 1 {
		t.Fatalf("child blocker evidence was not preserved: %+v", assessment.Candidates)
	}

	blocked, err := AssessWorkCandidates([]WorkCandidate{candidate("blocked", 0, 0, false)}, []WorkRelationship{{Dependent: "blocked", Prerequisite: "missing", Kind: RelationshipHardDependency, SourceRef: "docs/PLAN/003-post-release-roadmap.md", SourceDigest: "sha256:roadmap", Provenance: ProvenancePLAN}})
	if err != nil || blocked.State != WorkSetBlocked || blocked.Selected != nil {
		t.Fatalf("all-blocked parent state mismatch: %+v err=%v", blocked, err)
	}
	complete, err := AssessWorkCandidates([]WorkCandidate{candidate("done", 0, 0, true)}, nil)
	if err != nil || complete.State != WorkSetComplete || complete.Selected != nil {
		t.Fatalf("all-complete parent state mismatch: %+v err=%v", complete, err)
	}
}
