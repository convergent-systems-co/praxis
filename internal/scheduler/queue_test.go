package scheduler

import (
	"testing"
	"time"
)

func TestOrderRunnableUsesAgeAwarePriority(t *testing.T) {
	now := time.Now().UTC()
	items := []QueueItem{
		{Slice: Slice{ID: "new-high", Priority: 5, CreatedAt: now.Add(-time.Minute)}},
		{Slice: Slice{ID: "old-low", Priority: 1, CreatedAt: now.Add(-10 * time.Minute)}},
	}
	ordered := OrderRunnable(items, now, time.Minute)
	if ordered[0].Slice.ID != "old-low" {
		t.Fatalf("expected aged slice first, got %s", ordered[0].Slice.ID)
	}
}

func TestOrderRunnableFIFOWithinEffectivePriority(t *testing.T) {
	now := time.Now().UTC()
	items := []QueueItem{
		{Slice: Slice{ID: "b", Priority: 2, CreatedAt: now.Add(-2 * time.Minute)}},
		{Slice: Slice{ID: "a", Priority: 2, CreatedAt: now.Add(-3 * time.Minute)}},
	}
	ordered := OrderRunnable(items, now, 0)
	if ordered[0].Slice.ID != "a" {
		t.Fatalf("expected older slice first, got %s", ordered[0].Slice.ID)
	}
}
