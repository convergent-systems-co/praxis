package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type OperationalRole string

const (
	RoleReceiveGoal      OperationalRole = "receive_goal"
	RoleRetrieveMemory   OperationalRole = "retrieve_memory"
	RoleResolveContext   OperationalRole = "resolve_context"
	RoleAct              OperationalRole = "act"
	RoleEvaluateEvidence OperationalRole = "evaluate_evidence"
	RoleReflect          OperationalRole = "reflect"
	RoleProposeLearning  OperationalRole = "propose_learning"
)

var requiredOperationalRoles = []OperationalRole{RoleReceiveGoal, RoleRetrieveMemory, RoleResolveContext, RoleAct, RoleEvaluateEvidence, RoleReflect, RoleProposeLearning}

type OperationalGraph struct {
	Definition kernel.GraphDef
	Roles      map[OperationalRole]string
}

func (g OperationalGraph) Validate() error {
	if err := g.Definition.Validate(); err != nil {
		return err
	}
	nodes := map[string]bool{}
	for _, node := range g.Definition.Nodes {
		nodes[node.ID] = true
	}
	for _, role := range requiredOperationalRoles {
		nodeID := g.Roles[role]
		if nodeID == "" || !nodes[nodeID] {
			return fmt.Errorf("operational graph role %s is not bound to a node", role)
		}
	}
	return nil
}

type GraphResolver interface {
	ResolveOperationalGraph(ctx context.Context, ref string) (OperationalGraph, error)
}
type MemoryRetriever interface {
	RetrieveMemory(ctx context.Context, agentID, scope, goalRef string, limit int) ([]MemoryRecord, error)
}

type ExecutionContext struct {
	AgentID      string
	GenerationID string
	GoalRef      string
	Scope        string
	Memory       []MemoryRecord
}

type AgentNodeExecutor interface {
	ID() string
	ExecuteAgentNode(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution, agent ExecutionContext) (kernel.NodeResult, error)
}

type Runtime struct {
	Events    eventstore.Store
	Graphs    GraphResolver
	Memory    MemoryRetriever
	MaxMemory int
	Now       func() time.Time
}

type persistedIdentity struct {
	Agent      Agent      `json:"agent"`
	Generation Generation `json:"generation"`
}

