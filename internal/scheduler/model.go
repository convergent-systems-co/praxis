package scheduler

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

type Priority int

type ResourceRequirement struct {
	Key       string
	Capacity  int64
	Exclusive bool
}

type Slice struct {
	ID           string
	Dependencies []string
	Resources    []ResourceRequirement
	Priority     Priority
	CreatedAt    time.Time
	Deadline     *time.Time
	MaxAttempts  int
}

func (s Slice) Validate() error {
	if s.ID == "" {
		return errors.New("slice id is required")
	}
	if s.MaxAttempts < 0 {
		return errors.New("slice max attempts cannot be negative")
	}
	seen := map[string]struct{}{}
	for _, r := range s.Resources {
		if r.Key == "" || r.Capacity <= 0 {
			return errors.New("resource key and positive capacity are required")
		}
		if _, ok := seen[r.Key]; ok {
			return fmt.Errorf("duplicate resource requirement %q", r.Key)
		}
		seen[r.Key] = struct{}{}
	}
	return nil
}

// OrderedRequirements returns a deterministic acquisition order. The scheduler
// must acquire the complete set atomically; callers must not hold a prefix while waiting.
func (s Slice) OrderedRequirements() ([]ResourceRequirement, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	out := append([]ResourceRequirement(nil), s.Resources...)
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

type ResourceState struct {
	Key       string
	Capacity  int64
	Allocated int64
	Exclusive bool
}

func (r ResourceState) Available(req ResourceRequirement) bool {
	if r.Key != req.Key || r.Capacity < 0 || r.Allocated < 0 || r.Allocated > r.Capacity {
		return false
	}
	if req.Exclusive || r.Exclusive {
		return r.Allocated == 0 && req.Capacity <= r.Capacity
	}
	return r.Capacity-r.Allocated >= req.Capacity
}

// CanAcquireAll is a read-only admission check. Actual allocation must use an
// authoritative scheduler transaction/lock so the state cannot change between check and commit.
func CanAcquireAll(s Slice, states map[string]ResourceState) (bool, string, error) {
	reqs, err := s.OrderedRequirements()
	if err != nil {
		return false, "", err
	}
	for _, req := range reqs {
		state, ok := states[req.Key]
		if !ok || !state.Available(req) {
			return false, req.Key, nil
		}
	}
	return true, "", nil
}
