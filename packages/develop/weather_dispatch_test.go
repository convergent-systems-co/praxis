package develop

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/state"
)

const testBootstrapDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type weatherIssuedRoute struct {
	want      inference.DispatchBinding
	candidate inference.DispatchCandidate
	calls     int
	failAfter int
}

func (r *weatherIssuedRoute) PrepareDispatch(_ context.Context, got inference.DispatchBinding) (inference.DispatchCandidate, error) {
	r.calls++
	if got != r.want {
		return inference.DispatchCandidate{}, errors.New("binding mismatch")
	}
	if r.failAfter > 0 && r.calls >= r.failAfter {
		return inference.DispatchCandidate{}, errors.New("route revoked")
	}
	return r.candidate, nil
}

type weatherRequestID string

func (r weatherRequestID) ResolveIssuedInferenceRequestID(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, agent.ExecutionContext) (string, error) {
	return string(r), nil
}

type weatherInferenceExecutor struct {
	calls *int
	err   error
}

func (e weatherInferenceExecutor) ID() string         { return "executor:local" }
func (e weatherInferenceExecutor) ProviderID() string { return "provider:local" }
func (e weatherInferenceExecutor) ExecuteRoutedInference(context.Context, kernel.GraphDef, kernel.NodeDef, *kernel.RunExecution, agent.ExecutionContext) (agent.InferenceExecution, error) {
	*e.calls++
	return agent.InferenceExecution{Result: kernel.NodeResult{Outcome: "done", Evidence: []string{"artifact:weather"}}}, e.err
}

func TestWeatherDispatchConsumesExactAuthorityInvokesOnceAndPersistsOutcome(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	graph := Graph()
	node := kernel.NodeDef{ID: "implement", Class: kernel.NodeInference}
	run := &kernel.RunExecution{RunID: "run:weather"}
	execution := agent.ExecutionContext{AgentID: "agent:weather", GenerationID: "7", GoalRef: "goal:weather"}
	binding := inference.DispatchBinding{RequestID: "request:weather", SubjectAgentID: execution.AgentID, AgentGeneration: execution.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: execution.GoalRef}
	candidate := inference.DispatchCandidate{RouteRecordID: "route:weather", RequestID: binding.RequestID, SurfaceID: "surface:local", ExecutorID: "executor:local", ProviderID: "provider:local", WorkContext: weatherDispatchScope, TargetScope: weatherDispatchScope}
	routes := &weatherIssuedRoute{want: binding, candidate: candidate}
	authority := WeatherDispatchAuthority{Store: state.New(db)}
	governance := &goalstore.Repository{Store: authority.Store, BootstrapDigest: testBootstrapDigest}
	issuer, err := NewWeatherDispatchIssuer(authority.Store, governance)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := issuer.Issue(ctx, routes, binding, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	routed := agent.IssuedRoutedAgentExecutor{Requests: weatherRequestID(binding.RequestID), Routes: routes, Authority: authority, Executors: map[string]agent.RoutedInferenceExecutor{"executor:local": weatherInferenceExecutor{calls: &calls}}}
	result, err := routed.ExecuteAgentNode(ctx, graph, node, run, execution)
	if err != nil || result.Outcome != "done" || calls != 1 {
		t.Fatalf("result=%#v calls=%d err=%v", result, calls, err)
	}
	if routes.calls != 3 {
		t.Fatalf("expected issuance plus initial and immediate replay, got %d", routes.calls)
	}
	var effectState string
	if err := db.QueryRowContext(ctx, `SELECT state FROM effects WHERE approval_id=? AND capability_lease_id=?`, grant.ApprovalID, grant.LeaseID).Scan(&effectState); err != nil {
		t.Fatal(err)
	}
	if effectState != "succeeded" {
		t.Fatalf("effect state %q", effectState)
	}
	if _, err := routed.ExecuteAgentNode(ctx, graph, node, run, execution); err == nil || calls != 1 {
		t.Fatalf("replay invoked executor: calls=%d err=%v", calls, err)
	}
	if !reflect.DeepEqual(result.Evidence, []string{"artifact:weather", "route:weather", "effect:dispatch:" + grant.IntentDigest[len("sha256:"):]}) {
		t.Fatalf("evidence %#v", result.Evidence)
	}
}

func TestWeatherDispatchFailsClosedOnFinalRouteReplayAndRecordsUnknownExecutorError(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	// The two cases are kept explicit so their durable state remains independent.
	for _, unknown := range []bool{false, true} {
		t.Run(map[bool]string{false: "route-revoked", true: "executor-unknown"}[unknown], func(t *testing.T) {
			db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := state.New(db)
			authority := WeatherDispatchAuthority{Store: store}
			graph := Graph()
			node := kernel.NodeDef{ID: "implement", Class: kernel.NodeInference}
			run := &kernel.RunExecution{RunID: "run:weather"}
			execution := agent.ExecutionContext{AgentID: "agent:weather", GenerationID: "7", GoalRef: "goal:weather"}
			binding := inference.DispatchBinding{RequestID: "request:weather", SubjectAgentID: execution.AgentID, AgentGeneration: execution.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: execution.GoalRef}
			candidate := inference.DispatchCandidate{RouteRecordID: "route:weather", RequestID: binding.RequestID, SurfaceID: "surface:local", ExecutorID: "executor:local", ProviderID: "provider:local", WorkContext: weatherDispatchScope, TargetScope: weatherDispatchScope}
			routes := &weatherIssuedRoute{want: binding, candidate: candidate}
			governance := &goalstore.Repository{Store: store, BootstrapDigest: testBootstrapDigest}
			issuer, err := NewWeatherDispatchIssuer(store, governance)
			if err != nil {
				t.Fatal(err)
			}
			grant, err := issuer.Issue(ctx, routes, binding, now.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			execErr := error(nil)
			if unknown {
				execErr = errors.New("connection lost")
			} else {
				routes.failAfter = 3
			}
			routed := agent.IssuedRoutedAgentExecutor{Requests: weatherRequestID(binding.RequestID), Routes: routes, Authority: authority, Executors: map[string]agent.RoutedInferenceExecutor{"executor:local": weatherInferenceExecutor{calls: &calls, err: execErr}}}
			if _, err := routed.ExecuteAgentNode(ctx, graph, node, run, execution); err == nil {
				t.Fatal("expected denial/error")
			}
			if !unknown && calls != 0 {
				t.Fatal("revoked route reached executor")
			}
			if unknown {
				var got string
				if err := db.QueryRowContext(ctx, `SELECT state FROM effects WHERE approval_id=?`, grant.ApprovalID).Scan(&got); err != nil {
					t.Fatal(err)
				}
				if got != "unknown" {
					t.Fatalf("state %q", got)
				}
			}
		})
	}
}
