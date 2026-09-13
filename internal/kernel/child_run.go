package kernel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/scheduler"
)

// CapabilityGrant is the durable authority ceiling available to a run. It is
// not itself a runtime lease; privileged execution still requires a valid
// lease at the final capability/effect boundary.
type CapabilityGrant struct {
	Capability string
	Operations []string
	Scope      string
}

type RunAuthority struct {
	Scope        string
	Capabilities []CapabilityGrant
	Quota        scheduler.Quota
	Depth        int
	MaxDepth     int
}

type ChildRequirements struct {
	Scope        string
	Capabilities []CapabilityGrant
	Quota        scheduler.Quota
}

// DeriveChildAuthority computes the authority available to a child graph. A
// child may narrow parent scope/capabilities/quotas but can never expand them.
func DeriveChildAuthority(parent RunAuthority, child ChildRequirements) (RunAuthority, error) {
	if parent.Scope == "" || child.Scope == "" {
		return RunAuthority{}, errors.New("parent and child scopes are required")
	}
	if !capability.ScopeAllows(parent.Scope, child.Scope) {
		return RunAuthority{}, fmt.Errorf("child scope %q expands parent scope %q", child.Scope, parent.Scope)
	}
	if parent.MaxDepth > 0 && parent.Depth+1 > parent.MaxDepth {
		return RunAuthority{}, errors.New("nested graph depth limit exceeded")
	}

	grants := make([]CapabilityGrant, 0, len(child.Capabilities))
	for _, requested := range child.Capabilities {
		matched := false
		for _, granted := range parent.Capabilities {
			if granted.Capability != requested.Capability || !capability.ScopeAllows(granted.Scope, requested.Scope) {
				continue
			}
			for _, operation := range requested.Operations {
				if !slices.Contains(granted.Operations, operation) {
					return RunAuthority{}, fmt.Errorf("child operation %q for capability %q is not granted by parent", operation, requested.Capability)
				}
			}
			matched = true
			break
		}
		if !matched {
			return RunAuthority{}, fmt.Errorf("child capability %q scope %q is not granted by parent", requested.Capability, requested.Scope)
		}
		grants = append(grants, CapabilityGrant{
			Capability: requested.Capability,
			Operations: append([]string(nil), requested.Operations...),
			Scope:      requested.Scope,
		})
	}

	return RunAuthority{
		Scope:        child.Scope,
		Capabilities: grants,
		Quota:        scheduler.EffectiveQuota(parent.Quota, child.Quota),
		Depth:        parent.Depth + 1,
		MaxDepth:     parent.MaxDepth,
	}, nil
}

// RunTree owns cancellation lineage for active nested runs. Cancellation is
// propagated through context cancellation, so executors that honor context are
// interrupted without requiring cooperation from the LLM or client.
type RunTree struct {
	mu      sync.Mutex
	entries map[string]runTreeEntry
}

type runTreeEntry struct {
	parent string
	cancel context.CancelFunc
}

func NewRunTree() *RunTree { return &RunTree{entries: map[string]runTreeEntry{}} }

func (t *RunTree) RegisterRoot(parent context.Context, runID string) (context.Context, error) {
	return t.register(parent, runID, "")
}

func (t *RunTree) RegisterChild(parentCtx context.Context, parentRunID, childRunID string) (context.Context, error) {
	if parentRunID == "" {
		return nil, errors.New("parent run id is required")
	}
	t.mu.Lock()
	_, parentExists := t.entries[parentRunID]
	t.mu.Unlock()
	if !parentExists {
		return nil, fmt.Errorf("parent run %q is not registered", parentRunID)
	}
	return t.register(parentCtx, childRunID, parentRunID)
}

func (t *RunTree) register(parent context.Context, runID, parentRunID string) (context.Context, error) {
	if t == nil || parent == nil || runID == "" {
		return nil, errors.New("run tree, parent context, and run id are required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.entries[runID]; exists {
		return nil, fmt.Errorf("run %q is already registered", runID)
	}
	ctx, cancel := context.WithCancel(parent)
	t.entries[runID] = runTreeEntry{parent: parentRunID, cancel: cancel}
	return ctx, nil
}

func (t *RunTree) Cancel(runID string) error {
	if t == nil || runID == "" {
		return errors.New("run tree and run id are required")
	}
	t.mu.Lock()
	entry, ok := t.entries[runID]
	if !ok {
		t.mu.Unlock()
		return fmt.Errorf("run %q is not registered", runID)
	}
	entry.cancel()
	// Child contexts are derived from the parent context, so cancelling the
	// parent automatically propagates. Explicit descendant iteration is not
	// required and would create a second cancellation truth source.
	t.mu.Unlock()
	return nil
}

func (t *RunTree) Unregister(runID string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.entries[runID]; ok {
		entry.cancel()
		delete(t.entries, runID)
	}
}
