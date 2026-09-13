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

type RunObservationKind string

const (
	ObservationRunStarted    RunObservationKind = "started"
	ObservationRunResumed    RunObservationKind = "resumed"
	ObservationNodeCompleted RunObservationKind = "node_completed"
	ObservationTransitioned  RunObservationKind = "transitioned"
	ObservationRunTerminal   RunObservationKind = "terminal"
)

type RunObservation struct {
	Kind            RunObservationKind `json:"kind"`
	RunID           string             `json:"run_id"`
	GraphID         string             `json:"graph_id"`
	GraphVersion    string             `json:"graph_version"`
	NodeID          string             `json:"node_id,omitempty"`
	Outcome         string             `json:"outcome,omitempty"`
	FromNode        string             `json:"from_node,omitempty"`
	ToNode          string             `json:"to_node,omitempty"`
	State           RunState           `json:"state"`
	TransitionCount int                `json:"transition_count"`
	Evidence        []string           `json:"evidence,omitempty"`
}

type RunObserver interface {
	ObserveRun(ctx context.Context, observation RunObservation) error
}

// Run executes canonical graph transitions without persistence concerns.
func Run(ctx context.Context, graph GraphDef, run *RunExecution, executor NodeExecutor) error {
	return RunObserved(ctx, graph, run, executor, nil)
}

// RunObserved executes the same canonical graph semantics while exposing
// deterministic observations to an optional journal/projection boundary.
// Observer failure is fail-closed: execution stops rather than advancing
// authoritative state that could not be durably recorded.
func RunObserved(ctx context.Context, graph GraphDef, run *RunExecution, executor NodeExecutor, observer RunObserver) error {
	if err := graph.Validate(); err != nil {
		return fmt.Errorf("validate graph: %w", err)
	}
	if run == nil || executor == nil {
		return errors.New("run state and node executor are required")
	}
	if run.RunID == "" {
		return errors.New("run id is required")
	}
	if run.State.Terminal() {
		return errors.New("terminal run cannot be resumed")
	}

	resuming := run.TransitionCount > 0 || run.State == RunRunning || run.State == RunWaiting || run.State == RunSuspended || run.State == RunReconciling || run.State == RunCancelling
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
	if run.State == "" || run.State == RunQueued || run.State == RunRunnable || run.State == RunWaiting || run.State == RunSuspended || run.State == RunReconciling {
		run.State = RunRunning
	}
	observationKind := ObservationRunStarted
	if resuming {
		observationKind = ObservationRunResumed
	}
	if err := observeRun(ctx, observer, RunObservation{
		Kind:            observationKind,
		RunID:           run.RunID,
		GraphID:         run.GraphID,
		GraphVersion:    run.GraphVersion,
		NodeID:          run.CurrentNode,
		State:           run.State,
		TransitionCount: run.TransitionCount,
	}); err != nil {
		run.State = RunFailed
		return fmt.Errorf("record run %s: %w", observationKind, err)
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
			if err := observeRun(ctx, observer, RunObservation{
				Kind:            ObservationRunTerminal,
				RunID:           run.RunID,
				GraphID:         run.GraphID,
				GraphVersion:    run.GraphVersion,
				NodeID:          node.ID,
				State:           run.State,
				TransitionCount: run.TransitionCount,
			}); err != nil {
				return fmt.Errorf("record terminal state: %w", err)
			}
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
		if err := observeRun(ctx, observer, RunObservation{
			Kind:            ObservationNodeCompleted,
			RunID:           run.RunID,
			GraphID:         run.GraphID,
			GraphVersion:    run.GraphVersion,
			NodeID:          node.ID,
			Outcome:         result.Outcome,
			State:           run.State,
			TransitionCount: run.TransitionCount,
			Evidence:        append([]string(nil), result.Evidence...),
		}); err != nil {
			run.State = RunFailed
			return fmt.Errorf("record node completion: %w", err)
		}

		next, err := ResolveTransition(graph, node.ID, result.Outcome)
		if err != nil {
			run.State = RunFailed
			return err
		}
		from := run.CurrentNode
		run.CurrentNode = next
		run.TransitionCount++
		if err := observeRun(ctx, observer, RunObservation{
			Kind:            ObservationTransitioned,
			RunID:           run.RunID,
			GraphID:         run.GraphID,
			GraphVersion:    run.GraphVersion,
			FromNode:        from,
			ToNode:          next,
			Outcome:         result.Outcome,
			State:           run.State,
			TransitionCount: run.TransitionCount,
		}); err != nil {
			run.State = RunFailed
			return fmt.Errorf("record transition: %w", err)
		}
	}
}

func observeRun(ctx context.Context, observer RunObserver, observation RunObservation) error {
	if observer == nil {
		return nil
	}
	return observer.ObserveRun(ctx, observation)
}
