package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type MergeSemantics string

const (
	MergeCommutative MergeSemantics = "commutative"
	MergeVersioned   MergeSemantics = "versioned_lineage"
	MergeExclusive   MergeSemantics = "exclusive"
)

type Sensitivity string

const (
	PublicState    Sensitivity = "public"
	SensitiveState Sensitivity = "sensitive"
)

type PortableRecord struct {
	ID                 string               `json:"id"`
	Version            string               `json:"version"`
	Kind               RecordKind           `json:"kind"`
	EntityID           string               `json:"entity_id"`
	Scope              string               `json:"scope"`
	Portability        PortabilityClass     `json:"portability"`
	Merge              MergeSemantics       `json:"merge"`
	SourceInstallation string               `json:"source_installation"`
	SourceAgentID      string               `json:"source_agent_id,omitempty"`
	GenerationID       string               `json:"generation_id,omitempty"`
	SessionID          string               `json:"session_id"`
	ParentIDs          []string             `json:"parent_ids,omitempty"`
	SupersedesIDs      []string             `json:"supersedes_ids,omitempty"`
	EvidenceIDs        []string             `json:"evidence_ids,omitempty"`
	MutationType       string               `json:"mutation_type"`
	Trust              contracts.TrustClass `json:"trust"`
	Sensitivity        Sensitivity          `json:"sensitivity"`
	Payload            json.RawMessage      `json:"payload"`
	OccurredAt         time.Time            `json:"occurred_at"`
}

type StateEnvelope struct {
	ID                    string                  `json:"id"`
	Version               string                  `json:"version"`
	Source                contracts.PrincipalRef  `json:"source"`
	SourceSequence        int64                   `json:"source_sequence"`
	CreatedAt             time.Time               `json:"created_at"`
	Records               []PortableRecord        `json:"records"`
	CryptoProfile         contracts.CryptoProfile `json:"crypto_profile,omitempty"`
	ProtectionEvidenceRef string                  `json:"protection_evidence_ref,omitempty"`
}

type Conflict struct {
	ID                  string     `json:"id"`
	Version             string     `json:"version"`
	Kind                RecordKind `json:"kind"`
	EntityID            string     `json:"entity_id"`
	Scope               string     `json:"scope"`
	CompetingRecordIDs  []string   `json:"competing_record_ids"`
	SourceInstallations []string   `json:"source_installations"`
	SecuritySignificant bool       `json:"security_significant"`
	AllowedResolutions  []string   `json:"allowed_resolutions"`
}

func FreezeRecord(record PortableRecord) (PortableRecord, error) {
	record.ID, record.Version = "", portableRecordVersions.CurrentVersion()
	record.ParentIDs = canonicalStrings(record.ParentIDs)
	record.SupersedesIDs = canonicalStrings(record.SupersedesIDs)
	record.EvidenceIDs = canonicalStrings(record.EvidenceIDs)
	var payload any
	if len(record.Payload) == 0 || json.Unmarshal(record.Payload, &payload) != nil {
		return PortableRecord{}, errors.New("portable record requires valid canonicalizable payload")
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return PortableRecord{}, err
	}
	record.Payload = canonical
	if err := validateRecord(record, false); err != nil {
		return PortableRecord{}, err
	}
	record.ID, err = contentID(record)
	return record, err
}

func VerifyRecord(record PortableRecord) error {
	if err := validateRecord(record, true); err != nil {
		return err
	}
	id := record.ID
	record.ID = ""
	want, err := contentID(record)
	if err != nil {
		return err
	}
	if id != want {
		return errors.New("portable record digest mismatch")
	}
	return nil
}

