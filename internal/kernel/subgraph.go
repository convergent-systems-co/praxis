package kernel

import (
	"context"
	"errors"
	"fmt"
)

// SubgraphOutcomeContract maps child terminal state to a parent node outcome.
// It is explicit runtime configuration; graph/node names never imply semantics.
type SubgraphOutcomeContract struct {
	Succeeded string
	Failed    string
	Cancelled string
}

func (c SubgraphOutcomeContract) Validate() error {
	if c.Succeeded == "" || c.Failed == "" || c.Cancelled == "" {
		return errors.New("subgraph success, failure, and cancellation outcomes are required")
	}
	return nil
}

// SubgraphRuntime owns execution/recovery of child graphs. Implementations may
// be local or remote, but must bind the exact SubgraphRef and preserve parent
// authority/cancellation semantics. The kernel does not infer child identity.
type SubgraphRuntime interface {
	ExecuteSubgraph(ctx context.Context, parentGraph GraphDef, parentNode NodeDef, parentRun *RunExecution, ref SubgraphRef) (RunState, []string, *Suspension, error)
}

// ComposingExecutor wraps the normal node executor and dispatches NodeSubgraph
// through a dedicated runtime. This keeps subgraph execution out of arbitrary
// inference/capability executors.
type ComposingExecutor struct {
	Delegate  NodeExecutor
	Subgraphs SubgraphRuntime
	Outcomes  map[string]SubgraphOutcomeContract
}

func (e ComposingExecutor) ExecuteNode(ctx context.Context, graph GraphDef, node NodeDef, run *RunExecution) (NodeResult, error) {
	if node.Class != NodeSubgraph {
		if e.Delegate == nil {
			return NodeResult{}, errors.New("node delegate is required")
		}
		return e.Delegate.ExecuteNode(ctx, graph, node, run)
	}
	if node.Subgraph == nil {
		return NodeResult{}, &ExecutionError{Class: FailureValidation, Err: errors.New("subgraph node is missing graph reference")}
	}
	if e.Subgraphs == nil {
		return NodeResult{}, &ExecutionError{Class: FailureValidation, Err: errors.New("subgraph runtime is required")}
	}
	key := subgraphKey(*node.Subgraph)
	contract, ok := e.Outcomes[key]
	if !ok {
		return NodeResult{}, &ExecutionError{Class: FailureValidation, Err: fmt.Errorf("no outcome contract for subgraph %s", key)}
	}
	if err := contract.Validate(); err != nil {
		return NodeResult{}, &ExecutionError{Class: FailureValidation, Err: err}
	}
	state, evidence, suspension, err := e.Subgraphs.ExecuteSubgraph(ctx, graph, node, run, *node.Subgraph)
	if err != nil {
		return NodeResult{}, err
	}
	if suspension != nil {
		return NodeResult{Suspension: suspension}, nil
	}
	switch state {
	case RunSucceeded:
		return NodeResult{Outcome: contract.Succeeded, Evidence: append([]string(nil), evidence...)}, nil
	case RunFailed:
		return NodeResult{Outcome: contract.Failed, Evidence: append([]string(nil), evidence...)}, nil
	case RunCancelled:
		return NodeResult{Outcome: contract.Cancelled, Evidence: append([]string(nil), evidence...)}, nil
	default:
		return NodeResult{}, &ExecutionError{Class: FailureValidation, Err: fmt.Errorf("subgraph returned nonterminal state %q without suspension", state)}
	}
}

func subgraphKey(ref SubgraphRef) string {
	return ref.GraphID + "@" + ref.GraphVersion
}
