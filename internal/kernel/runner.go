package kernel

import (
	"context"
	"errors"
	"fmt"
)

type NodeResult struct {
	Outcome  string
	Evidence []string
}

type NodeExecutor interface {
	ExecuteNode(ctx context.Context, graph GraphDef, node NodeDef, run *RunExecution) (NodeResult, error)
}

type RunExecution struct {
	RunID           string
	GraphID         string
	GraphVersion    string
	CurrentNode     string
	State           RunState
	TransitionCount int
	Evidence        []string
}

// Run executes canonical graph transitions. It is intentionally agnostic to
// node semantics: capability/inference/human behavior is provided through the
// executor boundary and remains subject to its own contracts.
func Run(ctx context.Context, graph GraphDef, run *RunExecution, executor NodeExecutor) error {
	if err := graph.Validate(); err != nil {
		return fmt.Errorf("validate graph: %w", err)
	}
	if run == nil || executor == nil {
		return errors.New("run state and node executor are required")
	}
	if run.RunID == "" {
		return errors.New("run id is required")
	}
	if run.GraphID == "" {
		run.GraphID = graph.ID
	}
	if run.GraphVersion == "" {
		run.GraphVersion = graph.Version
	}
	if run.GraphID != graph.ID || run.GraphVersion != graph.Version {
		return errors.New("run graph identity/version mismatch")
	}
	if run.CurrentNode == "" {
		run.CurrentNode = graph.EntryNode
	}
	if run.State == "" || run.State == RunQueued || run.State == RunRunnable || run.State == RunWaiting {
		run.State = RunRunning
	}

	nodes := make(map[string]NodeDef, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}

	for {
		if err := ctx.Err(); err != nil {
			run.State = RunCancelling
			return err
		}
		node, ok := nodes[run.CurrentNode]
		if !ok {
			run.State = RunFailed
			return fmt.Errorf("current node %q does not exist", run.CurrentNode)
		}
		if node.Class == NodeTerminal {
			run.State = node.TerminalState
			return nil
		}
		if graph.MaxTransitions > 0 && run.TransitionCount >= graph.MaxTransitions {
			run.State = RunFailed
			return errors.New("graph transition bound exhausted")
		}

		result, err := executor.ExecuteNode(ctx, graph, node, run)
		if err != nil {
			run.State = RunFailed
			return fmt.Errorf("execute node %s: %w", node.ID, err)
		}
		if result.Outcome == "" {
			run.State = RunFailed
			return fmt.Errorf("node %s returned empty outcome", node.ID)
		}
		run.Evidence = append(run.Evidence, result.Evidence...)
		next, err := ResolveTransition(graph, node.ID, result.Outcome)
		if err != nil {
			run.State = RunFailed
			return err
		}
		run.CurrentNode = next
		run.TransitionCount++
	}
}
