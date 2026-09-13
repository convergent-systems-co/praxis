package kernel

import (
	"errors"
	"fmt"
)

// Checkpoint captures only durable execution state required to resume a graph.
// Model conversation history is intentionally not authoritative runtime state.
type Checkpoint struct {
	RunID              string
	AgentID            string
	GraphID            string
	GraphVersion       string
	CurrentNode        string
	RunState           RunState
	TransitionCount    int
	LastEventSequence  int64
	CompletedEvidence  []string
	OutstandingEffects []string
	ChildRuns          []string
	QuotaTokensUsed    int64
	QuotaCostMicros    int64
}

func (c Checkpoint) Validate(g GraphDef) error {
	if c.RunID == "" || c.GraphID == "" || c.GraphVersion == "" {
		return errors.New("checkpoint run, graph id, and graph version are required")
	}
	if c.GraphID != g.ID || c.GraphVersion != g.Version {
		return errors.New("checkpoint graph identity/version mismatch")
	}
	if c.TransitionCount < 0 || c.LastEventSequence < 0 || c.QuotaTokensUsed < 0 || c.QuotaCostMicros < 0 {
		return errors.New("checkpoint counters cannot be negative")
	}
	if g.MaxTransitions > 0 && c.TransitionCount > g.MaxTransitions {
		return fmt.Errorf("checkpoint transition count %d exceeds graph bound %d", c.TransitionCount, g.MaxTransitions)
	}
	if c.CurrentNode != "" {
		found := false
		for _, n := range g.Nodes {
			if n.ID == c.CurrentNode {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("checkpoint current node %q does not exist in graph", c.CurrentNode)
		}
	}
	if c.RunState.Terminal() && len(c.OutstandingEffects) > 0 {
		return errors.New("terminal checkpoint cannot retain unresolved external effects")
	}
	return nil
}

// ResumeState returns the safe runtime state to enter after restart. Privileged
// external conditions are revalidated elsewhere before runnable work is admitted.
func (c Checkpoint) ResumeState() RunState {
	switch c.RunState {
	case RunRunning, RunRunnable:
		return RunWaiting
	case RunCancelling:
		if len(c.OutstandingEffects) > 0 {
			return RunReconciling
		}
		return RunCancelling
	default:
		return c.RunState
	}
}
