package contracts

import (
	"errors"
	"testing"
)

func relationship(kind WorkRelationshipKind, provenance RelationshipProvenance) WorkRelationship {
	return WorkRelationship{Dependent: "#101", Prerequisite: "#102", Kind: kind, SourceRef: "docs/PLAN/003-post-release-roadmap.md#101", SourceDigest: "sha256:roadmap", Provenance: provenance}
}

func TestOnlyAuthoritativeHardDependenciesBlockReadiness(t *testing.T) {
	for _, kind := range []WorkRelationshipKind{RelationshipConsumer, RelationshipInteraction, RelationshipAdvisory} {
		readiness, blocked, err := EvaluateWorkReadiness("#101", []WorkRelationship{relationship(kind, ProvenancePLAN)}, map[string]bool{})
		if err != nil || readiness != WorkReady || len(blocked) != 0 {
			t.Fatalf("%s incorrectly blocked work: %s %v %v", kind, readiness, blocked, err)
		}
	}
	readiness, blocked, err := EvaluateWorkReadiness("#101", []WorkRelationship{relationship(RelationshipHardDependency, ProvenancePLAN)}, map[string]bool{})
	if err != nil || readiness != WorkBlocked || len(blocked) != 1 || blocked[0] != "#102" {
		t.Fatalf("hard dependency did not block: %s %v %v", readiness, blocked, err)
	}
	readiness, blocked, err = EvaluateWorkReadiness("#101", []WorkRelationship{relationship(RelationshipHardDependency, ProvenancePLAN)}, map[string]bool{"#102": true})
	if err != nil || readiness != WorkReady || len(blocked) != 0 {
		t.Fatalf("completed prerequisite remained blocking: %s %v %v", readiness, blocked, err)
	}
}

func TestModelCannotMintBlockingDependency(t *testing.T) {
	_, _, err := EvaluateWorkReadiness("#101", []WorkRelationship{relationship(RelationshipHardDependency, ProvenanceModelProposal)}, map[string]bool{})
	if !errors.Is(err, ErrInferredHardDependency) {
		t.Fatalf("model-derived hard dependency was accepted: %v", err)
	}
}

func TestRelationshipRequiresDurableProvenance(t *testing.T) {
	invalid := relationship(RelationshipInteraction, ProvenancePLAN)
	invalid.SourceDigest = ""
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidWorkRelationship) {
		t.Fatalf("unbound relationship did not fail closed: %v", err)
	}
}
