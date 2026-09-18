package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

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

type IssuedDispatchAuthorization struct {
	EffectID     string
	IntentDigest string
}

// IssuedDispatchAuthority is a separately governed exact-action boundary. It
// cannot select a route or executor; it can only consume authority for the
// already replay-verified binding and candidate.
type IssuedDispatchAuthority interface {
	AuthorizeIssuedDispatch(context.Context, inference.DispatchBinding, inference.DispatchCandidate, time.Time) (IssuedDispatchAuthorization, error)
	RecordDispatchAttempt(context.Context, string, time.Time) error
	RecordDispatchSucceeded(context.Context, string, []byte, time.Time) error
	RecordDispatchUnknown(context.Context, string, []byte, time.Time) error
}

// IssuedRoutedAgentExecutor invokes a selected executor only when a separately
// governed exact-action authority is composed and successfully consumed.
type IssuedRoutedAgentExecutor struct {
	Delegate  AgentNodeExecutor
	Requests  IssuedInferenceRequestResolver
	Routes    IssuedDispatchCandidateLoader
	Authority IssuedDispatchAuthority
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
	binding := inference.DispatchBinding{RequestID: requestID, SubjectAgentID: agent.AgentID, AgentGeneration: agent.GenerationID, RunID: run.RunID, GraphID: graph.ID, GraphVersion: graph.Version, NodeID: node.ID, GoalRef: agent.GoalRef}
	candidate, err := e.Routes.PrepareDispatch(ctx, binding)
	if err != nil {
		return kernel.NodeResult{}, err
	}
	if candidate.RequestID != requestID || candidate.RouteRecordID == "" || candidate.SurfaceID == "" || candidate.ExecutorID == "" || candidate.ProviderID == "" || candidate.WorkContext == "" || candidate.TargetScope == "" {
		return kernel.NodeResult{}, errors.New("issued dispatch candidate does not bind the active request and selected surface")
	}
	selected := e.Executors[candidate.ExecutorID]
	if selected == nil || selected.ID() != candidate.ExecutorID || selected.ProviderID() != candidate.ProviderID {
		return kernel.NodeResult{}, errors.New("issued selected executor/provider is unavailable at dispatch boundary")
	}
	if e.Authority == nil {
		return kernel.NodeResult{}, ErrRoutedInferenceDispatchAuthorizationRequired
	}
	// Re-read all issuance, revocation, expiry, route, and surface evidence at
	// the last boundary before exact authority consumption and invocation.
	revalidated, err := e.Routes.PrepareDispatch(ctx, binding)
	if err != nil || !reflect.DeepEqual(revalidated, candidate) {
		return kernel.NodeResult{}, errors.New("issued dispatch route changed before invocation")
	}
	now := time.Now().UTC()
	authorized, err := e.Authority.AuthorizeIssuedDispatch(ctx, binding, candidate, now)
	if err != nil {
		return kernel.NodeResult{}, err
	}
	if authorized.EffectID == "" || authorized.IntentDigest == "" {
		return kernel.NodeResult{}, errors.New("dispatch authority returned incomplete invocation evidence")
	}
	if err := e.Authority.RecordDispatchAttempt(ctx, authorized.EffectID, now); err != nil {
		return kernel.NodeResult{}, err
	}
	execution, dispatchErr := selected.ExecuteRoutedInference(ctx, graph, node, run, agent)
	observedAt := time.Now().UTC()
	if dispatchErr != nil {
		evidence, _ := json.Marshal(map[string]string{"error": dispatchErr.Error(), "route_record_id": candidate.RouteRecordID})
		if err := e.Authority.RecordDispatchUnknown(ctx, authorized.EffectID, evidence, observedAt); err != nil {
			return kernel.NodeResult{}, errors.Join(dispatchErr, err)
		}
		return kernel.NodeResult{}, dispatchErr
	}
	if execution.Result.Outcome == "" {
		err := errors.New("inference executor returned an empty outcome")
		evidence, _ := json.Marshal(map[string]string{"error": err.Error(), "route_record_id": candidate.RouteRecordID})
		if recordErr := e.Authority.RecordDispatchUnknown(ctx, authorized.EffectID, evidence, observedAt); recordErr != nil {
			return kernel.NodeResult{}, errors.Join(err, recordErr)
		}
		return kernel.NodeResult{}, err
	}
	execution.Result.Evidence = append(execution.Result.Evidence, candidate.RouteRecordID, authorized.EffectID)
	evidence, _ := json.Marshal(execution.Result)
	if err := e.Authority.RecordDispatchSucceeded(ctx, authorized.EffectID, evidence, observedAt); err != nil {
		return kernel.NodeResult{}, err
	}
	return execution.Result, nil
}
