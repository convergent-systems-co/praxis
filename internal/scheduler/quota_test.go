package scheduler

import (
	"errors"
	"testing"
)

func TestEffectiveQuotaCannotExpandParent(t *testing.T) {
	parent := Quota{MaxActiveSlices: 4, MaxTokens: 1000}
	child := Quota{MaxActiveSlices: 10, MaxTokens: 500}
	q := EffectiveQuota(parent, child)
	if q.MaxActiveSlices != 4 || q.MaxTokens != 500 {
		t.Fatalf("unexpected effective quota: %+v", q)
	}
}

func TestQuotaRejectsAtLimit(t *testing.T) {
	q := Quota{MaxActiveSlices: 2}
	if err := q.Allows(Usage{ActiveSlices: 2}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
}
