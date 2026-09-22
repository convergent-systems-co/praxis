package contracts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const (
	GateAlternativesFromDossier = "dossier.offered_alternatives"
	GateContentAddressingSHA256 = "sha256-exact-bytes"
	GateAuthorityHuman          = "human"
	GateDossierStatusUndecided  = "undecided"
	MaxGovernedArtifactBytes    = 1 << 20
)

// GovernedOutputContract declares an exact future artifact role. It is a
// proposal-time contract, not the future artifact or evidence that it exists.
type GovernedOutputContract struct {
	Role          string `json:"role"`
	EvidenceClass string `json:"evidence_class"`
	SourceRef     string `json:"source_ref"`
	SchemaID      string `json:"schema_id"`
}

// GovernedArtifactEvidence is captured by the controller from the exact
// qualified producer checkpoint. Bytes are preserved in the completion event
// so gate reconciliation never rereads a mutable checkout path.
type GovernedArtifactEvidence struct {
	ProducerCandidateID       string `json:"producer_candidate_id"`
	ProducerSpecificationHash string `json:"producer_specification_digest"`
	ValidationProfileDigest   string `json:"validation_profile_digest"`
	ConformanceQualified      bool   `json:"conformance_qualified"`
	Checkpoint                string `json:"checkpoint"`
	Role                      string `json:"role"`
	EvidenceClass             string `json:"evidence_class"`
	SourceRef                 string `json:"source_ref"`
	SchemaID                  string `json:"schema_id"`
	Digest                    string `json:"digest"`
	Bytes                     []byte `json:"bytes"`
}

func (e GovernedArtifactEvidence) Validate() error {
	if e.ProducerCandidateID == "" || e.Checkpoint == "" || e.Role == "" || e.EvidenceClass == "" || e.SourceRef == "" || e.SchemaID == "" {
		return errors.New("governed artifact identity is incomplete")
	}
	if !e.ConformanceQualified {
		return errors.New("governed artifact producer is not conformance qualified")
	}
	if err := ValidateSHA256Digest(e.ProducerSpecificationHash); err != nil {
		return fmt.Errorf("governed artifact producer specification: %w", err)
	}
	if err := ValidateSHA256Digest(e.ValidationProfileDigest); err != nil {
		return fmt.Errorf("governed artifact validation profile: %w", err)
	}
	if len(e.Bytes) == 0 || len(e.Bytes) > MaxGovernedArtifactBytes {
		return errors.New("governed artifact bytes are missing or exceed the bound")
	}
	sum := sha256.Sum256(e.Bytes)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != e.Digest {
		return fmt.Errorf("governed artifact digest mismatch: got %s want %s", got, e.Digest)
	}
	return nil
}

type GateDossierRequirement struct {
	ProducerCandidateID string `json:"producer_candidate_id"`
	Role                string `json:"role"`
	EvidenceClass       string `json:"evidence_class"`
	SchemaID            string `json:"schema_id"`
}

// AuthorityGateContract is the immutable proposal-time schema. It describes
// how future governed evidence is resolved; it never embeds that evidence.
type AuthorityGateContract struct {
	QuestionSchemaID     string                 `json:"question_schema_id"`
	Question             string                 `json:"question"`
	RequiredDossier      GateDossierRequirement `json:"required_dossier"`
	AlternativesRule     string                 `json:"alternatives_rule"`
	RequiredAlternatives []string               `json:"required_alternatives,omitempty"`
	AuthorityPrincipal   string                 `json:"authority_principal_kind"`
	DownstreamSemantics  string                 `json:"downstream_consequence_semantics"`
	ContentAddressing    string                 `json:"content_addressing"`
}

type GateDossierEvidenceRef struct {
	SourceRef    string `json:"source_ref"`
	SourceDigest string `json:"source_digest"`
}

// GateDossier is runtime evidence produced by DOS. Status must remain
// undecided; the owner decision is a separate authority record.
type GateDossier struct {
	SchemaID            string                   `json:"schema_id"`
	GateID              string                   `json:"gate_id"`
	ProducerCandidateID string                   `json:"producer_candidate_id"`
	Role                string                   `json:"role"`
	EvidenceClass       string                   `json:"evidence_class"`
	Status              string                   `json:"status"`
	Question            string                   `json:"question"`
	OfferedAlternatives []string                 `json:"offered_alternatives"`
	Consequences        map[string]string        `json:"consequences"`
	Evidence            []GateDossierEvidenceRef `json:"evidence"`
	Recommendation      string                   `json:"recommendation,omitempty"`
}

func ParseGovernedOutputContracts(specification []byte) ([]GovernedOutputContract, error) {
	var spec struct {
		GovernedOutputs []GovernedOutputContract `json:"governed_outputs"`
	}
	if err := UnmarshalExactJSON(specification, &spec, false); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for _, output := range spec.GovernedOutputs {
		if output.Role == "" || output.EvidenceClass == "" || output.SourceRef == "" || output.SchemaID == "" || filepath.IsAbs(output.SourceRef) {
			return nil, errors.New("governed output contract is incomplete or has an absolute source_ref")
		}
		clean := filepath.Clean(output.SourceRef)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, errors.New("governed output source_ref escapes repository")
		}
		key := output.Role + "\x00" + output.EvidenceClass
		if _, duplicate := seen[key]; duplicate {
			return nil, errors.New("duplicate governed output role/evidence class")
		}
		seen[key] = struct{}{}
	}
	return spec.GovernedOutputs, nil
}

