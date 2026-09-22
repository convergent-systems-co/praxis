package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// WorkPlanSafetyKernelVersion is intentionally unknown to pre-bootstrap
// binaries. A v4 proposal uses model_gate_proposal and an accepted plan uses
// authority_gate, so older validators reject both before selection.
const WorkPlanSafetyKernelVersion = "praxis-human-interface-bootstrap-v4/1"

// WorkPlanSafetyBinding makes the pre-v4 execution prerequisites part of the
// proposal and accepted-plan digest. The referenced activation manifest is
// verified against the running process at every mutating lifecycle boundary
// and before Goal-drive admission.
type WorkPlanSafetyBinding struct {
	KernelVersion             string `json:"kernel_version"`
	ActivationManifestDigest  string `json:"activation_manifest_digest"`
	ValidationProfileDigest   string `json:"validation_profile_digest"`
	SpecificationBundleDigest string `json:"specification_bundle_digest"`
}

// ErrGateAuthorityNotEffective marks a completed authority gate whose recorded
// decision is authentic historical evidence but is no longer current authority
// (revoked, expired, or issued by a superseded root). The completion stays
// readable; it may no longer authorize new work.
var ErrGateAuthorityNotEffective = errors.New("authority gate decision is authentic but no longer effective")

// ErrCompletionUnauthenticated marks a completion, presented as evidence that a
// unit or gate finished, whose authenticated attestation or governing lineage
// cannot be resolved. It is never demoted to "historical": it is refused.
var ErrCompletionUnauthenticated = errors.New("completion is not authenticated by the installation's governed store")

// ErrKernelShapedWithoutSafety rejects a plan whose content only exists in the
// safety kernel's vocabulary (explicit candidate kinds, preserved specification
// bytes, gate provenance) but which does not carry the safety binding. Such a
// plan is either a downgrade attempt or malformed; it can never be a legacy
// plan because legacy candidates carry none of these fields.
var ErrKernelShapedWithoutSafety = errors.New("plan carries safety-kernel content but no safety binding")

// RejectKernelShapedWithoutSafety is the content half of downgrade resistance
// (I9). The classification half is durable Goal identity in the GoalStore.
func RejectKernelShapedWithoutSafety(candidates []WorkCandidate, relationships []WorkRelationship) error {
	for _, candidate := range candidates {
		if candidate.Kind != "" || len(candidate.Specification) > 0 || candidate.Provenance == ProvenanceAuthorityGate || candidate.Provenance == ProvenanceModelGateProposal {
			return fmt.Errorf("%w: candidate %q", ErrKernelShapedWithoutSafety, candidate.ID)
		}
	}
	for _, relationship := range relationships {
		if len(relationship.Specification) > 0 {
			return fmt.Errorf("%w: relationship %q -> %q", ErrKernelShapedWithoutSafety, relationship.Dependent, relationship.Prerequisite)
		}
	}
	return nil
}

func ComputeSpecificationBundleDigest(candidates []WorkCandidate, relationships []WorkRelationship) (string, error) {
	type record struct {
		Kind   string `json:"kind"`
		Key    string `json:"key"`
		Digest string `json:"digest"`
		Bytes  []byte `json:"bytes"`
	}
	var records []record
	requirements := map[string]record{}
	for _, candidate := range candidates {
		records = append(records, record{Kind: "candidate", Key: candidate.ID, Digest: candidate.SourceDigest, Bytes: candidate.Specification})
		for _, requirement := range candidate.Requirements {
			item := record{Kind: "requirement", Key: requirement.ID, Digest: requirement.SourceDigest, Bytes: requirement.Specification}
			if prior, ok := requirements[requirement.ID]; ok && (prior.Digest != item.Digest || string(prior.Bytes) != string(item.Bytes)) {
				return "", fmt.Errorf("conflicting requirement specification %q", requirement.ID)
			}
			requirements[requirement.ID] = item
		}
	}
	for _, item := range requirements {
		records = append(records, item)
	}
	for _, relationship := range relationships {
		records = append(records, record{Kind: "relationship", Key: relationship.Dependent + "\x00" + relationship.Prerequisite, Digest: relationship.SourceDigest, Bytes: relationship.Specification})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		return records[i].Key < records[j].Key
	})
	payload, err := json.Marshal(records)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (b WorkPlanSafetyBinding) Validate() error {
	if b.KernelVersion != WorkPlanSafetyKernelVersion {
		return fmt.Errorf("unsupported work-plan safety kernel %q", b.KernelVersion)
	}
	for name, digest := range map[string]string{
		"activation manifest":  b.ActivationManifestDigest,
		"validation profile":   b.ValidationProfileDigest,
		"specification bundle": b.SpecificationBundleDigest,
	} {
		if err := ValidateSHA256Digest(digest); err != nil {
			return fmt.Errorf("%s digest: %w", name, err)
		}
	}
	return nil
}

