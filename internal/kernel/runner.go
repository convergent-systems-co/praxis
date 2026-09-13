package kernel

import (
	"context"
	"errors"
	"fmt"
	"time"
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
	AttemptCounts   map[string]int
}

type RunObservationKind string

const (
	ObservationRunStarted        RunObservationKind = "started"
	ObservationRunResumed        RunObservationKind = "resumed"
	ObservationRunStateChanged   RunObservationKind = "state_changed"
	ObservationNodeAttemptFailed RunObservationKind = "node_attempt_failed"
	ObservationNodeCompleted     RunObservationKind = "node_completed"
	ObservationTransitioned      RunObservationKind = "transitioned"
	ObservationRunTerminal       RunObservationKind = "terminal"
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
	Attempt         int                `json:"attempt,omitempty"`
	FailureClass    FailureClass       `json:"failure_class,omitempty"`
	Evidence        []string           `json:"evidence,omitempty"`
}

type RunObserver interface {
	ObserveRun(ctx context.Context, observation RunObservation) error
}

func Run(ctx context.Context, graph GraphDef, run *RunExecution, executor NodeExecutor) error {
	return RunObserved(ctx, graph, run, executor, nil)
}

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
	if run.AttemptCounts == nil {
		run.AttemptCounts = map[string]int{}
	}

	resuming := run.TransitionCount > 0 || len(run.AttemptCounts) > 0 || run.State == RunRunning || run.State == RunWaiting || run.State == RunSuspended || run.State == RunReconciling || run.State == RunCancelling
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
	if err := observeRun(ctx, observer, baseObservation(run, observationKind, run.CurrentNode)); err != nil {
		run.State = RunFailed
		return fmt.Errorf("record run %s: %w", observationKind, err)
	}

	nodes := make(map[string]NodeDef, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}

	for {
		if err := ctx.Err(); err != nil {
			if recordErr := recordTerminal(ctx, observer, run, RunCancelled, FailureCancellation); recordErr != nil {
				return fmt.Errorf("record cancellation: %w", recordErr)
			}
			return err
		}
		node, ok := nodes[run.CurrentNode]
		if !ok {
			err := fmt.Errorf("current node %q does not exist", run.CurrentNode)
			if recordErr := recordTerminal(ctx, observer, run, RunFailed, FailureValidation); recordErr != nil {
				return fmt.Errorf("%v; record failure: %w", err, recordErr)
			}
			return err
		}
		if node.Class == NodeTerminal {
			if err := recordTerminal(ctx, observer, run, node.TerminalState, ""); err != nil {
				return fmt.Errorf("record terminal state: %w", err)
			}
			return nil
		}
		if graph.MaxTransitions > 0 && run.TransitionCount >= graph.MaxTransitions {
			err := errors.New("graph transition bound exhausted")
			if recordErr := recordTerminal(ctx, observer, run, RunFailed, FailureResourceQuota); recordErr != nil {
				return fmt.Errorf("%v; record failure: %w", err, recordErr)
			}
			return err
		}

		result, attempt, err := executeNodeWithRetry(ctx, graph, node, run, executor, observer)
		if err != nil {
			class := FailureOf(err)
			state := RunFailed
			if class == FailureCancellation || errors.Is(err, context.Canceled) || (errors.Is(err, context.DeadlineExceeded) && ctx.Err() != nil) {
				state = RunCancelled
				class = FailureCancellation
			} else if class == FailureExternalEffectUnknown {
				if recordErr := recordStateChange(ctx, observer, run, RunReconciling, class); recordErr != nil {
					return fmt.Errorf("%v; record reconciliation state: %w", err, recordErr)
				}
				return err
			}
			if recordErr := recordTerminal(ctx, observer, run, state, class); recordErr != nil {
				return fmt.Errorf("%v; record terminal failure: %w", err, recordErr)
			}
			return fmt.Errorf("execute node %s attempt %d: %w", node.ID, attempt, err)
		}
		if result.Outcome == "" {
			err := &ExecutionError{Class: FailureValidation, Err: fmt.Errorf("node %s returned empty outcome", node.ID)}
			if recordErr := recordTerminal(ctx, observer, run, RunFailed, FailureValidation); recordErr != nil {
				return fmt.Errorf("%v; record failure: %w", err, recordErr)
			}
			return err
		}
		run.Evidence = append(run.Evidence, result.Evidence...)
		observation := baseObservation(run, ObservationNodeCompleted, node.ID)
		observation.Outcome = result.Outcome
		observation.Attempt = attempt
		observation.Evidence = append([]string(nil), result.Evidence...)
		if err := observeRun(ctx, observer, observation); err != nil {
			run.State = RunFailed
			return fmt.Errorf("record node completion: %w", err)
		}

		next, err := ResolveTransition(graph, node.ID, result.Outcome)
		if err != nil {
			if recordErr := recordTerminal(ctx, observer, run, RunFailed, FailureValidation); recordErr != nil {
				return fmt.Errorf("%v; record failure: %w", err, recordErr)
			}
			return err
		}
		from := run.CurrentNode
		run.CurrentNode = next
		run.TransitionCount++
		observation = baseObservation(run, ObservationTransitioned, "")
		observation.FromNode = from
		observation.ToNode = next
		observation.Outcome = result.Outcome
		if err := observeRun(ctx, observer, observation); err != nil {
			run.State = RunFailed
			return fmt.Errorf("record transition: %w", err)
		}
	}
}

