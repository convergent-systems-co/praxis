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
	inspection, err := r.Inspect(ctx, agentID)
	if err != nil {
		return Agent{}, Generation{}, 0, err
	}
	return inspection.Agent, inspection.CurrentGeneration, inspection.Version, nil
}

type AgentInspection struct {
	Agent             Agent          `json:"agent"`
	CurrentGeneration Generation     `json:"current_generation"`
	GenerationHistory []Generation   `json:"generation_history"`
	Memory            []MemoryRecord `json:"memory"`
	Version           int64          `json:"version"`
}

func (r Runtime) Inspect(ctx context.Context, agentID string) (AgentInspection, error) {
	if r.Events == nil || agentID == "" {
		return AgentInspection{}, errors.New("agent runtime store and identity are required")
	}
	events, err := r.Events.LoadAggregate(ctx, agentID, 0)
	if err != nil {
		return AgentInspection{}, err
	}
	if len(events) == 0 {
		return AgentInspection{}, errors.New("agent not found")
	}
	var identity persistedIdentity
	var generations []Generation
	var memories []MemoryRecord
	var version int64
	for i, event := range events {
		if event.AggregateType != "agent" {
			return AgentInspection{}, fmt.Errorf("agent event %d has wrong aggregate type", i)
		}
		switch event.Type {
		case "agent.created":
			if i != 0 {
				return AgentInspection{}, errors.New("agent created event is not first")
			}
			if err := json.Unmarshal(event.Payload, &identity); err != nil {
				return AgentInspection{}, err
			}
			generations = append(generations, identity.Generation)
		case "agent.generation_promoted":
			var next Generation
			if err := json.Unmarshal(event.Payload, &next); err != nil {
				return AgentInspection{}, err
			}
			if next.AgentID != identity.Agent.ID || next.ParentGeneration != identity.Generation.ID || next.Number != identity.Generation.Number+1 {
				return AgentInspection{}, errors.New("invalid promoted generation lineage")
			}
			if err := next.Validate(); err != nil {
				return AgentInspection{}, err
			}
			identity.Generation = next
			identity.Agent.CurrentGeneration = next.ID
			generations = append(generations, next)
		case "agent.memory_recorded":
			var memory MemoryRecord
			if err := json.Unmarshal(event.Payload, &memory); err != nil {
				return AgentInspection{}, err
			}
			if err := memory.Validate(); err != nil {
				return AgentInspection{}, err
			}
			if memory.AgentID != identity.Agent.ID {
				return AgentInspection{}, errors.New("foreign memory in agent aggregate")
			}
			memories = append(memories, memory)
		case "agent.memory_superseded":
			var change struct {
				MemoryID     string `json:"memory_id"`
				SupersededBy string `json:"superseded_by"`
			}
			if err := json.Unmarshal(event.Payload, &change); err != nil {
				return AgentInspection{}, err
			}
			found := false
			for index := range memories {
				if memories[index].ID == change.MemoryID {
					memories[index].SupersededBy = change.SupersededBy
					found = true
				}
			}
			if !found {
				return AgentInspection{}, errors.New("superseded memory does not exist")
			}
		default:
			return AgentInspection{}, fmt.Errorf("unknown agent event %s", event.Type)
		}
		version = event.AggregateVersion
	}
	if err := identity.Agent.Validate(); err != nil {
		return AgentInspection{}, err
	}
	if err := identity.Generation.Validate(); err != nil {
		return AgentInspection{}, err
	}
	if identity.Agent.ID != agentID || identity.Generation.ID != identity.Agent.CurrentGeneration {
		return AgentInspection{}, errors.New("persisted agent projection is inconsistent")
	}
	return AgentInspection{Agent: identity.Agent, CurrentGeneration: identity.Generation, GenerationHistory: generations, Memory: memories, Version: version}, nil
}

