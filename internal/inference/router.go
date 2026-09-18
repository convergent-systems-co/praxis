package inference

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

type Tier string

const (
	D0 Tier = "D0"
	D1 Tier = "D1"
	D2 Tier = "D2"
)

type Budget struct {
	Tier                    Tier
	Deadline                time.Duration
	MaxTokens               int
	MaxAttempts             int
	MinQuality              float64
	PreferTimeToFirstAction bool
}

type ExecutorEvidence struct {
	ID                    string
	SupportsTier          map[Tier]bool
	Quality               float64
	P50Latency            time.Duration
	P50TimeToFirstAction  time.Duration
	Available             bool
}

type Decision struct {
	ExecutorID string
	Tier       Tier
	Reason     string
}

// Route selects the lowest-latency eligible executor that satisfies the
// requested tier and quality floor. D0 intentionally returns no executor.
func Route(b Budget, candidates []ExecutorEvidence) (Decision, error) {
	if b.Tier == D0 {
		return Decision{Tier: D0, Reason: "deterministic fast path"}, nil
	}
	if b.Tier != D1 && b.Tier != D2 {
		return Decision{}, fmt.Errorf("unknown reasoning tier %q", b.Tier)
	}
	eligible := make([]ExecutorEvidence, 0, len(candidates))
	for _, c := range candidates {
		if !c.Available || !c.SupportsTier[b.Tier] || c.Quality < b.MinQuality {
			continue
		}
		if b.Deadline > 0 && c.P50Latency > b.Deadline {
			continue
		}
		eligible = append(eligible, c)
	}
	if len(eligible) == 0 {
		return Decision{}, errors.New("no executor satisfies reasoning budget")
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		if b.PreferTimeToFirstAction {
			if eligible[i].P50TimeToFirstAction != eligible[j].P50TimeToFirstAction {
				return eligible[i].P50TimeToFirstAction < eligible[j].P50TimeToFirstAction
			}
		}
		if eligible[i].P50Latency != eligible[j].P50Latency {
			return eligible[i].P50Latency < eligible[j].P50Latency
		}
		return eligible[i].Quality > eligible[j].Quality
	})
	return Decision{ExecutorID: eligible[0].ID, Tier: b.Tier, Reason: "fastest executor meeting quality and budget"}, nil
}
