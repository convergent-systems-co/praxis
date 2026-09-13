package preference

import (
	"errors"
	"sort"
	"time"
)

type SourceClass string

const (
	SourceExplicitUser   SourceClass = "explicit_user"
	SourceExplicitOrg    SourceClass = "explicit_org"
	SourceMigrated       SourceClass = "migrated"
	SourceLearned        SourceClass = "learned"
	SourcePresetSeed     SourceClass = "preset_seed"
	SourcePackageDefault SourceClass = "package_default"
)

type Record struct {
	ID                   string      `json:"id"`
	Version              string      `json:"version,omitempty"`
	SubjectID            string      `json:"subject_id,omitempty"`
	ContractID           string      `json:"contract_id,omitempty"`
	ContractVersion      string      `json:"contract_version,omitempty"`
	SlotID               string      `json:"slot_id"`
	Value                string      `json:"value"`
	ScopeKind            string      `json:"scope_kind,omitempty"`
	Scope                string      `json:"scope"`
	ScopeDepth           int         `json:"scope_depth"`
	Source               SourceClass `json:"source"`
	Confidence           float64     `json:"confidence,omitempty"`
	Provenance           string      `json:"provenance,omitempty"`
	EvidenceIDs          []string    `json:"evidence_ids,omitempty"`
	AuthorityID          string      `json:"authority_id,omitempty"`
	AuthorityEvidenceRef string      `json:"authority_evidence_ref,omitempty"`
	OriginRecordID       string      `json:"origin_record_id,omitempty"`
	OriginSource         SourceClass `json:"origin_source,omitempty"`
	OriginAuthorityID    string      `json:"origin_authority_id,omitempty"`
	OriginAuthorityRef   string      `json:"origin_authority_ref,omitempty"`
	SupersedesID         string      `json:"supersedes_id,omitempty"`
	CreatedAt            time.Time   `json:"created_at,omitempty"`
	UpdatedAt            time.Time   `json:"updated_at"`
	ValidUntil           *time.Time  `json:"valid_until,omitempty"`
	Superseded           bool        `json:"-"`
}

func (r Record) active(now time.Time) bool {
	return !r.Superseded && (r.ValidUntil == nil || now.Before(*r.ValidUntil))
}

func sourceRank(s SourceClass) int {
	switch s {
	case SourceExplicitUser:
		return 60
	case SourceExplicitOrg:
		return 50
	case SourceMigrated:
		return 40
	case SourceLearned:
		return 30
	case SourcePresetSeed:
		return 20
	case SourcePackageDefault:
		return 10
	default:
		return 0
	}
}

func recordRank(record Record) int {
	if record.Source == SourceMigrated {
		return sourceRank(record.OriginSource)
	}
	return sourceRank(record.Source)
}

// Resolve selects one record deterministically. Policy constraints are applied
// separately and can still reject the resulting value.
func Resolve(slotID string, candidates []Record, now time.Time) (Record, error) {
	eligible := make([]Record, 0, len(candidates))
	for _, r := range candidates {
		if r.ID == "" || r.SlotID != slotID || !r.active(now) || recordRank(r) == 0 {
			continue
		}
		eligible = append(eligible, r)
	}
	if len(eligible) == 0 {
		return Record{}, errors.New("no applicable preference record")
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		ri, rj := recordRank(eligible[i]), recordRank(eligible[j])
		if ri != rj {
			return ri > rj
		}
		if eligible[i].ScopeDepth != eligible[j].ScopeDepth {
			return eligible[i].ScopeDepth > eligible[j].ScopeDepth
		}
		if eligible[i].Source == SourceLearned && eligible[i].Confidence != eligible[j].Confidence {
			return eligible[i].Confidence > eligible[j].Confidence
		}
		return eligible[i].UpdatedAt.After(eligible[j].UpdatedAt)
	})
	return eligible[0], nil
}
