package scheduler

import (
	"errors"
	"math"
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

func TestBudgetChargesChildReservationToAncestors(t *testing.T) {
	root := NewBudget(nil, Quota{MaxActiveSlices: 2, MaxAttempts: 3, MaxTokens: 100})
	child := NewBudget(root, Quota{MaxActiveSlices: 5, MaxAttempts: 2, MaxTokens: 80})
	if got := child.Quota(); got.MaxActiveSlices != 2 || got.MaxAttempts != 2 || got.MaxTokens != 80 {
		t.Fatalf("child quota expanded parent: %+v", got)
	}
	reservation := Usage{ActiveSlices: 1, Attempts: 1, Tokens: 40}
	if err := child.Reserve(reservation); err != nil {
		t.Fatal(err)
	}
	if got := root.Usage(); got != reservation {
		t.Fatalf("parent did not receive child charge: %+v", got)
	}
	if err := child.Reserve(Usage{ActiveSlices: 2, Attempts: 1, Tokens: 1}); !errors.Is(err, ErrQuotaExceeded) {
		t.Fatalf("expected ancestor quota denial, got %v", err)
	}
	if got := root.Usage(); got != reservation {
		t.Fatalf("denied reservation changed ancestor usage: %+v", got)
	}
}

func TestBudgetReleaseIsAtomicAndCannotOverRelease(t *testing.T) {
	root := NewBudget(nil, Quota{MaxAttempts: 4})
	child := NewBudget(root, Quota{MaxAttempts: 4})
	reserved := Usage{Attempts: 2}
	if err := child.Reserve(reserved); err != nil {
		t.Fatal(err)
	}
	if err := child.Release(Usage{Attempts: 3}); err == nil {
		t.Fatal("expected over-release to fail")
	}
	if got := child.Usage(); got != reserved {
		t.Fatalf("failed release changed child usage: %+v", got)
	}
	if err := child.Release(reserved); err != nil {
		t.Fatal(err)
	}
	if got := root.Usage(); got != (Usage{}) {
		t.Fatalf("release did not return ancestor usage to zero: %+v", got)
	}
}

func TestBudgetRejectsNegativeReservationWithoutMutation(t *testing.T) {
	b := NewBudget(nil, Quota{MaxTokens: 10})
	if err := b.Reserve(Usage{Tokens: -1}); !errors.Is(err, ErrInvalidUsage) {
		t.Fatalf("expected invalid usage, got %v", err)
	}
	if got := b.Usage(); got != (Usage{}) {
		t.Fatalf("invalid reservation changed usage: %+v", got)
	}
}

func TestBudgetRejectsCounterOverflowWithoutMutation(t *testing.T) {
	b := NewBudget(nil, Quota{})
	if err := b.Reserve(Usage{Attempts: math.MaxInt}); err != nil {
		t.Fatal(err)
	}
	if err := b.Reserve(Usage{Attempts: 1}); !errors.Is(err, ErrUsageOverflow) {
		t.Fatalf("expected overflow rejection, got %v", err)
	}
	if got := b.Usage(); got.Attempts != math.MaxInt {
		t.Fatalf("overflow attempt changed usage: %+v", got)
	}
}
