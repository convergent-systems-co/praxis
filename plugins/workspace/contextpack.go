package workspace

import (
	"errors"
	"sort"
)

type EvidenceClass string

const (
	EvidenceExact      EvidenceClass = "exact"
	EvidenceStructural EvidenceClass = "structural"
	EvidenceSemantic   EvidenceClass = "semantic"
	EvidenceInferred   EvidenceClass = "inferred"
)

type Evidence struct {
	ID          string
	Path        string
	Class       EvidenceClass
	Bytes       int
	TokenEstimate int
	Score       float64
	Sensitive   bool
	Content     string
}

type ContextBudget struct {
	MaxBytes  int
	MaxTokens int
	MaxItems  int
}

type ContextPack struct {
	Items          []Evidence
	BytesUsed      int
	TokensUsed     int
	TruncatedCount int
}

// BuildContextPack ranks evidence then admits only items that fit every active
// budget. Sensitive evidence must already have passed destination release policy.
func BuildContextPack(items []Evidence, budget ContextBudget) (ContextPack, error) {
	if budget.MaxBytes < 0 || budget.MaxTokens < 0 || budget.MaxItems < 0 {
		return ContextPack{}, errors.New("context budget cannot be negative")
	}
	ordered := append([]Evidence(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Score != ordered[j].Score {
			return ordered[i].Score > ordered[j].Score
		}
		return ordered[i].ID < ordered[j].ID
	})

	pack := ContextPack{}
	for _, item := range ordered {
		if item.Bytes < 0 || item.TokenEstimate < 0 {
			return ContextPack{}, errors.New("evidence size/token estimate cannot be negative")
		}
		if budget.MaxItems > 0 && len(pack.Items) >= budget.MaxItems {
			pack.TruncatedCount++
			continue
		}
		if budget.MaxBytes > 0 && pack.BytesUsed+item.Bytes > budget.MaxBytes {
			pack.TruncatedCount++
			continue
		}
		if budget.MaxTokens > 0 && pack.TokensUsed+item.TokenEstimate > budget.MaxTokens {
			pack.TruncatedCount++
			continue
		}
		pack.Items = append(pack.Items, item)
		pack.BytesUsed += item.Bytes
		pack.TokensUsed += item.TokenEstimate
	}
	return pack, nil
}