func validateRecord(record PortableRecord, requireID bool) error {
	if requireID && record.ID == "" {
		return errors.New("portable record identity is required")
	}
	if _, _, err := portableRecordVersions.Canonicalize(record.Version, nil); err != nil {
		return err
	}
	if record.EntityID == "" || record.Scope == "" || record.SourceInstallation == "" || record.SessionID == "" || record.MutationType == "" || record.Trust == "" || record.OccurredAt.IsZero() {
		return errors.New("portable record requires versioned identity, provenance, scope, mutation, trust, and time")
	}
	class, err := Classify(record.Kind)
	if err != nil || class != record.Portability {
		return errors.New("portable record class does not match its registered family")
	}
	if record.Portability == LocalEphemeral || record.Portability == Nonportable {
		return errors.New("local/nonportable state cannot become a portable record")
	}
	switch record.Portability {
	case PortableMergeable:
		if record.Merge != MergeCommutative && record.Merge != MergeExclusive {
			return errors.New("mergeable state requires declared commutative or exclusive semantics")
		}
	case PortableVersioned:
		if record.Merge != MergeVersioned {
			return errors.New("versioned state requires lineage merge semantics")
		}
	case PortableSingleWriter, LocalSensitiveReference:
		if record.Merge != MergeExclusive {
			return errors.New("single-writer/reference state requires exclusive semantics")
		}
	}
	if record.Sensitivity != PublicState && record.Sensitivity != SensitiveState {
		return errors.New("portable record sensitivity is required")
	}
	return nil
}

func FreezeEnvelope(envelope StateEnvelope) (StateEnvelope, error) {
	envelope.ID, envelope.Version = "", portableEnvelopeVersions.CurrentVersion()
	if envelope.Source.Validate() != nil || envelope.SourceSequence <= 0 || envelope.CreatedAt.IsZero() || len(envelope.Records) == 0 {
		return StateEnvelope{}, errors.New("portable envelope requires source, positive sequence, time, and records")
	}
	envelope.Records = append([]PortableRecord(nil), envelope.Records...)
	sort.Slice(envelope.Records, func(i, j int) bool { return envelope.Records[i].ID < envelope.Records[j].ID })
	seen := map[string]bool{}
	for _, record := range envelope.Records {
		if err := VerifyRecord(record); err != nil {
			return StateEnvelope{}, err
		}
		if seen[record.ID] {
			return StateEnvelope{}, errors.New("portable envelope contains duplicate record identity")
		}
		seen[record.ID] = true
		if record.Sensitivity == SensitiveState && (envelope.CryptoProfile == "" || envelope.ProtectionEvidenceRef == "") {
			return StateEnvelope{}, errors.New("sensitive portable state requires declared cryptographic protection evidence")
		}
	}
	if envelope.CryptoProfile != "" {
		if err := envelope.CryptoProfile.Validate(); err != nil {
			return StateEnvelope{}, err
		}
	}
	var err error
	envelope.ID, err = contentID(envelope)
	return envelope, err
}

func VerifyEnvelope(envelope StateEnvelope) error {
	id := envelope.ID
	if id == "" {
		return errors.New("portable envelope identity and supported version are required")
	}
	if _, _, err := portableEnvelopeVersions.Canonicalize(envelope.Version, nil); err != nil {
		return err
	}
	envelope.ID = ""
	frozen, err := FreezeEnvelope(envelope)
	if err != nil {
		return err
	}
	if id != frozen.ID {
		return errors.New("portable envelope digest mismatch")
	}
	return nil
}

func freezeConflict(conflict Conflict) (Conflict, error) {
	conflict.ID, conflict.Version = "", portableConflictVersions.CurrentVersion()
	conflict.CompetingRecordIDs = canonicalStrings(conflict.CompetingRecordIDs)
	conflict.SourceInstallations = canonicalStrings(conflict.SourceInstallations)
	conflict.AllowedResolutions = canonicalStrings(conflict.AllowedResolutions)
	if conflict.Kind == "" || conflict.EntityID == "" || conflict.Scope == "" || len(conflict.CompetingRecordIDs) < 2 || len(conflict.SourceInstallations) == 0 || len(conflict.AllowedResolutions) == 0 {
		return Conflict{}, errors.New("portable conflict requires subject, competing records, sources, and resolutions")
	}
	var err error
	conflict.ID, err = contentID(conflict)
	return conflict, err
}

func contentID(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func canonicalStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