func ParseAuthorityGateContract(specification []byte) (AuthorityGateContract, error) {
	var raw struct {
		AuthorityGateContract
		Dossier       json.RawMessage `json:"dossier"`
		DossierRef    string          `json:"dossier_ref"`
		DossierDigest string          `json:"dossier_digest"`
	}
	if err := UnmarshalExactJSON(specification, &raw, false); err != nil {
		return AuthorityGateContract{}, err
	}
	if len(bytes.TrimSpace(raw.Dossier)) != 0 || raw.DossierRef != "" || raw.DossierDigest != "" {
		return AuthorityGateContract{}, errors.New("proposal-time gate specification must not embed future dossier evidence")
	}
	c := raw.AuthorityGateContract
	if c.QuestionSchemaID == "" || c.Question == "" || c.RequiredDossier.ProducerCandidateID == "" || c.RequiredDossier.Role == "" || c.RequiredDossier.EvidenceClass == "" || c.RequiredDossier.SchemaID == "" {
		return AuthorityGateContract{}, errors.New("authority gate question/dossier contract is incomplete")
	}
	if c.AlternativesRule != GateAlternativesFromDossier || c.AuthorityPrincipal != GateAuthorityHuman || c.DownstreamSemantics == "" || c.ContentAddressing != GateContentAddressingSHA256 {
		return AuthorityGateContract{}, errors.New("authority gate binding rules are unsupported or incomplete")
	}
	seen := map[string]struct{}{}
	for _, alternative := range c.RequiredAlternatives {
		if strings.TrimSpace(alternative) == "" {
			return AuthorityGateContract{}, errors.New("authority gate required alternative is empty")
		}
		if _, duplicate := seen[alternative]; duplicate {
			return AuthorityGateContract{}, errors.New("authority gate duplicates a required alternative")
		}
		seen[alternative] = struct{}{}
	}
	return c, nil
}

func ResolveGateDossier(gateID string, contract AuthorityGateContract, artifacts []GovernedArtifactEvidence) (GovernedArtifactEvidence, GateDossier, error) {
	var matches []GovernedArtifactEvidence
	for _, artifact := range artifacts {
		if artifact.ProducerCandidateID == contract.RequiredDossier.ProducerCandidateID && artifact.Role == contract.RequiredDossier.Role && artifact.EvidenceClass == contract.RequiredDossier.EvidenceClass && artifact.SchemaID == contract.RequiredDossier.SchemaID {
			matches = append(matches, artifact)
		}
	}
	if len(matches) != 1 {
		return GovernedArtifactEvidence{}, GateDossier{}, fmt.Errorf("gate %q requires exactly one qualified dossier artifact, found %d", gateID, len(matches))
	}
	artifact := matches[0]
	if err := artifact.Validate(); err != nil {
		return GovernedArtifactEvidence{}, GateDossier{}, err
	}
	var dossier GateDossier
	if err := UnmarshalExactJSON(artifact.Bytes, &dossier, true); err != nil {
		return GovernedArtifactEvidence{}, GateDossier{}, fmt.Errorf("decode gate dossier: %w", err)
	}
	if dossier.SchemaID != contract.RequiredDossier.SchemaID || dossier.GateID != gateID || dossier.ProducerCandidateID != artifact.ProducerCandidateID || dossier.Role != artifact.Role || dossier.EvidenceClass != artifact.EvidenceClass || dossier.Status != GateDossierStatusUndecided || dossier.Question != contract.Question {
		return GovernedArtifactEvidence{}, GateDossier{}, errors.New("gate dossier identity, role, status, or question does not satisfy the proposal-time contract")
	}
	if len(dossier.OfferedAlternatives) < 2 || len(dossier.Evidence) == 0 {
		return GovernedArtifactEvidence{}, GateDossier{}, errors.New("gate dossier lacks alternatives or primary evidence")
	}
	seen := map[string]struct{}{}
	for _, alternative := range dossier.OfferedAlternatives {
		if strings.TrimSpace(alternative) == "" || strings.TrimSpace(dossier.Consequences[alternative]) == "" {
			return GovernedArtifactEvidence{}, GateDossier{}, errors.New("gate dossier alternative lacks a consequence")
		}
		if _, duplicate := seen[alternative]; duplicate {
			return GovernedArtifactEvidence{}, GateDossier{}, errors.New("gate dossier duplicates an offered alternative")
		}
		seen[alternative] = struct{}{}
	}
	for _, required := range contract.RequiredAlternatives {
		if !slices.Contains(dossier.OfferedAlternatives, required) {
			return GovernedArtifactEvidence{}, GateDossier{}, fmt.Errorf("gate dossier omits required alternative %q", required)
		}
	}
	for _, evidence := range dossier.Evidence {
		if evidence.SourceRef == "" || ValidateSHA256Digest(evidence.SourceDigest) != nil {
			return GovernedArtifactEvidence{}, GateDossier{}, errors.New("gate dossier primary evidence reference is incomplete")
		}
	}
	return artifact, dossier, nil
}
