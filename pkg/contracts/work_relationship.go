package contracts

import (
	"crypto/sha256"
	"encoding/hex"
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
	ProvenanceADR               RelationshipProvenance = "adr"
	ProvenanceSPEC              RelationshipProvenance = "spec"
	ProvenancePLAN              RelationshipProvenance = "plan"
	ProvenanceContract          RelationshipProvenance = "contract"
	ProvenanceIssue             RelationshipProvenance = "issue"
	ProvenanceModelProposal     RelationshipProvenance = "model_proposal"
	ProvenanceModelGateProposal RelationshipProvenance = "model_gate_proposal"
	ProvenanceAuthorityGate     RelationshipProvenance = "authority_gate"
)

// WorkRelationship is a durable, provenance-bearing edge between work units.
// Dependent requires Prerequisite only when Kind is an authoritative hard
// dependency; every other kind is useful context but cannot block readiness.
type WorkRelationship struct {
	Dependent     string                 `json:"dependent"`
	Prerequisite  string                 `json:"prerequisite"`
	Kind          WorkRelationshipKind   `json:"kind"`
	SourceRef     string                 `json:"source_ref"`
	SourceDigest  string                 `json:"source_digest"`
	Provenance    RelationshipProvenance `json:"provenance"`
	Specification []byte                 `json:"specification,omitempty"`
	Rationale     string                 `json:"rationale,omitempty"`
}

// ValidateSpecification validates a proposal-time relationship exactly.
func (r WorkRelationship) ValidateSpecification() error { return r.validateSpecification(false) }

// ValidateAcceptedSpecification validates a relationship of an accepted plan,
// admitting only the provenance rewrite that acceptance performs.
func (r WorkRelationship) ValidateAcceptedSpecification() error { return r.validateSpecification(true) }

func (r WorkRelationship) validateSpecification(accepted bool) error {
	if len(r.Specification) == 0 {
		return fmt.Errorf("relationship %q -> %q has no preserved specification bytes", r.Dependent, r.Prerequisite)
	}
	sum := sha256.Sum256(r.Specification)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != r.SourceDigest {
		return fmt.Errorf("relationship %q -> %q specification digest mismatch: got %s want %s", r.Dependent, r.Prerequisite, got, r.SourceDigest)
	}
	var spec struct {
		Dependent    string                 `json:"dependent"`
		Prerequisite string                 `json:"prerequisite"`
		Kind         WorkRelationshipKind   `json:"kind"`
		Provenance   RelationshipProvenance `json:"provenance"`
		Rationale    string                 `json:"rationale"`
	}
	if err := UnmarshalExactJSON(r.Specification, &spec, false); err != nil {
		return fmt.Errorf("relationship specification is not valid JSON: %w", err)
	}
	if spec.Dependent != r.Dependent || spec.Prerequisite != r.Prerequisite || spec.Kind != r.Kind || !specificationProvenanceMatches(spec.Provenance, r.Provenance, accepted) || spec.Rationale != r.Rationale || r.Rationale == "" {
		return errors.New("relationship specification content does not match executable record")
	}
	return nil
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
