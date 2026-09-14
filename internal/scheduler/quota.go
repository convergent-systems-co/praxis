package scheduler

import (
	"errors"
	"math"
	"sync"
)

type Quota struct {
	MaxActiveSlices int
	MaxAttempts     int
	MaxTokens       int64
	MaxCostMicros   int64
}

type Usage struct {
	ActiveSlices int
	Attempts     int
	Tokens       int64
	CostMicros   int64
}

var ErrQuotaExceeded = errors.New("scheduler quota exceeded")
var ErrInvalidUsage = errors.New("scheduler usage must be non-negative")
var ErrUsageOverflow = errors.New("scheduler usage overflow")

func (q Quota) Allows(u Usage) error {
	if q.MaxActiveSlices > 0 && u.ActiveSlices >= q.MaxActiveSlices {
		return ErrQuotaExceeded
	}
	if q.MaxAttempts > 0 && u.Attempts >= q.MaxAttempts {
		return ErrQuotaExceeded
	}
	if q.MaxTokens > 0 && u.Tokens >= q.MaxTokens {
		return ErrQuotaExceeded
	}
	if q.MaxCostMicros > 0 && u.CostMicros >= q.MaxCostMicros {
		return ErrQuotaExceeded
	}
	return nil
}

// Budget is a hierarchical admission boundary. A reservation made in a
// child scope is charged to that scope and every ancestor while holding one
// root lock, so concurrent child work cannot escape its parent's quota.
// Budgets are an in-process admission primitive; durable providers remain
// responsible for persisting their authoritative lease/accounting state.
type Budget struct {
	root   *budgetRoot
	parent *Budget
	quota  Quota
	usage  Usage
}

type budgetRoot struct{ mu sync.Mutex }

// NewBudget derives a child budget by intersecting its requested quota with
// the parent's effective quota. A nil parent creates a root budget.
func NewBudget(parent *Budget, requested Quota) *Budget {
	quota := requested
	if parent != nil {
		quota = EffectiveQuota(parent.quota, requested)
	}
	b := &Budget{parent: parent, quota: quota}
	if parent == nil {
		b.root = &budgetRoot{}
	} else {
		b.root = parent.root
	}
	return b
}

func (b *Budget) Quota() Quota {
	if b == nil {
		return Quota{}
	}
	return b.quota
}

func (b *Budget) Usage() Usage {
	if b == nil || b.root == nil {
		return Usage{}
	}
	b.root.mu.Lock()
	defer b.root.mu.Unlock()
	return b.usage
}

// Reserve atomically admits usage in this scope and all ancestors. It fails
// without changing any scope when any quota would be exceeded.
func (b *Budget) Reserve(delta Usage) error {
	if b == nil || b.root == nil {
		return errors.New("scheduler budget is required")
	}
	if delta.ActiveSlices < 0 || delta.Attempts < 0 || delta.Tokens < 0 || delta.CostMicros < 0 {
		return ErrInvalidUsage
	}
	b.root.mu.Lock()
	defer b.root.mu.Unlock()
	for scope := b; scope != nil; scope = scope.parent {
		if err := allowsDelta(scope.quota, scope.usage, delta); err != nil {
			return err
		}
	}
	for scope := b; scope != nil; scope = scope.parent {
		scope.usage, _ = checkedAddUsage(scope.usage, delta)
	}
	return nil
}

// Release removes a prior reservation from this scope and all ancestors.
// Over-release is rejected and leaves all usage unchanged.
func (b *Budget) Release(delta Usage) error {
	if b == nil || b.root == nil {
		return errors.New("scheduler budget is required")
	}
	if delta.ActiveSlices < 0 || delta.Attempts < 0 || delta.Tokens < 0 || delta.CostMicros < 0 {
		return ErrInvalidUsage
	}
	b.root.mu.Lock()
	defer b.root.mu.Unlock()
	for scope := b; scope != nil; scope = scope.parent {
		if !usageAtLeast(scope.usage, delta) {
			return errors.New("scheduler budget release exceeds reservation")
		}
	}
	for scope := b; scope != nil; scope = scope.parent {
		scope.usage = subtractUsage(scope.usage, delta)
	}
	return nil
}

func allowsDelta(q Quota, used, delta Usage) error {
	next, ok := checkedAddUsage(used, delta)
	if !ok {
		return ErrUsageOverflow
	}
	if q.MaxActiveSlices > 0 && next.ActiveSlices > q.MaxActiveSlices ||
		q.MaxAttempts > 0 && next.Attempts > q.MaxAttempts ||
		q.MaxTokens > 0 && next.Tokens > q.MaxTokens ||
		q.MaxCostMicros > 0 && next.CostMicros > q.MaxCostMicros {
		return ErrQuotaExceeded
	}
	return nil
}

func addUsage(a, b Usage) Usage {
	result, _ := checkedAddUsage(a, b)
	return result
}

func checkedAddUsage(a, b Usage) (Usage, bool) {
	if b.ActiveSlices > math.MaxInt-a.ActiveSlices || b.Attempts > math.MaxInt-a.Attempts || b.Tokens > math.MaxInt64-a.Tokens || b.CostMicros > math.MaxInt64-a.CostMicros {
		return Usage{}, false
	}
	return Usage{ActiveSlices: a.ActiveSlices + b.ActiveSlices, Attempts: a.Attempts + b.Attempts, Tokens: a.Tokens + b.Tokens, CostMicros: a.CostMicros + b.CostMicros}, true
}

func subtractUsage(a, b Usage) Usage {
	return Usage{ActiveSlices: a.ActiveSlices - b.ActiveSlices, Attempts: a.Attempts - b.Attempts, Tokens: a.Tokens - b.Tokens, CostMicros: a.CostMicros - b.CostMicros}
}

func usageAtLeast(a, b Usage) bool {
	return a.ActiveSlices >= b.ActiveSlices && a.Attempts >= b.Attempts && a.Tokens >= b.Tokens && a.CostMicros >= b.CostMicros
}

// EffectiveQuota intersects parent and child quotas by taking the tighter
// non-zero limit in each dimension. A child cannot expand parent authority.
func EffectiveQuota(parent, child Quota) Quota {
	return Quota{
		MaxActiveSlices: minPositive(parent.MaxActiveSlices, child.MaxActiveSlices),
		MaxAttempts:     minPositive(parent.MaxAttempts, child.MaxAttempts),
		MaxTokens:       minPositive64(parent.MaxTokens, child.MaxTokens),
		MaxCostMicros:   minPositive64(parent.MaxCostMicros, child.MaxCostMicros),
	}
}

func minPositive(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func minPositive64(a, b int64) int64 {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}
