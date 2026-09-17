package agent

import (
	"context"
	"errors"

	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
)

var ErrRoutedInferenceDispatchAuthorizationRequired = errors.New("routed inference requires separate dispatch authorization")

// IssuedDispatchCandidateLoader loads routing evidence; it does not grant
// dispatch authority. Production composition uses IssuedRouteLedger.
type IssuedDispatchCandidateLoader interface {
	PrepareDispatch(context.Context, inference.DispatchBinding) (inference.DispatchCandidate, error)
}

type IssuedInferenceRequestResolver interface {
	ResolveIssuedInferenceRequestID(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, ExecutionContext) (string, error)
}

// IssuedRoutedAgentExecutor qualifies the production executor boundary against
// issued v2 routing evidence. It intentionally stops before executor invocation
// until a separately governed dispatch-authority boundary is composed.
type IssuedRoutedAgentExecutor struct {
	Delegate  AgentNodeExecutor
	Requests  IssuedInferenceRequestResolver
	Routes    IssuedDispatchCandidateLoader
	Executors map[string]RoutedInferenceExecutor
}

func (e IssuedRoutedAgentExecutor) ID() string { return "issued-governed-inference-router" }

func (e IssuedRoutedAgentExecutor) ExecuteAgentNode(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution, agent ExecutionContext) (kernel.NodeResult, error) {
	if node.Class != kernel.NodeInference {
		if e.Delegate == nil {
			return kernel.NodeResult{}, errors.New("ordinary graph node requires a delegate executor")
		}
		return e.Delegate.ExecuteAgentNode(ctx, graph, node, run, agent)
	}
	if e.Requests == nil || e.Routes == nil || len(e.Executors) == 0 {
		return kernel.NodeResult{}, errors.New("issued routed inference requires request resolver, issued-route ledger, and executor registry")
	}
	requestID, err := e.Requests.ResolveIssuedInferenceRequestID(ctx, graph, node, run, agent)
	if err != nil {
		return kernel.NodeResult{}, err
	}
	candidate, err := e.Routes.PrepareDispatch(ctx, inference.DispatchBinding{RequestID: requestID, SubjectAgentID: agent.AgentID, AgentGeneration: agent.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: agent.GoalRef})
	if err != nil {
		return kernel.NodeResult{}, err
	}
	if candidate.RequestID != requestID || candidate.RouteRecordID == "" || candidate.SurfaceID == "" || candidate.ExecutorID == "" || candidate.ProviderID == "" {
		return kernel.NodeResult{}, errors.New("issued dispatch candidate does not bind the active request and selected surface")
	}
	selected := e.Executors[candidate.ExecutorID]
	if selected == nil || selected.ID() != candidate.ExecutorID || selected.ProviderID() != candidate.ProviderID {
		return kernel.NodeResult{}, errors.New("issued selected executor/provider is unavailable at dispatch boundary")
	}
	return kernel.NodeResult{}, ErrRoutedInferenceDispatchAuthorizationRequired
}
