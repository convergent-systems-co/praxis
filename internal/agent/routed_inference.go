package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

var ErrRoutedInferenceOutcomeUnknown = errors.New("routed inference outcome is unknown and requires reconciliation")

type InferenceRoutingPlan struct {
	Request    inference.RouteRequest
	Policy     inference.RoutingPolicy
	Candidates []inference.RouteCandidate
}

type InferencePlanResolver interface {
	ResolveInferencePlan(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution, agent ExecutionContext) (InferenceRoutingPlan, error)
}

type InferenceExecution struct {
	Result      kernel.NodeResult
	RawMeasures []adaptation.Measure
	Invariants  []adaptation.InvariantResult
}

type RoutedInferenceExecutor interface {
	ID() string
	ProviderID() string
	ExecuteRoutedInference(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution, agent ExecutionContext) (InferenceExecution, error)
}

// RoutedAgentExecutor binds package-selected evidence routing to the actual
// operational-graph inference dispatch. Non-inference nodes remain delegated
// to the graph's ordinary executor.
type RoutedAgentExecutor struct {
	Delegate  AgentNodeExecutor
	Plans     InferencePlanResolver
	Authority inference.EligibilityAuthority
	Routes    *inference.RouteLedger
	Evidence  *adaptation.Ledger
	Executors map[string]RoutedInferenceExecutor
	Now       func() time.Time
}

func (e RoutedAgentExecutor) ID() string { return "governed-inference-router" }

func (e RoutedAgentExecutor) ExecuteAgentNode(ctx context.Context, graph kernel.GraphDef, node kernel.NodeDef, run *kernel.RunExecution, agent ExecutionContext) (kernel.NodeResult, error) {
	if node.Class != kernel.NodeInference {
		if e.Delegate == nil {
			return kernel.NodeResult{}, errors.New("ordinary graph node requires a delegate executor")
		}
		return e.Delegate.ExecuteAgentNode(ctx, graph, node, run, agent)
	}
	if e.Plans == nil || e.Authority == nil || e.Routes == nil || e.Evidence == nil || len(e.Executors) == 0 {
		return kernel.NodeResult{}, errors.New("routed inference requires plan, authority, route ledger, adaptive ledger, and executors")
	}
	plan, err := e.Plans.ResolveInferencePlan(ctx, graph, node, run, agent)
	if err != nil {
		return kernel.NodeResult{}, err
	}
	request, err := inference.FreezeRouteRequest(plan.Request)
	if err != nil || request.ID != plan.Request.ID || request.SubjectAgentID != agent.AgentID || request.RunID != run.RunID {
		return kernel.NodeResult{}, errors.New("inference plan request does not bind the active agent run")
	}

	existingRoute, existingOutcome, err := e.Routes.Execution(ctx, agent.AgentID, request.ID)
	if err != nil {
		return kernel.NodeResult{}, err
	}
	if existingOutcome != nil {
		if err := e.Evidence.Record(ctx, existingOutcome.Observation); err != nil {
			return kernel.NodeResult{}, fmt.Errorf("reconcile routed inference observation: %w", err)
		}
		if existingOutcome.Failure != "" {
			return kernel.NodeResult{}, fmt.Errorf("persisted routed inference failure: %s", existingOutcome.Failure)
		}
		return kernel.NodeResult{Outcome: existingOutcome.ResultOutcome, Evidence: append([]string(nil), existingOutcome.ResultEvidence...)}, nil
	}
	if existingRoute != nil {
		return kernel.NodeResult{}, ErrRoutedInferenceOutcomeUnknown
	}

	observations, err := e.Evidence.Observations(ctx, agent.AgentID)
	if err != nil {
		return kernel.NodeResult{}, fmt.Errorf("load durable routing observations: %w", err)
	}
	measurements, err := e.Evidence.Measurements(ctx, agent.AgentID)
	if err != nil {
		return kernel.NodeResult{}, fmt.Errorf("load durable routing measurements: %w", err)
	}
	decision, err := inference.RouteFromEvidence(ctx, request, plan.Policy, plan.Candidates, observations, measurements, e.Authority)
	if err != nil {
		return kernel.NodeResult{}, err
	}
	selected := e.Executors[decision.ExecutorID]
	if selected == nil || selected.ID() != decision.ExecutorID || selected.ProviderID() != decision.ProviderID {
		return kernel.NodeResult{}, errors.New("selected executor/provider is unavailable at dispatch")
	}
	eligibility, err := e.Authority.EvaluateRouteEligibility(ctx, request, inference.RouteCandidate{ExecutorID: decision.ExecutorID, ProviderID: decision.ProviderID})
	if err != nil || eligibility.ID != decision.EligibilityID {
		return kernel.NodeResult{}, errors.New("dispatch eligibility changed after selection")
	}
	now := time.Now().UTC()
	if e.Now != nil {
		now = e.Now().UTC()
	}
	record, err := inference.FreezeRouteRecord(inference.RouteRecord{Request: request, Policy: plan.Policy, Eligibility: eligibility, Decision: decision, DecidedAt: now})
	if err != nil {
		return kernel.NodeResult{}, err
	}
	if err := e.Routes.Record(ctx, record); err != nil {
		return kernel.NodeResult{}, fmt.Errorf("persist route before dispatch: %w", err)
	}

	execution, dispatchErr := selected.ExecuteRoutedInference(ctx, graph, node, run, agent)
	observedAt := time.Now().UTC()
	if e.Now != nil {
		observedAt = e.Now().UTC()
	}
	observedOutcome := execution.Result.Outcome
	resultOutcome := execution.Result.Outcome
	failure := ""
	if dispatchErr != nil {
		observedOutcome = "executor_error"
		resultOutcome = ""
		failure = dispatchErr.Error()
	} else if observedOutcome == "" {
		return kernel.NodeResult{}, errors.New("inference executor returned an empty outcome")
	}
	observation, err := adaptation.FreezeObservation(adaptation.Observation{SubjectAgentID: request.SubjectAgentID, RunID: request.RunID, GoalClass: request.GoalClass, Domain: request.Domain, BehaviorKey: request.BehaviorKey, Context: request.Context, CausationRoot: record.ID, Trust: contracts.TrustObserved, ReasoningTier: string(request.Tier), ProviderID: decision.ProviderID, Outcome: observedOutcome, PathID: record.ID, RawMeasures: execution.RawMeasures, Invariants: execution.Invariants, ObservedAt: observedAt})
	if err != nil {
		return kernel.NodeResult{}, err
	}
	execution.Result.Evidence = append(execution.Result.Evidence, record.ID, observation.ID)
	outcome, err := inference.FreezeRouteOutcome(inference.RouteOutcome{RouteRecordID: record.ID, RequestID: request.ID, ExecutorID: decision.ExecutorID, ProviderID: decision.ProviderID, ResultOutcome: resultOutcome, ResultEvidence: execution.Result.Evidence, Failure: failure, Observation: observation, ObservedAt: observedAt})
	if err != nil {
		return kernel.NodeResult{}, err
	}
	if err := e.Routes.RecordOutcome(ctx, agent.AgentID, outcome); err != nil {
		return kernel.NodeResult{}, fmt.Errorf("persist routed inference outcome: %w", err)
	}
	if err := e.Evidence.Record(ctx, observation); err != nil {
		return kernel.NodeResult{}, fmt.Errorf("project routed inference observation: %w", err)
	}
	if dispatchErr != nil {
		return kernel.NodeResult{}, dispatchErr
	}
	return execution.Result, nil
}