func executeNodeWithRetry(ctx context.Context, graph GraphDef, node NodeDef, run *RunExecution, executor NodeExecutor, observer RunObserver) (NodeResult, int, error) {
	for {
		attempt := run.AttemptCounts[node.ID] + 1
		run.AttemptCounts[node.ID] = attempt
		result, err := executor.ExecuteNode(ctx, graph, node, run)
		if err == nil {
			return result, attempt, nil
		}
		class := FailureOf(err)
		observation := baseObservation(run, ObservationNodeAttemptFailed, node.ID)
		observation.Attempt = attempt
		observation.FailureClass = class
		if observeErr := observeRun(ctx, observer, observation); observeErr != nil {
			return NodeResult{}, attempt, fmt.Errorf("record failed attempt: %w", observeErr)
		}
		if !node.Retry.Allows(class, attempt) {
			return NodeResult{}, attempt, err
		}
		if err := waitRetry(ctx, time.Duration(node.Retry.BackoffMillis)*time.Millisecond); err != nil {
			return NodeResult{}, attempt, &ExecutionError{Class: FailureCancellation, Err: err}
		}
	}
}

func waitRetry(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func baseObservation(run *RunExecution, kind RunObservationKind, nodeID string) RunObservation {
	return RunObservation{
		Kind:            kind,
		RunID:           run.RunID,
		GraphID:         run.GraphID,
		GraphVersion:    run.GraphVersion,
		NodeID:          nodeID,
		State:           run.State,
		TransitionCount: run.TransitionCount,
	}
}

func recordStateChange(ctx context.Context, observer RunObserver, run *RunExecution, state RunState, class FailureClass) error {
	run.State = state
	observation := baseObservation(run, ObservationRunStateChanged, run.CurrentNode)
	observation.FailureClass = class
	return observeRun(ctx, observer, observation)
}

func recordTerminal(ctx context.Context, observer RunObserver, run *RunExecution, state RunState, class FailureClass) error {
	run.State = state
	observation := baseObservation(run, ObservationRunTerminal, run.CurrentNode)
	observation.FailureClass = class
	return observeRun(ctx, observer, observation)
}

func observeRun(ctx context.Context, observer RunObserver, observation RunObservation) error {
	if observer == nil {
		return nil
	}
	return observer.ObserveRun(ctx, observation)
}
