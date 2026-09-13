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
	runtimeA := agent.Runtime{Events: provider.Events(), Graphs: fixedGraphs{operationalFixture()}, MaxMemory: 2, Now: func() time.Time { return now }}
	runtimeA.Memory = runtimeA
	if err := runtimeA.Create(ctx, identity, generation, contracts.PrincipalRef{ID: "human-1", Kind: "user"}, "create-agent-1"); err != nil {
		t.Fatal(err)
	}
	if err := runtimeA.Remember(ctx, memory, contracts.PrincipalRef{ID: "human-1", Kind: "user"}, "remember-1"); err != nil {
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
	runtimeB := agent.Runtime{Events: restarted.Events(), Graphs: fixedGraphs{operationalFixture()}, MaxMemory: 2, Now: func() time.Time { return now.Add(time.Minute) }}
	runtimeB.Memory = runtimeB
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
	if len(contextsB) != len(want) {
		t.Fatalf("goal/memory/context did not reach operational graph: %#v", contextsB)
	}
	for _, executionContext := range contextsB {
		if executionContext.AgentID != "agent-1" || executionContext.GenerationID != "generation-1" || executionContext.GoalRef != "goal:two" || executionContext.Scope != "project:praxis" || len(executionContext.Memory) != 1 || executionContext.Memory[0].ID != "memory-1" {
			t.Fatalf("exact agent context did not reach every operational node: %#v", executionContext)
		}
	}
}

