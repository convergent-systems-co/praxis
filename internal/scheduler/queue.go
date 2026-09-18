package scheduler

import (
	"sort"
	"time"
)

// QueueItem represents a runnable slice waiting for admission.
type QueueItem struct {
	Slice Slice
}

// OrderRunnable returns a new slice ordered by effective priority, then age.
// Aging prevents permanent starvation under sustained higher-priority load.
func OrderRunnable(items []QueueItem, now time.Time, agingInterval time.Duration) []QueueItem {
	out := append([]QueueItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		pi := effectivePriority(out[i].Slice, now, agingInterval)
		pj := effectivePriority(out[j].Slice, now, agingInterval)
		if pi != pj {
			return pi > pj
		}
		if !out[i].Slice.CreatedAt.Equal(out[j].Slice.CreatedAt) {
			return out[i].Slice.CreatedAt.Before(out[j].Slice.CreatedAt)
		}
		return out[i].Slice.ID < out[j].Slice.ID
	})
	return out
}

func effectivePriority(s Slice, now time.Time, agingInterval time.Duration) Priority {
	p := s.Priority
	if agingInterval <= 0 || s.CreatedAt.IsZero() || !now.After(s.CreatedAt) {
		return p
	}
	ageSteps := int(now.Sub(s.CreatedAt) / agingInterval)
	return p + Priority(ageSteps)
}
