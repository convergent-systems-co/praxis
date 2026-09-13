package scheduler

import "errors"

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