func TestAgentGenerationIntrospectionAndRollbackPreserveHistoryAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	now := time.Date(2026, 9, 13, 21, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "governance", Kind: "service"}
	provider, err := stateprovider.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := agent.Runtime{Events: provider.Events(), Now: func() time.Time { return now }}
	identity := agent.Agent{ID: "agent-lineage", OwnerScope: "human:1", CurrentGeneration: "g1", Lifecycle: agent.AgentActive, CreatedAt: now}
	g1 := agent.Generation{ID: "g1", AgentID: identity.ID, Number: 1, GraphRefs: []string{"agent.operations@1"}, CreationReason: "seed", GovernanceRef: "create", CreatedAt: now}
	if err := runtime.Create(ctx, identity, g1, actor, "create-lineage"); err != nil {
		t.Fatal(err)
	}
	g2 := agent.Generation{ID: "g2", AgentID: identity.ID, Number: 2, ParentGeneration: "g1", GraphRefs: []string{"agent.operations@2"}, LearningRefs: []string{"finding:improve"}, CreationReason: "promoted learning", GovernanceRef: "promotion:1", CreatedAt: now.Add(time.Minute)}
	if err := runtime.PromoteGeneration(ctx, identity.ID, g2, actor, "promote-g2"); err != nil {
		t.Fatal(err)
	}
	g3 := agent.Generation{ID: "g3", AgentID: identity.ID, Number: 3, ParentGeneration: "g2", GraphRefs: append([]string(nil), g1.GraphRefs...), LearningRefs: []string{"rollback:g1"}, CreationReason: "rollback to generation 1 behavior", GovernanceRef: "rollback:1", CreatedAt: now.Add(2 * time.Minute)}
	if err := runtime.PromoteGeneration(ctx, identity.ID, g3, actor, "rollback-g1"); err != nil {
		t.Fatal(err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := stateprovider.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	inspection, err := (agent.Runtime{Events: restarted.Events()}).Inspect(ctx, identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.CurrentGeneration.ID != "g3" || inspection.Agent.CurrentGeneration != "g3" {
		t.Fatalf("runtime introspection lost lineage: %#v", inspection)
	}
	historyByID := map[string]agent.Generation{}
	for _, historical := range inspection.GenerationHistory {
		historyByID[historical.ID] = historical
	}
	if historyByID["g1"].ParentGeneration != "" || historyByID["g2"].ParentGeneration != "g1" || historyByID["g3"].ParentGeneration != "g2" || !reflect.DeepEqual(historyByID["g3"].GraphRefs, g1.GraphRefs) {
		t.Fatal("rollback deleted or rewrote generation history")
	}
}

func TestPersistentMemoryRetrievalIsScopedBoundedAndSupersessionAware(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	now := time.Date(2026, 9, 13, 22, 0, 0, 0, time.UTC)
	actor := contracts.PrincipalRef{ID: "human", Kind: "user"}
	provider, err := stateprovider.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	runtime := agent.Runtime{Events: provider.Events(), Now: func() time.Time { return now }}
	identity := agent.Agent{ID: "memory-agent", OwnerScope: "human", CurrentGeneration: "g1", Lifecycle: agent.AgentActive, CreatedAt: now}
	generation := agent.Generation{ID: "g1", AgentID: identity.ID, Number: 1, GraphRefs: []string{"agent.operations@1"}, CreationReason: "seed", GovernanceRef: "create", CreatedAt: now}
	if err := runtime.Create(ctx, identity, generation, actor, "create-memory-agent"); err != nil {
		t.Fatal(err)
	}
	provenance := contracts.ProvenanceRef{SourceType: "observation", Trust: contracts.TrustObserved, ObservedAt: now}
	records := []agent.MemoryRecord{{ID: "old", AgentID: identity.ID, Scope: "project:a", Type: agent.MemoryFact, ContentRef: "fact:old", Provenance: provenance, Trust: contracts.TrustObserved, Confidence: .9, CreatedAt: now}, {ID: "replacement", AgentID: identity.ID, Scope: "project:a", Type: agent.MemoryFact, ContentRef: "fact:new", Provenance: provenance, Trust: contracts.TrustObserved, Confidence: .8, CreatedAt: now.Add(time.Second)}, {ID: "foreign-scope", AgentID: identity.ID, Scope: "project:b", Type: agent.MemoryFact, ContentRef: "fact:foreign", Provenance: provenance, Trust: contracts.TrustObserved, Confidence: 1, CreatedAt: now}, {ID: "global", AgentID: identity.ID, Scope: "global", Type: agent.MemoryContext, ContentRef: "context:global", Provenance: provenance, Trust: contracts.TrustObserved, Confidence: .2, CreatedAt: now}}
	for index, record := range records {
		if err := runtime.Remember(ctx, record, actor, "remember-"+record.ID); err != nil {
			t.Fatalf("remember %d: %v", index, err)
		}
	}
	if err := runtime.SupersedeMemory(ctx, identity.ID, "old", "replacement", actor, "supersede-old"); err != nil {
		t.Fatal(err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := stateprovider.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	runtime = agent.Runtime{Events: restarted.Events(), Now: func() time.Time { return now.Add(time.Minute) }}
	got, err := runtime.RetrieveMemory(ctx, identity.ID, "project:a", "goal:any", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "replacement" {
		t.Fatalf("retrieval was not bounded/scoped/supersession-aware: %#v", got)
	}
	inspection, err := runtime.Inspect(ctx, identity.ID)
	if err != nil {
		t.Fatal(err)
	}
	memoryByID := map[string]agent.MemoryRecord{}
	for _, memory := range inspection.Memory {
		memoryByID[memory.ID] = memory
	}
	for _, record := range records {
		if _, ok := memoryByID[record.ID]; !ok {
			t.Fatalf("memory reconstruction lost required record %s", record.ID)
		}
	}
	if memoryByID["old"].SupersededBy != "replacement" || memoryByID["replacement"].SupersededBy != "" || memoryByID["foreign-scope"].Scope != "project:b" || memoryByID["global"].Scope != "global" {
		t.Fatalf("memory reconstruction changed semantic identity, scope, or supersession: %#v", memoryByID)
	}
}

func TestOperationalGraphFailsClosedWhenAgentRoleIsMissing(t *testing.T) {
	graph := operationalFixture()
	delete(graph.Roles, agent.RoleProposeLearning)
	if err := graph.Validate(); err == nil {
		t.Fatal("agent operational graph accepted missing learning role")
	}
}
