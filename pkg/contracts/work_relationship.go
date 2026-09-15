package contracts

import (
	"errors"
	"fmt"
)

type WorkRelationshipKind string

const (
	RelationshipHardDependency WorkRelationshipKind = "hard_dependency"
	RelationshipConsumer       WorkRelationshipKind = "consumer"
	RelationshipInteraction    WorkRelationshipKind = "interaction"
	RelationshipAdvisory       WorkRelationshipKind = "advisory"
)

type RelationshipProvenance string

const (
	ProvenanceADR           RelationshipProvenance = "adr"
	ProvenanceSPEC          RelationshipProvenance = "spec"
	ProvenancePLAN          RelationshipProvenance = "plan"
	ProvenanceContract      RelationshipProvenance = "contract"
	ProvenanceIssue         RelationshipProvenance = "issue"
	ProvenanceModelProposal RelationshipProvenance = "model_proposal"
)

// WorkRelationship is a durable, provenance-bearing edge between work units.
// Dependent requires Prerequisite only when Kind is an authoritative hard
// dependency; every other kind is useful context but cannot block readiness.
type WorkRelationship struct {
	Dependent    string                 `json:"dependent"`
	Prerequisite string                 `json:"prerequisite"`
	Kind         WorkRelationshipKind   `json:"kind"`
	SourceRef    string                 `json:"source_ref"`
	SourceDigest string                 `json:"source_digest"`
	Provenance   RelationshipProvenance `json:"provenance"`
}

type WorkReadiness string

const (
	WorkReady   WorkReadiness = "ready"
	WorkBlocked WorkReadiness = "blocked"
)

var (
	ErrInvalidWorkRelationship = errors.New("invalid work relationship")
	ErrInferredHardDependency  = errors.New("model-derived relationship cannot block work")
)

func (r WorkRelationship) Validate() error {
	if r.Dependent == "" || r.Prerequisite == "" || r.Dependent == r.Prerequisite || r.SourceRef == "" || r.SourceDigest == "" {
		return fmt.Errorf("%w: identities and source provenance are required", ErrInvalidWorkRelationship)
	}
	switch r.Kind {
	case RelationshipHardDependency, RelationshipConsumer, RelationshipInteraction, RelationshipAdvisory:
	default:
		return fmt.Errorf("%w: unknown relationship kind %q", ErrInvalidWorkRelationship, r.Kind)
	}
	switch r.Provenance {
	case ProvenanceADR, ProvenanceSPEC, ProvenancePLAN, ProvenanceContract, ProvenanceIssue, ProvenanceModelProposal:
	default:
		return fmt.Errorf("%w: unknown relationship provenance %q", ErrInvalidWorkRelationship, r.Provenance)
	}
	if r.Kind == RelationshipHardDependency && r.Provenance == ProvenanceModelProposal {
		return ErrInferredHardDependency
	}
	return nil
}

func EvaluateWorkReadiness(workID string, relationships []WorkRelationship, completed map[string]bool) (WorkReadiness, []string, error) {
	if workID == "" {
		return "", nil, fmt.Errorf("%w: work identity is required", ErrInvalidWorkRelationship)
	}
	blockedBy := []string{}
	for _, relationship := range relationships {
		if err := relationship.Validate(); err != nil {
			return "", nil, err
		}
		if relationship.Dependent == workID && relationship.Kind == RelationshipHardDependency && !completed[relationship.Prerequisite] {
			blockedBy = append(blockedBy, relationship.Prerequisite)
		}
	}
	if len(blockedBy) > 0 {
		return WorkBlocked, blockedBy, nil
	}
	return WorkReady, blockedBy, nil
}
