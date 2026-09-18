package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
)

type fixedIssuedRequestID string

func (r fixedIssuedRequestID) ResolveIssuedInferenceRequestID(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, ExecutionContext) (string, error) {
	return string(r), nil
}

type fixedDispatchCandidate struct {
	want      inference.DispatchBinding
	candidate inference.DispatchCandidate
	err       error
}

func (l fixedDispatchCandidate) PrepareDispatch(_ context.Context, got inference.DispatchBinding) (inference.DispatchCandidate, error) {
	if got != l.want {
		return inference.DispatchCandidate{}, errors.New("active work binding mismatch")
	}
	return l.candidate, l.err
}

type neverDispatchedExecutor struct{ calls *int }

func (e neverDispatchedExecutor) ID() string         { return "executor:local" }
func (e neverDispatchedExecutor) ProviderID() string { return "provider:local" }
func (e neverDispatchedExecutor) ExecuteRoutedInference(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, ExecutionContext) (InferenceExecution, error) {
	*e.calls++
	return InferenceExecution{}, nil
}

func TestIssuedRoutedInferenceStopsAtSeparateDispatchAuthorityBoundary(t *testing.T) {
	graph := kernel.GraphDef{ID: "praxis.package.develop.default", Version: "0.2.0"}
	node := kernel.NodeDef{ID: "implement", Class: kernel.NodeInference}
	run := &kernel.RunExecution{RunID: "run:weather"}
	agent := ExecutionContext{AgentID: "agent:weather", GenerationID: "7", GoalRef: "goal:weather"}
	want := inference.DispatchBinding{RequestID: "request:weather", SubjectAgentID: agent.AgentID, AgentGeneration: agent.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: agent.GoalRef}
	candidate := inference.DispatchCandidate{RouteRecordID: "route:weather", RequestID: want.RequestID, SurfaceID: "surface:local", ExecutorID: "executor:local", ProviderID: "provider:local", WorkContext: "develop:weather-dashboard", TargetScope: "develop:weather-dashboard"}
	calls := 0
	executor := IssuedRoutedAgentExecutor{Requests: fixedIssuedRequestID(want.RequestID), Routes: fixedDispatchCandidate{want: want, candidate: candidate}, Executors: map[string]RoutedInferenceExecutor{"executor:local": neverDispatchedExecutor{calls: &calls}}}
	_, err := executor.ExecuteAgentNode(context.Background(), graph, node, run, agent)
	if !errors.Is(err, ErrRoutedInferenceDispatchAuthorizationRequired) || calls != 0 {
		t.Fatalf("qualification crossed dispatch boundary: calls=%d err=%v", calls, err)
	}
}

func TestIssuedRoutedInferenceRejectsFabricatedOrMismatchedCandidateBeforeDispatch(t *testing.T) {
	graph := kernel.GraphDef{ID: "praxis.package.develop.default", Version: "0.2.0"}
	node := kernel.NodeDef{ID: "implement", Class: kernel.NodeInference}
	run := &kernel.RunExecution{RunID: "run:weather"}
	agent := ExecutionContext{AgentID: "agent:weather", GenerationID: "7", GoalRef: "goal:weather"}
	want := inference.DispatchBinding{RequestID: "request:weather", SubjectAgentID: agent.AgentID, AgentGeneration: agent.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: agent.GoalRef}
	calls := 0
	executor := IssuedRoutedAgentExecutor{Requests: fixedIssuedRequestID(want.RequestID), Routes: fixedDispatchCandidate{want: want, candidate: inference.DispatchCandidate{ExecutorID: "forged", ProviderID: "provider:forged"}}, Executors: map[string]RoutedInferenceExecutor{"executor:local": neverDispatchedExecutor{calls: &calls}}}
	if _, err := executor.ExecuteAgentNode(context.Background(), graph, node, run, agent); err == nil || calls != 0 {
		t.Fatalf("fabricated candidate reached dispatch: calls=%d err=%v", calls, err)
	}
	executor.Routes = fixedDispatchCandidate{want: want, err: errors.New("revoked issued route")}
	if _, err := executor.ExecuteAgentNode(context.Background(), graph, node, run, agent); err == nil || calls != 0 {
		t.Fatalf("revoked route reached dispatch: calls=%d err=%v", calls, err)
	}
}
