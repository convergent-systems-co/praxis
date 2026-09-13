package agent_test

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/internal/stateprovider"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fixedGraphs struct{ graph agent.OperationalGraph }

func (f fixedGraphs) ResolveOperationalGraph(_ context.Context, ref string) (agent.OperationalGraph, error) {
	return f.graph, nil
}

type fixedMemory struct{ records []agent.MemoryRecord }

func (f fixedMemory) RetrieveMemory(_ context.Context, agentID, scope, goalRef string, limit int) ([]agent.MemoryRecord, error) {
	return append([]agent.MemoryRecord(nil), f.records...), nil
}

type recordingExecutor struct {
	id       string
	visited  *[]string
	contexts *[]agent.ExecutionContext
}

func (e recordingExecutor) ID() string { return e.id }
func (e recordingExecutor) ExecuteAgentNode(_ context.Context, _ kernel.GraphDef, node kernel.NodeDef, _ *kernel.RunExecution, c agent.ExecutionContext) (kernel.NodeResult, error) {
	*e.visited = append(*e.visited, node.ID)
	*e.contexts = append(*e.contexts, c)
	return kernel.NodeResult{Outcome: "next", Evidence: []string{"executed:" + node.ID}}, nil
}

func operationalFixture() agent.OperationalGraph {
	roles := map[agent.OperationalRole]string{agent.RoleReceiveGoal: "receive", agent.RoleRetrieveMemory: "memory", agent.RoleResolveContext: "context", agent.RoleAct: "act", agent.RoleEvaluateEvidence: "evidence", agent.RoleReflect: "reflect", agent.RoleProposeLearning: "learn"}
	nodes := []kernel.NodeDef{{ID: "receive", Class: kernel.NodeDeterministic}, {ID: "memory", Class: kernel.NodeDeterministic}, {ID: "context", Class: kernel.NodeDeterministic}, {ID: "act", Class: kernel.NodeCapability}, {ID: "evidence", Class: kernel.NodeDeterministic}, {ID: "reflect", Class: kernel.NodeInference}, {ID: "learn", Class: kernel.NodeDeterministic}, {ID: "done", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded}}
	var transitions []kernel.TransitionDef
	for i := 0; i < len(nodes)-1; i++ {
		transitions = append(transitions, kernel.TransitionDef{From: nodes[i].ID, Outcome: "next", To: nodes[i+1].ID})
	}
	return agent.OperationalGraph{Definition: kernel.GraphDef{ID: "agent.operations", Version: "2", EntryNode: "receive", Nodes: nodes, Transitions: transitions, MaxTransitions: 8}, Roles: roles}
}

func TestPersistentAgentExecutesOperationalGraphAcrossRestartAndProviderReplacement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	now := time.Date(2026, 9, 13, 20, 0, 0, 0, time.UTC)
	provider, err := stateprovider.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	identity := agent.Agent{ID: "agent-1", OwnerScope: "human:1", CurrentGeneration: "generation-1", Lifecycle: agent.AgentActive, CreatedAt: now}
	generation := agent.Generation{ID: "generation-1", AgentID: "agent-1", Number: 1, GraphRefs: []string{"agent.operations@2"}, CreationReason: "fixture", GovernanceRef: "governance:create", CreatedAt: now}
	memory := agent.MemoryRecord{ID: "memory-1", AgentID: "agent-1", Scope: "project:praxis", Type: agent.MemoryFact, ContentRef: "fact:original-goal", Provenance: contracts.ProvenanceRef{SourceType: "observation", Trust: contracts.TrustObserved, ObservedAt: now}, Trust: contracts.TrustObserved, Confidence: 1, CreatedAt: now}
	runtimeA := agent.Runtime{Events: provider.Events(), Graphs: fixedGraphs{operationalFixture()}, Memory: fixedMemory{[]agent.MemoryRecord{memory}}, MaxMemory: 2, Now: func() time.Time { return now }}
	if err := runtimeA.Create(ctx, identity, generation, contracts.PrincipalRef{ID: "human-1", Kind: "user"}, "create-agent-1"); err != nil {
		t.Fatal(err)
	}
	var visitedA []string
	var contextsA []agent.ExecutionContext
	first, err := runtimeA.Execute(ctx, agent.ExecuteRequest{AgentID: "agent-1", RunID: "run-a", GoalRef: "goal:one", Scope: "project:praxis", Executor: recordingExecutor{id: "provider-a", visited: &visitedA, contexts: &contextsA}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Run.State != kernel.RunSucceeded || first.ExecutorID != "provider-a" || first.GenerationID != "generation-1" {
		t.Fatalf("unexpected first execution: %#v", first)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := stateprovider.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	runtimeB := agent.Runtime{Events: restarted.Events(), Graphs: fixedGraphs{operationalFixture()}, Memory: fixedMemory{[]agent.MemoryRecord{memory}}, MaxMemory: 2, Now: func() time.Time { return now.Add(time.Minute) }}
	var visitedB []string
	var contextsB []agent.ExecutionContext
	second, err := runtimeB.Execute(ctx, agent.ExecuteRequest{AgentID: "agent-1", RunID: "run-b", GoalRef: "goal:two", Scope: "project:praxis", Executor: recordingExecutor{id: "provider-b", visited: &visitedB, contexts: &contextsB}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"receive", "memory", "context", "act", "evidence", "reflect", "learn"}
	if !reflect.DeepEqual(visitedA, want) || !reflect.DeepEqual(visitedB, want) {
		t.Fatalf("operational roles were not executed: first=%v second=%v", visitedA, visitedB)
	}
	if second.ExecutorID != "provider-b" || second.AgentID != first.AgentID || second.GenerationID != first.GenerationID || second.GraphVersion != "2" {
		t.Fatalf("identity changed with provider/restart: first=%#v second=%#v", first, second)
	}
	if len(contextsB) != len(want) || len(contextsB[0].Memory) != 1 || contextsB[0].GoalRef != "goal:two" {
		t.Fatalf("goal/memory/context did not reach operational graph: %#v", contextsB)
	}
}

func TestOperationalGraphFailsClosedWhenAgentRoleIsMissing(t *testing.T) {
	graph := operationalFixture()
	delete(graph.Roles, agent.RoleProposeLearning)
	if err := graph.Validate(); err == nil {
		t.Fatal("agent operational graph accepted missing learning role")
	}
}
