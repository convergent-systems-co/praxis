package preference

import (
	"errors"
	"sort"
	"time"
)

type SourceClass string

const (
	SourceExplicitUser SourceClass = "explicit_user"
	SourceExplicitOrg  SourceClass = "explicit_org"
	SourceMigrated     SourceClass = "migrated"
	SourceLearned      SourceClass = "learned"
	SourcePresetSeed   SourceClass = "preset_seed"
	SourcePackageDefault SourceClass = "package_default"
)

type Record struct {
	ID          string
	SlotID      string
	Value       string
	Scope       string
	ScopeDepth  int
	Source      SourceClass
	Confidence  float64
	UpdatedAt   time.Time
	ValidUntil  *time.Time
	Superseded  bool
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

// Resolve selects one record deterministically. Policy constraints are applied
// separately and can still reject the resulting value.
func Resolve(slotID string, candidates []Record, now time.Time) (Record, error) {
	eligible := make([]Record, 0, len(candidates))
	for _, r := range candidates {
		if r.ID == "" || r.SlotID != slotID || !r.active(now) || sourceRank(r.Source) == 0 {
			continue
		}
		eligible = append(eligible, r)
	}
	if len(eligible) == 0 {
		return Record{}, errors.New("no applicable preference record")
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		ri, rj := sourceRank(eligible[i].Source), sourceRank(eligible[j].Source)
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