func (r Runtime) Create(ctx context.Context, a Agent, g Generation, actor contracts.PrincipalRef, commandID string) error {
	if r.Events == nil {
		return errors.New("agent runtime event store is required")
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if err := g.Validate(); err != nil {
		return err
	}
	if a.ID != g.AgentID || a.CurrentGeneration != g.ID {
		return errors.New("agent and generation identity mismatch")
	}
	if err := actor.Validate(); err != nil {
		return err
	}
	if commandID == "" {
		return errors.New("create command id is required")
	}
	payload, err := json.Marshal(persistedIdentity{a, g})
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	_, err = r.Events.Append(ctx, a.ID, 0, []eventstore.Event{{ID: a.ID + ":generation:" + g.ID, AggregateType: "agent", Type: "agent.created", Version: "1", Actor: actor, CommandID: commandID, CorrelationID: commandID, Trust: contracts.TrustUserConfirmed, Payload: payload, CreatedAt: now}})
	return err
}

func (r Runtime) Load(ctx context.Context, agentID string) (Agent, Generation, int64, error) {
	if r.Events == nil || agentID == "" {
		return Agent{}, Generation{}, 0, errors.New("agent runtime store and identity are required")
	}
	events, err := r.Events.LoadAggregate(ctx, agentID, 0)
	if err != nil {
		return Agent{}, Generation{}, 0, err
	}
	if len(events) == 0 {
		return Agent{}, Generation{}, 0, errors.New("agent not found")
	}
	var identity persistedIdentity
	var version int64
	for i, event := range events {
		if event.AggregateType != "agent" {
			return Agent{}, Generation{}, 0, fmt.Errorf("agent event %d has wrong aggregate type", i)
		}
		switch event.Type {
		case "agent.created":
			if i != 0 {
				return Agent{}, Generation{}, 0, errors.New("agent created event is not first")
			}
			if err := json.Unmarshal(event.Payload, &identity); err != nil {
				return Agent{}, Generation{}, 0, err
			}
		default:
			return Agent{}, Generation{}, 0, fmt.Errorf("unknown agent event %s", event.Type)
		}
		version = event.AggregateVersion
	}
	if err := identity.Agent.Validate(); err != nil {
		return Agent{}, Generation{}, 0, err
	}
	if err := identity.Generation.Validate(); err != nil {
		return Agent{}, Generation{}, 0, err
	}
	if identity.Agent.ID != agentID || identity.Generation.ID != identity.Agent.CurrentGeneration {
		return Agent{}, Generation{}, 0, errors.New("persisted agent projection is inconsistent")
	}
	return identity.Agent, identity.Generation, version, nil
}

type ExecuteRequest struct {
	AgentID  string
	RunID    string
	GoalRef  string
	Scope    string
	Executor AgentNodeExecutor
}
type ExecuteResult struct {
	AgentID      string
	GenerationID string
	GraphID      string
	GraphVersion string
	ExecutorID   string
	MemoryIDs    []string
	Run          kernel.RunExecution
}

func (r Runtime) Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	if r.Graphs == nil || r.Memory == nil || req.Executor == nil {
		return ExecuteResult{}, errors.New("graph resolver, memory retriever, and executor are required")
	}
	if req.RunID == "" || req.GoalRef == "" || req.Scope == "" || req.Executor.ID() == "" {
		return ExecuteResult{}, errors.New("run, goal, scope, and executor identity are required")
	}
	a, g, _, err := r.Load(ctx, req.AgentID)
	if err != nil {
		return ExecuteResult{}, err
	}
	if a.Lifecycle != AgentActive {
		return ExecuteResult{}, errors.New("agent is not active")
	}
	if len(g.GraphRefs) == 0 {
		return ExecuteResult{}, errors.New("agent generation has no operational graph")
	}
	graph, err := r.Graphs.ResolveOperationalGraph(ctx, g.GraphRefs[0])
	if err != nil {
		return ExecuteResult{}, err
	}
	if err := graph.Validate(); err != nil {
		return ExecuteResult{}, err
	}
	if g.GraphRefs[0] != graph.Definition.ID+"@"+graph.Definition.Version {
		return ExecuteResult{}, errors.New("resolved operational graph identity/version mismatch")
	}
	limit := r.MaxMemory
	if limit <= 0 {
		limit = 16
	}
	memories, err := r.Memory.RetrieveMemory(ctx, a.ID, req.Scope, req.GoalRef, limit)
	if err != nil {
		return ExecuteResult{}, err
	}
	if len(memories) > limit {
		return ExecuteResult{}, errors.New("memory retriever exceeded bounded context limit")
	}
	for _, memory := range memories {
		if memory.AgentID != a.ID || !memory.Active(time.Now().UTC()) {
			return ExecuteResult{}, errors.New("memory retrieval returned invalid or foreign record")
		}
	}
	agentContext := ExecutionContext{AgentID: a.ID, GenerationID: g.ID, GoalRef: req.GoalRef, Scope: req.Scope, Memory: append([]MemoryRecord(nil), memories...)}
	run := kernel.RunExecution{RunID: req.RunID}
	journal := &kernel.EventJournal{Store: r.Events, Actor: contracts.PrincipalRef{ID: a.ID, Kind: "agent"}, CommandID: "agent-run:" + req.RunID, CorrelationID: req.RunID, Trust: contracts.TrustObserved, Now: r.Now}
	if err := kernel.RunObserved(ctx, graph.Definition, &run, agentExecutorAdapter{executor: req.Executor, context: agentContext}, journal); err != nil {
		return ExecuteResult{}, err
	}
	result := ExecuteResult{AgentID: a.ID, GenerationID: g.ID, GraphID: graph.Definition.ID, GraphVersion: graph.Definition.Version, ExecutorID: req.Executor.ID(), Run: run}
	for _, memory := range memories {
		result.MemoryIDs = append(result.MemoryIDs, memory.ID)
	}
	return result, nil
}

type agentExecutorAdapter struct {
	executor AgentNodeExecutor
	context  ExecutionContext
}

func (a agentExecutorAdapter) ExecuteNode(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution) (kernel.NodeResult, error) {
	return a.executor.ExecuteAgentNode(ctx, graph, node, run, a.context)
}