func validateSafetyGraph(candidates []WorkCandidate, relationships []WorkRelationship, proposal bool) error {
	sequence := map[int]string{}
	ids := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate.Kind != WorkCandidateOrdinary && candidate.Kind != WorkCandidateAuthorityGate {
			return fmt.Errorf("candidate %q has unknown or missing kind %q", candidate.ID, candidate.Kind)
		}
		// Completion is execution-derived state. Proposal, acceptance, import,
		// and attachment bytes can never supply it; it exists only as an
		// overlay of validated generation-specific ledger evidence.
		if candidate.Completed {
			return fmt.Errorf("candidate %q supplies execution-derived completed state; completion derives only from qualified ledger evidence", candidate.ID)
		}
		if candidate.Priority <= 0 || candidate.Sequence <= 0 {
			return fmt.Errorf("candidate %q priority and sequence must be positive", candidate.ID)
		}
		if prior := sequence[candidate.Sequence]; prior != "" {
			return fmt.Errorf("candidates %q and %q duplicate sequence %d", prior, candidate.ID, candidate.Sequence)
		}
		sequence[candidate.Sequence] = candidate.ID
		specErr := candidate.ValidateSpecification()
		if !proposal {
			specErr = candidate.ValidateAcceptedSpecification()
		}
		if specErr != nil {
			return specErr
		}
		requirements := map[string]struct{}{}
		for _, requirement := range candidate.Requirements {
			if err := requirement.ValidateSpecification(); err != nil {
				return err
			}
			if _, duplicate := requirements[requirement.ID]; duplicate {
				return fmt.Errorf("candidate %q duplicates requirement %q", candidate.ID, requirement.ID)
			}
			requirements[requirement.ID] = struct{}{}
		}
		if proposal && candidate.Kind == WorkCandidateAuthorityGate && candidate.Provenance != ProvenanceModelGateProposal {
			return fmt.Errorf("authority gate %q lacks model_gate_proposal provenance", candidate.ID)
		}
		ids[candidate.ID] = struct{}{}
	}
	endpoints := map[string]struct{}{}
	adjacency := map[string][]string{}
	for _, relationship := range relationships {
		relationshipErr := relationship.ValidateSpecification()
		if !proposal {
			relationshipErr = relationship.ValidateAcceptedSpecification()
		}
		if relationshipErr != nil {
			return relationshipErr
		}
		key := relationship.Dependent + "\x00" + relationship.Prerequisite
		if _, duplicate := endpoints[key]; duplicate {
			return fmt.Errorf("duplicate relationship endpoint %q -> %q", relationship.Dependent, relationship.Prerequisite)
		}
		endpoints[key] = struct{}{}
		if relationship.Kind == RelationshipHardDependency {
			adjacency[relationship.Dependent] = append(adjacency[relationship.Dependent], relationship.Prerequisite)
		}
	}
	state := map[string]uint8{}
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return fmt.Errorf("hard-dependency cycle includes %q", id)
		}
		if state[id] == 2 {
			return nil
		}
		state[id] = 1
		for _, prerequisite := range adjacency[id] {
			if _, ok := ids[prerequisite]; !ok {
				return errors.New("hard dependency names unknown candidate")
			}
			if err := visit(prerequisite); err != nil {
				return err
			}
		}
		state[id] = 2
		return nil
	}
	for id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