func (r Runtime) PromoteGeneration(ctx context.Context, agentID string, next Generation, actor contracts.PrincipalRef, commandID string) error {
	inspection, err := r.Inspect(ctx, agentID)
	if err != nil {
		return err
	}
	if err := actor.Validate(); err != nil {
		return err
	}
	if commandID == "" {
		return errors.New("promotion command id is required")
	}
	if next.AgentID != agentID || next.ParentGeneration != inspection.CurrentGeneration.ID || next.Number != inspection.CurrentGeneration.Number+1 {
		return errors.New("generation promotion must extend the active lineage")
	}
	if err := next.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(next)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	_, err = r.Events.Append(ctx, agentID, inspection.Version, []eventstore.Event{{ID: agentID + ":generation:" + next.ID, AggregateType: "agent", Type: "agent.generation_promoted", Version: "1", Actor: actor, CommandID: commandID, CorrelationID: commandID, Trust: contracts.TrustObserved, Payload: payload, CreatedAt: now}})
	return err
}

func (r Runtime) Remember(ctx context.Context, memory MemoryRecord, actor contracts.PrincipalRef, commandID string) error {
	inspection, err := r.Inspect(ctx, memory.AgentID)
	if err != nil {
		return err
	}
	if err := memory.Validate(); err != nil {
		return err
	}
	if err := actor.Validate(); err != nil {
		return err
	}
	if commandID == "" {
		return errors.New("memory command id is required")
	}
	for _, existing := range inspection.Memory {
		if existing.ID == memory.ID {
			return errors.New("memory id already exists")
		}
	}
	payload, err := json.Marshal(memory)
	if err != nil {
		return err
	}
	_, err = r.Events.Append(ctx, memory.AgentID, inspection.Version, []eventstore.Event{{ID: memory.AgentID + ":memory:" + memory.ID, AggregateType: "agent", Type: "agent.memory_recorded", Version: "1", Actor: actor, CommandID: commandID, CorrelationID: commandID, Trust: memory.Trust, Payload: payload, CreatedAt: memory.CreatedAt}})
	return err
}

func (r Runtime) SupersedeMemory(ctx context.Context, agentID, memoryID, replacementID string, actor contracts.PrincipalRef, commandID string) error {
	if memoryID == "" || replacementID == "" || memoryID == replacementID {
		return errors.New("distinct memory and replacement ids are required")
	}
	inspection, err := r.Inspect(ctx, agentID)
	if err != nil {
		return err
	}
	existing, replacement := false, false
	for _, memory := range inspection.Memory {
		if memory.ID == memoryID && memory.SupersededBy == "" {
			existing = true
		}
		if memory.ID == replacementID {
			replacement = true
		}
	}
	if !existing || !replacement {
		return errors.New("active memory and replacement must exist")
	}
	payload, _ := json.Marshal(struct {
		MemoryID     string `json:"memory_id"`
		SupersededBy string `json:"superseded_by"`
	}{memoryID, replacementID})
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	_, err = r.Events.Append(ctx, agentID, inspection.Version, []eventstore.Event{{ID: agentID + ":memory-superseded:" + memoryID, AggregateType: "agent", Type: "agent.memory_superseded", Version: "1", Actor: actor, CommandID: commandID, CorrelationID: commandID, Trust: contracts.TrustUserConfirmed, Payload: payload, CreatedAt: now}})
	return err
}

func (r Runtime) RetrieveMemory(ctx context.Context, agentID, scope, goalRef string, limit int) ([]MemoryRecord, error) {
	if limit <= 0 {
		return nil, errors.New("positive memory limit is required")
	}
	inspection, err := r.Inspect(ctx, agentID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	ranked := RankableMemory(inspection.Memory, now)
	out := make([]MemoryRecord, 0, limit)
	for _, memory := range ranked {
		if memory.Scope != scope && memory.Scope != "global" {
			continue
		}
		out = append(out, memory)
		if len(out) == limit {
			break
		}
	}
	return out, nil
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
	now := time.Now().UTC()
	if r.Now != nil {
		now = r.Now().UTC()
	}
	for _, memory := range memories {
		if memory.AgentID != a.ID || !memory.Active(now) {
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
