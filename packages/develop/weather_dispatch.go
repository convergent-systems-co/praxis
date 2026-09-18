package develop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/agent"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/inference"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const weatherDispatchScope = "develop:weather-dashboard"

// WeatherDispatchAuthority is deliberately package-specific. It cannot issue
// or consume authority for another Develop node or another workflow scope.
type WeatherDispatchAuthority struct{ Store *state.Store }

// WeatherDispatchIssuer is the production grant boundary. The approver is
// derived from the protected installation bootstrap, never accepted from the
// caller or model-visible workflow.
type WeatherDispatchIssuer struct {
	Store      *state.Store
	Governance *goalstore.Repository
}

func NewWeatherDispatchIssuer(store *state.Store, governance *goalstore.Repository) (WeatherDispatchIssuer, error) {
	if store == nil || governance == nil || governance.Store == nil || governance.BootstrapDigest == "" {
		return WeatherDispatchIssuer{}, errors.New("weather dispatch issuer requires protected installation governance")
	}
	if _, err := contracts.InstallationOwnerPrincipal(governance.BootstrapDigest); err != nil {
		return WeatherDispatchIssuer{}, fmt.Errorf("weather dispatch issuer bootstrap: %w", err)
	}
	return WeatherDispatchIssuer{Store: store, Governance: governance}, nil
}

func (i WeatherDispatchIssuer) Issue(ctx context.Context, routes agent.IssuedDispatchCandidateLoader, binding inference.DispatchBinding, expiresAt time.Time) (state.ExactDispatchGrant, error) {
	if i.Store == nil || i.Governance == nil || routes == nil {
		return state.ExactDispatchGrant{}, errors.New("weather dispatch issuance requires state and issued routes")
	}
	candidate, err := routes.PrepareDispatch(ctx, binding)
	if err != nil {
		return state.ExactDispatchGrant{}, err
	}
	intent, err := freezeWeatherDispatchIntent(binding, candidate)
	if err != nil {
		return state.ExactDispatchGrant{}, err
	}
	approver, err := contracts.InstallationOwnerPrincipal(i.Governance.BootstrapDigest)
	if err != nil {
		return state.ExactDispatchGrant{}, err
	}
	return i.Store.IssueExactDispatchAuthority(ctx, intent, approver, time.Now().UTC(), expiresAt)
}

func (a WeatherDispatchAuthority) AuthorizeIssuedDispatch(ctx context.Context, binding inference.DispatchBinding, candidate inference.DispatchCandidate, now time.Time) (agent.IssuedDispatchAuthorization, error) {
	if a.Store == nil {
		return agent.IssuedDispatchAuthorization{}, errors.New("weather dispatch authority requires state")
	}
	intent, err := freezeWeatherDispatchIntent(binding, candidate)
	if err != nil {
		return agent.IssuedDispatchAuthorization{}, err
	}
	invocation, err := a.Store.AuthorizeExactDispatch(ctx, intent, now)
	if err != nil {
		return agent.IssuedDispatchAuthorization{}, err
	}
	return agent.IssuedDispatchAuthorization{EffectID: invocation.EffectID, IntentDigest: invocation.IntentDigest}, nil
}

func (a WeatherDispatchAuthority) RecordDispatchAttempt(ctx context.Context, effectID string, now time.Time) error {
	return a.Store.MarkEffectDispatched(ctx, effectID, now)
}
func (a WeatherDispatchAuthority) RecordDispatchSucceeded(ctx context.Context, effectID string, evidence []byte, now time.Time) error {
	return a.Store.MarkEffectOutcome(ctx, effectID, state.EffectSucceeded, evidence, nil, now)
}
func (a WeatherDispatchAuthority) RecordDispatchUnknown(ctx context.Context, effectID string, evidence []byte, now time.Time) error {
	return a.Store.MarkEffectOutcome(ctx, effectID, state.EffectUnknown, nil, evidence, now)
}

func freezeWeatherDispatchIntent(binding inference.DispatchBinding, candidate inference.DispatchCandidate) (contracts.ActionIntent, error) {
	graph := Graph()
	if binding.RequestID == "" || binding.SubjectAgentID == "" || binding.AgentGeneration == "" || binding.RunID == "" || binding.GraphID != graph.ID || binding.GraphVersion != graph.Version || binding.NodeID != "implement" || binding.GoalRef == "" {
		return contracts.ActionIntent{}, errors.New("dispatch authority is limited to exact Develop weather implement work")
	}
	if candidate.RequestID != binding.RequestID || candidate.RouteRecordID == "" || candidate.SurfaceID == "" || candidate.ExecutorID == "" || candidate.ProviderID == "" || candidate.WorkContext != weatherDispatchScope || candidate.TargetScope != weatherDispatchScope {
		return contracts.ActionIntent{}, errors.New("dispatch authority requires the qualified weather issued route and selected surface")
	}
	parameters := map[string]string{
		"goal_ref": binding.GoalRef, "graph_id": binding.GraphID, "graph_version": binding.GraphVersion, "node_id": binding.NodeID,
		"agent_id": binding.SubjectAgentID, "agent_generation": binding.AgentGeneration, "run_id": binding.RunID, "request_id": binding.RequestID,
		"route_record_id": candidate.RouteRecordID, "surface_id": candidate.SurfaceID, "executor_id": candidate.ExecutorID, "provider_id": candidate.ProviderID,
	}
	seed, _ := json.Marshal(struct {
		Binding   inference.DispatchBinding   `json:"binding"`
		Candidate inference.DispatchCandidate `json:"candidate"`
	}{binding, candidate})
	sum := sha256.Sum256(seed)
	intent := contracts.ActionIntent{Version: "v1", ID: "intent:dispatch:" + hex.EncodeToString(sum[:]), Actor: contracts.PrincipalRef{ID: binding.SubjectAgentID, Kind: "agent"}, Operation: state.ExactDispatchCapability, Target: "executor:" + candidate.ExecutorID + "@provider:" + candidate.ProviderID, Parameters: parameters, Scope: "issued-route:" + candidate.RouteRecordID, Preconditions: map[string]string{"request_id": binding.RequestID, "route_record_id": candidate.RouteRecordID}}
	if _, err := intent.Digest(); err != nil {
		return contracts.ActionIntent{}, err
	}
	return intent, nil
}
