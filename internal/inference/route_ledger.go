package inference

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/adaptation"
	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const routeDecisionEvent = "inference.route.decided"
const routeOutcomeEvent = "inference.route.outcome_observed"
const surfaceRouteDecisionEvent = "inference.surface_route.decided"
const unifiedRouteRecordEvent = "inference.route.recorded"

var routeEventVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "inference.route_decision_event", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
var routeOutcomeEventVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "inference.route_outcome_event", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)
var surfaceRouteEventVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
	Contract:       "inference.surface_route_decision_event",
	CurrentVersion: "v2",
	Versions: []contracts.ContractVersionDefinition{
		{Version: "v1", Disposition: contracts.VersionUnsupportedPreRelease, Rationale: "v1 accepted caller-asserted eligibility and untrusted event authority"},
		{Version: "v2", Disposition: contracts.VersionCurrent},
	},
}, nil)
var unifiedRouteRecordVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{
	Contract:       "inference.unified_route_record",
	CurrentVersion: "v2",
	Versions: []contracts.ContractVersionDefinition{
		{Version: "v1", Disposition: contracts.VersionUnsupportedPreRelease, Rationale: "v1 persisted route and surface authority in separate records"},
		{Version: "v2", Disposition: contracts.VersionCurrent},
	},
}, nil)

// UnifiedRouteRecord is the sole new-write routing authority. Its embedded
// decision owns the effective target, governed eligibility lineage, and either
// one selected execution surface or one stable terminal routing failure.
type UnifiedRouteRecord struct {
	ID       string                 `json:"id"`
	Version  string                 `json:"version"`
	Decision SurfaceRoutingDecision `json:"decision"`
}

func FreezeUnifiedRouteRecord(record UnifiedRouteRecord) (UnifiedRouteRecord, error) {
	record.ID, record.Version = "", unifiedRouteRecordVersions.CurrentVersion()
	if err := validateUnifiedRouteRecord(record, false); err != nil {
		return UnifiedRouteRecord{}, err
	}
	record.ID = inferenceDigest(record)
	return record, nil
}

func VerifyUnifiedRouteRecord(record UnifiedRouteRecord) error {
	if err := validateUnifiedRouteRecord(record, true); err != nil {
		return err
	}
	id := record.ID
	record.ID = ""
	if id != inferenceDigest(record) {
		return errors.New("unified route record digest mismatch")
	}
	return nil
}

func validateUnifiedRouteRecord(record UnifiedRouteRecord, requireID bool) error {
	if requireID && record.ID == "" {
		return errors.New("unified route record identity is required")
	}
	if record.Version != unifiedRouteRecordVersions.CurrentVersion() {
		return errors.New("unified route record v2 is required")
	}
	if err := VerifySurfaceRoutingDecision(record.Decision); err != nil {
		return err
	}
	_, err := selectedSurface(record.Decision)
	return err
}

func selectedSurface(decision SurfaceRoutingDecision) (*ExecutorSurface, error) {
	if decision.Outcome == SurfaceSelected {
		if decision.SelectedSurfaceID == "" || len(decision.ReasonCodes) == 0 {
			return nil, errors.New("selected route requires exactly one selected surface")
		}
		var selected *ExecutorSurface
		for index := range decision.Evaluations {
			if decision.Evaluations[index].Surface.ID == decision.SelectedSurfaceID {
				if selected != nil || !decision.Evaluations[index].Eligible {
					return nil, errors.New("selected route does not bind one eligible surface")
				}
				copy := decision.Evaluations[index].Surface
				selected = &copy
			}
		}
		if selected == nil {
			return nil, errors.New("selected route surface is absent from evaluations")
		}
		return selected, nil
	}
	if decision.SelectedSurfaceID != "" || decision.SelectedProfile != "" || decision.Fallback {
		return nil, errors.New("terminal routing failure cannot select a surface")
	}
	switch decision.Outcome {
	case SurfaceNoEligible, SurfaceRequiredUnavailable, SurfaceFallbackProhibited, SurfaceAPIUseProhibited, SurfaceTelemetryUnsatisfied:
		if len(decision.ReasonCodes) == 0 {
			return nil, errors.New("terminal routing failure requires stable reason codes")
		}
		return nil, nil
	default:
		return nil, errors.New("unrecognized terminal routing outcome")
	}
}

type RouteRecord struct {
	ID          string              `json:"id"`
	Version     string              `json:"version"`
	Request     RouteRequest        `json:"request"`
	Policy      RoutingPolicy       `json:"policy"`
	Eligibility EligibilityEvidence `json:"eligibility"`
	Decision    EvidenceDecision    `json:"decision"`
	DecidedAt   time.Time           `json:"decided_at"`
}

type RouteOutcome struct {
	ID             string                 `json:"id"`
	Version        string                 `json:"version"`
	RouteRecordID  string                 `json:"route_record_id"`
	RequestID      string                 `json:"request_id"`
	ExecutorID     string                 `json:"executor_id"`
	ProviderID     string                 `json:"provider_id"`
	ResultOutcome  string                 `json:"result_outcome"`
	ResultEvidence []string               `json:"result_evidence,omitempty"`
	Failure        string                 `json:"failure,omitempty"`
	Observation    adaptation.Observation `json:"observation"`
	ObservedAt     time.Time              `json:"observed_at"`
}

func FreezeRouteOutcome(outcome RouteOutcome) (RouteOutcome, error) {
	outcome.ID, outcome.Version = "", routeOutcomeEventVersions.CurrentVersion()
	outcome.ResultEvidence = append([]string(nil), outcome.ResultEvidence...)
	sort.Strings(outcome.ResultEvidence)
	if err := validateRouteOutcome(outcome, false); err != nil {
		return RouteOutcome{}, err
	}
	outcome.ID = inferenceDigest(outcome)
	return outcome, nil
}

func VerifyRouteOutcome(outcome RouteOutcome) error {
	if err := validateRouteOutcome(outcome, true); err != nil {
		return err
	}
	id := outcome.ID
	outcome.ID = ""
	if id != inferenceDigest(outcome) {
		return errors.New("route outcome digest mismatch")
	}
	return nil
}

func validateRouteOutcome(outcome RouteOutcome, requireID bool) error {
	if requireID && outcome.ID == "" {
		return errors.New("route outcome identity is required")
	}
	if outcome.Version != routeOutcomeEventVersions.CurrentVersion() || outcome.RouteRecordID == "" || outcome.RequestID == "" || outcome.ExecutorID == "" || outcome.ProviderID == "" || outcome.ObservedAt.IsZero() {
		return errors.New("route outcome requires route, request, executor, provider, version, and time")
	}
	if (outcome.ResultOutcome == "") == (outcome.Failure == "") {
		return errors.New("route outcome requires exactly one result or failure")
	}
	if err := adaptation.VerifyObservation(outcome.Observation); err != nil {
		return err
	}
	if outcome.Observation.ID == "" || outcome.Observation.ObservedAt != outcome.ObservedAt || outcome.Observation.PathID != outcome.RouteRecordID || outcome.Observation.ProviderID != outcome.ProviderID {
		return errors.New("route outcome does not bind its adaptive observation")
	}
	if outcome.Observation.CausationRoot != outcome.RouteRecordID {
		return errors.New("route outcome observation causation root does not bind the route record")
	}
	wantObservedOutcome := outcome.ResultOutcome
	if outcome.Failure != "" {
		wantObservedOutcome = "executor_error"
	}
	if outcome.Observation.Outcome != wantObservedOutcome {
		return errors.New("route outcome observation result does not match the persisted result")
	}
	return nil
}

func FreezeRouteRecord(record RouteRecord) (RouteRecord, error) {
	record.ID, record.Version = "", routeEventVersions.CurrentVersion()
	if err := validateRouteRecord(record, false); err != nil {
		return RouteRecord{}, err
	}
	record.ID = inferenceDigest(record)
	return record, nil
}

func VerifyRouteRecord(record RouteRecord) error {
	if err := validateRouteRecord(record, true); err != nil {
		return err
	}
	id := record.ID
	record.ID = ""
	if id != inferenceDigest(record) {
		return errors.New("route record digest mismatch")
	}
	return nil
}

func validateRouteRecord(record RouteRecord, requireID bool) error {
	if requireID && record.ID == "" {
		return errors.New("route record identity is required")
	}
	policy, err := FreezeRoutingPolicy(record.Policy)
	if err != nil || policy.ID != record.Policy.ID {
		return errors.New("route record policy is not frozen")
	}
	request, err := FreezeRouteRequest(record.Request)
	if err != nil || request.ID != record.Request.ID {
		return errors.New("route record request is not frozen")
	}
	candidate := RouteCandidate{ExecutorID: record.Decision.ExecutorID, ProviderID: record.Decision.ProviderID}
	if err := verifyEligibility(record.Eligibility, record.Request, candidate); err != nil {
		return err
	}
	decision := record.Decision
	id := decision.ID
	decision.ID = ""
	if id == "" || id != inferenceDigest(decision) || record.Decision.RequestID != record.Request.ID || record.Decision.PolicyID != record.Policy.ID || record.Decision.EligibilityID != record.Eligibility.ID {
		return errors.New("route decision does not bind request, policy, and eligibility evidence")
	}
	if record.Version != routeEventVersions.CurrentVersion() || record.DecidedAt.IsZero() {
		return errors.New("route record requires versioned execution scope and time")
	}
	return nil
}

type RouteLedger struct{ store eventstore.Store }

func NewRouteLedger(store eventstore.Store) (*RouteLedger, error) {
	if store == nil {
		return nil, errors.New("route event store is required")
	}
	return &RouteLedger{store: store}, nil
}

func routeAggregate(subject string) string { return "inference-routes:" + subject }

func (l *RouteLedger) RecordUnifiedRoute(ctx context.Context, record UnifiedRouteRecord) error {
	if err := VerifyUnifiedRouteRecord(record); err != nil {
		return err
	}
	decision := record.Decision
	subject := decision.Request.RouteRequest.SubjectAgentID
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ID == "event:"+record.ID {
			existing, decodeErr := decodeUnifiedRouteRecordEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if inferenceDigest(existing) != inferenceDigest(record) {
				return errors.New("unified route event identity is already bound to different payload")
			}
			return verifyUnifiedRouteEventMetadata(event, existing, subject)
		}
		switch event.Type {
		case unifiedRouteRecordEvent:
			existing, decodeErr := decodeUnifiedRouteRecordEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if existing.Decision.Request.ID == decision.Request.ID || existing.Decision.Request.RouteRequest.ID == decision.Request.RouteRequest.ID {
				return errors.New("route request already has a different unified routing record")
			}
		case routeDecisionEvent:
			legacy, decodeErr := decodeRouteRecordEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if legacy.Request.ID == decision.Request.RouteRequest.ID {
				return errors.New("route request already has an authoritative legacy route record")
			}
		case surfaceRouteDecisionEvent:
			legacy, decodeErr := decodeSurfaceDecisionEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if legacy.Request.ID == decision.Request.ID || legacy.Request.RouteRequest.ID == decision.Request.RouteRequest.ID {
				return errors.New("route request already has an authoritative legacy surface decision")
			}
		}
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, routeAggregate(subject), int64(len(events)), []eventstore.Event{{
		ID: "event:" + record.ID, AggregateType: "inference_routes", Type: unifiedRouteRecordEvent,
		Version: unifiedRouteRecordVersions.CurrentVersion(), Actor: decision.Evaluator,
		CommandID: "unified-route:" + record.ID, CorrelationID: decision.Request.RouteRequest.RunID,
		CausationID: decision.Request.ID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: decision.DecidedAt,
	}})
	return err
}

func (l *RouteLedger) UnifiedRoutes(ctx context.Context, subject string) ([]UnifiedRouteRecord, error) {
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return nil, err
	}
	out := []UnifiedRouteRecord{}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != unifiedRouteRecordEvent {
			continue
		}
		record, err := decodeUnifiedRouteRecordEvent(event)
		if err != nil {
			return nil, err
		}
		request := record.Decision.Request
		if seen[request.ID] || seen[request.RouteRequest.ID] {
			return nil, errors.New("duplicate unified routing lineage")
		}
		if err := verifyUnifiedRouteEventMetadata(event, record, subject); err != nil {
			return nil, err
		}
		for _, other := range events {
			switch other.Type {
			case routeDecisionEvent:
				legacy, err := decodeRouteRecordEvent(other)
				if err != nil {
					return nil, err
				}
				if legacy.Request.ID == request.RouteRequest.ID {
					return nil, errors.New("unified route record is paired with a legacy route record")
				}
			case surfaceRouteDecisionEvent:
				legacy, err := decodeSurfaceDecisionEvent(other)
				if err != nil {
					return nil, err
				}
				if legacy.Request.ID == request.ID || legacy.Request.RouteRequest.ID == request.RouteRequest.ID {
					return nil, errors.New("unified route record is paired with a legacy surface decision")
				}
			}
		}
		seen[request.ID], seen[request.RouteRequest.ID] = true, true
		out = append(out, record)
	}
	return out, nil
}

func decodeUnifiedRouteRecordEvent(event eventstore.Event) (UnifiedRouteRecord, error) {
	payload, _, err := unifiedRouteRecordVersions.Canonicalize(event.Version, event.Payload)
	if err != nil {
		return UnifiedRouteRecord{}, err
	}
	var record UnifiedRouteRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return UnifiedRouteRecord{}, err
	}
	if err := VerifyUnifiedRouteRecord(record); err != nil {
		return UnifiedRouteRecord{}, err
	}
	return record, nil
}

func verifyUnifiedRouteEventMetadata(event eventstore.Event, record UnifiedRouteRecord, subject string) error {
	decision := record.Decision
	if decision.Request.RouteRequest.SubjectAgentID != subject || event.ID != "event:"+record.ID || event.AggregateID != routeAggregate(subject) || event.AggregateType != "inference_routes" || event.Type != unifiedRouteRecordEvent || event.Version != unifiedRouteRecordVersions.CurrentVersion() || event.Actor != decision.Evaluator || event.CommandID != "unified-route:"+record.ID || event.CorrelationID != decision.Request.RouteRequest.RunID || event.CausationID != decision.Request.ID || event.Trust != contracts.TrustPolicy || !event.CreatedAt.Equal(decision.DecidedAt) {
		return errors.New("unified route event metadata is invalid")
	}
	return nil
}

func decodeRouteRecordEvent(event eventstore.Event) (RouteRecord, error) {
	payload, _, err := routeEventVersions.Canonicalize(event.Version, event.Payload)
	if err != nil {
		return RouteRecord{}, err
	}
	var record RouteRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return RouteRecord{}, err
	}
	if err := VerifyRouteRecord(record); err != nil {
		return RouteRecord{}, err
	}
	return record, nil
}

func (l *RouteLedger) Record(ctx context.Context, record RouteRecord) error {
	if err := VerifyRouteRecord(record); err != nil {
		return err
	}
	events, err := l.store.LoadAggregate(ctx, routeAggregate(record.Request.SubjectAgentID), 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ID == "event:"+record.ID {
			return nil
		}
		if event.Type == routeDecisionEvent {
			payload, _, canonicalErr := routeEventVersions.Canonicalize(event.Version, event.Payload)
			if canonicalErr != nil {
				return canonicalErr
			}
			var existing RouteRecord
			if err := json.Unmarshal(payload, &existing); err != nil {
				return err
			}
			if existing.Request.ID == record.Request.ID {
				return errors.New("route request already has a different decision")
			}
		}
		if event.Type == unifiedRouteRecordEvent {
			unified, decodeErr := decodeUnifiedRouteRecordEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if unified.Decision.Request.RouteRequest.ID == record.Request.ID {
				return errors.New("route request already has a unified routing record")
			}
		}
		if event.Type == surfaceRouteDecisionEvent {
			surface, decodeErr := decodeSurfaceDecisionEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if surface.Request.RouteRequest.ID == record.Request.ID {
				return errors.New("route request already has a legacy surface decision")
			}
		}
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, routeAggregate(record.Request.SubjectAgentID), int64(len(events)), []eventstore.Event{{ID: "event:" + record.ID, AggregateType: "inference_routes", Type: routeDecisionEvent, Version: routeEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: record.Eligibility.AuthorityID, Kind: "authority"}, CommandID: "route:" + record.ID, CorrelationID: record.Request.RunID, Trust: contracts.TrustPolicy, Payload: payload, CreatedAt: record.DecidedAt}})
	return err
}

func (l *RouteLedger) Records(ctx context.Context, subject string) ([]RouteRecord, error) {
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return nil, err
	}
	out := []RouteRecord{}
	for _, event := range events {
		if event.Type != routeDecisionEvent {
			continue
		}
		payload, _, err := routeEventVersions.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var record RouteRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, err
		}
		if err := VerifyRouteRecord(record); err != nil {
			return nil, err
		}
		if record.Request.SubjectAgentID != subject || event.ID != "event:"+record.ID || event.Actor != (contracts.PrincipalRef{ID: record.Eligibility.AuthorityID, Kind: "authority"}) || event.CorrelationID != record.Request.RunID || event.Trust != contracts.TrustPolicy {
			return nil, errors.New("route event metadata does not bind authority and run")
		}
		for _, other := range events {
			if other.Type != unifiedRouteRecordEvent && other.Type != surfaceRouteDecisionEvent {
				continue
			}
			if other.Type == unifiedRouteRecordEvent {
				unified, err := decodeUnifiedRouteRecordEvent(other)
				if err != nil {
					return nil, err
				}
				if unified.Decision.Request.RouteRequest.ID == record.Request.ID {
					return nil, errors.New("legacy route record is paired with a unified route record")
				}
			} else {
				surface, err := decodeSurfaceDecisionEvent(other)
				if err != nil {
					return nil, err
				}
				if surface.Request.RouteRequest.ID == record.Request.ID {
					return nil, errors.New("legacy route record is paired with a legacy surface decision")
				}
			}
		}
		out = append(out, record)
	}
	return out, nil
}

func (l *RouteLedger) RecordOutcome(ctx context.Context, subject string, outcome RouteOutcome) error {
	if err := VerifyRouteOutcome(outcome); err != nil {
		return err
	}
	bindings, err := l.executionBindings(ctx, subject)
	if err != nil {
		return err
	}
	binding, ok := bindings[outcome.RouteRecordID]
	if !ok || binding.RequestID != outcome.RequestID || binding.ExecutorID != outcome.ExecutorID || binding.ProviderID != outcome.ProviderID || !observationMatchesRequest(outcome.Observation, binding.Request) {
		return errors.New("route outcome does not match its selected execution")
	}
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ID == "event:"+outcome.ID {
			existing, decodeErr := decodeRouteOutcomeEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if inferenceDigest(existing) != inferenceDigest(outcome) {
				return errors.New("route outcome event identity is already bound to different payload")
			}
			return verifyRouteOutcomeEventMetadata(event, existing, subject, binding)
		}
		if event.Type == routeOutcomeEvent {
			var existing RouteOutcome
			canonical, _, canonicalErr := routeOutcomeEventVersions.Canonicalize(event.Version, event.Payload)
			if canonicalErr != nil {
				return canonicalErr
			}
			if err := json.Unmarshal(canonical, &existing); err != nil {
				return err
			}
			if existing.RequestID == outcome.RequestID {
				return errors.New("route request already has a different outcome")
			}
		}
	}
	payload, err := json.Marshal(outcome)
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, routeAggregate(subject), int64(len(events)), []eventstore.Event{{ID: "event:" + outcome.ID, AggregateType: "inference_routes", Type: routeOutcomeEvent, Version: routeOutcomeEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: subject, Kind: "agent"}, CommandID: "route-outcome:" + outcome.ID, CorrelationID: binding.Request.RunID, CausationID: outcome.RouteRecordID, Trust: contracts.TrustObserved, Payload: payload, CreatedAt: outcome.ObservedAt}})
	return err
}

func decodeRouteOutcomeEvent(event eventstore.Event) (RouteOutcome, error) {
	payload, _, err := routeOutcomeEventVersions.Canonicalize(event.Version, event.Payload)
	if err != nil {
		return RouteOutcome{}, err
	}
	var outcome RouteOutcome
	if err := json.Unmarshal(payload, &outcome); err != nil {
		return RouteOutcome{}, err
	}
	if err := VerifyRouteOutcome(outcome); err != nil {
		return RouteOutcome{}, err
	}
	return outcome, nil
}

func verifyRouteOutcomeEventMetadata(event eventstore.Event, outcome RouteOutcome, subject string, binding executionBinding) error {
	if binding.RequestID != outcome.RequestID || binding.ExecutorID != outcome.ExecutorID || binding.ProviderID != outcome.ProviderID || !observationMatchesRequest(outcome.Observation, binding.Request) || event.ID != "event:"+outcome.ID || event.AggregateID != routeAggregate(subject) || event.AggregateType != "inference_routes" || event.Type != routeOutcomeEvent || event.Version != routeOutcomeEventVersions.CurrentVersion() || event.Actor != (contracts.PrincipalRef{ID: subject, Kind: "agent"}) || event.CommandID != "route-outcome:"+outcome.ID || event.CorrelationID != binding.Request.RunID || event.CausationID != outcome.RouteRecordID || event.Trust != contracts.TrustObserved || !event.CreatedAt.Equal(outcome.ObservedAt) {
		return errors.New("route outcome event metadata or lineage is invalid")
	}
	return nil
}

func (l *RouteLedger) Outcomes(ctx context.Context, subject string) ([]RouteOutcome, error) {
	bindings, err := l.executionBindings(ctx, subject)
	if err != nil {
		return nil, err
	}
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return nil, err
	}
	out := []RouteOutcome{}
	seenRequests := map[string]bool{}
	for _, event := range events {
		if event.Type != routeOutcomeEvent {
			continue
		}
		payload, _, err := routeOutcomeEventVersions.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var outcome RouteOutcome
		if err := json.Unmarshal(payload, &outcome); err != nil {
			return nil, err
		}
		if err := VerifyRouteOutcome(outcome); err != nil {
			return nil, err
		}
		binding, ok := bindings[outcome.RouteRecordID]
		if !ok || binding.RequestID != outcome.RequestID || binding.ExecutorID != outcome.ExecutorID || binding.ProviderID != outcome.ProviderID || !observationMatchesRequest(outcome.Observation, binding.Request) || event.ID != "event:"+outcome.ID || event.AggregateID != routeAggregate(subject) || event.AggregateType != "inference_routes" || event.Type != routeOutcomeEvent || event.Version != routeOutcomeEventVersions.CurrentVersion() || event.Actor != (contracts.PrincipalRef{ID: subject, Kind: "agent"}) || event.CommandID != "route-outcome:"+outcome.ID || event.CorrelationID != binding.Request.RunID || event.CausationID != outcome.RouteRecordID || event.Trust != contracts.TrustObserved || !event.CreatedAt.Equal(outcome.ObservedAt) || seenRequests[outcome.RequestID] {
			return nil, errors.New("route outcome event metadata or lineage is invalid")
		}
		seenRequests[outcome.RequestID] = true
		out = append(out, outcome)
	}
	return out, nil
}

type executionBinding struct {
	RequestID  string
	Request    RouteRequest
	ExecutorID string
	ProviderID string
}

func (l *RouteLedger) executionBindings(ctx context.Context, subject string) (map[string]executionBinding, error) {
	legacy, err := l.Records(ctx, subject)
	if err != nil {
		return nil, err
	}
	unified, err := l.UnifiedRoutes(ctx, subject)
	if err != nil {
		return nil, err
	}
	out := make(map[string]executionBinding, len(legacy)+len(unified))
	for _, record := range legacy {
		out[record.ID] = executionBinding{RequestID: record.Request.ID, Request: record.Request, ExecutorID: record.Decision.ExecutorID, ProviderID: record.Decision.ProviderID}
	}
	for _, record := range unified {
		surface, err := selectedSurface(record.Decision)
		if err != nil {
			return nil, err
		}
		if surface == nil {
			continue
		}
		out[record.ID] = executionBinding{RequestID: record.Decision.Request.ID, Request: record.Decision.Request.RouteRequest, ExecutorID: surface.ExecutorID, ProviderID: surface.ProviderID}
	}
	return out, nil
}

func observationMatchesRequest(observation adaptation.Observation, request RouteRequest) bool {
	return request.SubjectAgentID == observation.SubjectAgentID && request.RunID == observation.RunID && request.GoalClass == observation.GoalClass && request.Domain == observation.Domain && request.BehaviorKey == observation.BehaviorKey && request.Context == observation.Context && string(request.Tier) == observation.ReasoningTier
}

func (l *RouteLedger) Execution(ctx context.Context, subject, requestID string) (*RouteRecord, *RouteOutcome, error) {
	records, err := l.Records(ctx, subject)
	if err != nil {
		return nil, nil, err
	}
	outcomes, err := l.Outcomes(ctx, subject)
	if err != nil {
		return nil, nil, err
	}
	var record *RouteRecord
	var outcome *RouteOutcome
	for index := range records {
		if records[index].Request.ID == requestID {
			copy := records[index]
			record = &copy
		}
	}
	for index := range outcomes {
		if outcomes[index].RequestID == requestID {
			copy := outcomes[index]
			outcome = &copy
		}
	}
	return record, outcome, nil
}

// RecordSurfaceDecision is retained only to fail closed for callers of the
// pre-release split persistence API. New writes must use RecordUnifiedRoute.
func (l *RouteLedger) RecordSurfaceDecision(_ context.Context, _ SurfaceRoutingDecision) error {
	return errors.Join(contracts.ErrUnsupportedPreReleaseContractVersion, errors.New("surface decision persistence was replaced by unified route record v2"))
}

func (l *RouteLedger) SurfaceDecisions(ctx context.Context, subject string) ([]SurfaceRoutingDecision, error) {
	unified, err := l.UnifiedRoutes(ctx, subject)
	if err != nil {
		return nil, err
	}
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return nil, err
	}
	out := make([]SurfaceRoutingDecision, 0, len(unified))
	seenRequests := map[string]bool{}
	for _, record := range unified {
		out = append(out, record.Decision)
		seenRequests[record.Decision.Request.ID] = true
		seenRequests[record.Decision.Request.RouteRequest.ID] = true
	}
	routes := map[string]RouteRecord{}
	for _, event := range events {
		if event.Type != routeDecisionEvent {
			continue
		}
		payload, _, err := routeEventVersions.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var route RouteRecord
		if err := json.Unmarshal(payload, &route); err != nil {
			return nil, err
		}
		if err := VerifyRouteRecord(route); err != nil {
			return nil, err
		}
		if err := verifyRouteEventForSurfaceBridge(event, route); err != nil {
			return nil, err
		}
		routes[route.Request.ID] = route
	}
	for _, event := range events {
		if event.Type != surfaceRouteDecisionEvent {
			continue
		}
		decision, err := decodeSurfaceDecisionEvent(event)
		if err != nil {
			return nil, err
		}
		if err := verifySurfaceEventMetadata(event, decision, subject); err != nil || seenRequests[decision.Request.ID] || seenRequests[decision.Request.RouteRequest.ID] {
			return nil, errors.New("surface route event metadata or request lineage is invalid")
		}
		if _, ok := routes[decision.Request.RouteRequest.ID]; ok {
			return nil, errors.New("legacy surface decision is paired with a legacy route record")
		}
		seenRequests[decision.Request.ID] = true
		seenRequests[decision.Request.RouteRequest.ID] = true
		out = append(out, decision)
	}
	return out, nil
}

func decodeSurfaceDecisionEvent(event eventstore.Event) (SurfaceRoutingDecision, error) {
	payload, _, err := surfaceRouteEventVersions.Canonicalize(event.Version, event.Payload)
	if err != nil {
		return SurfaceRoutingDecision{}, err
	}
	var decision SurfaceRoutingDecision
	if err := json.Unmarshal(payload, &decision); err != nil {
		return SurfaceRoutingDecision{}, err
	}
	if err := VerifySurfaceRoutingDecision(decision); err != nil {
		return SurfaceRoutingDecision{}, err
	}
	return decision, nil
}

func verifySurfaceEventMetadata(event eventstore.Event, decision SurfaceRoutingDecision, subject string) error {
	if decision.Request.RouteRequest.SubjectAgentID != subject || event.ID != "event:"+decision.ID || event.AggregateID != routeAggregate(subject) || event.AggregateType != "inference_routes" || event.Type != surfaceRouteDecisionEvent || event.Version != surfaceRouteEventVersions.CurrentVersion() || event.Actor != decision.Evaluator || event.CommandID != "surface-route:"+decision.ID || event.CorrelationID != decision.Request.RouteRequest.RunID || event.CausationID != decision.Request.RouteRequest.ID || event.Trust != contracts.TrustPolicy || !event.CreatedAt.Equal(decision.DecidedAt) {
		return errors.New("surface route event metadata is invalid")
	}
	return nil
}

func verifySurfaceRouteBinding(surface SurfaceRoutingDecision, route RouteRecord) error {
	if surface.Request.RouteRequest.ID != route.Request.ID || inferenceDigest(surface.Request.RouteRequest) != inferenceDigest(route.Request) || surface.Outcome != SurfaceSelected {
		return errors.New("surface and evidence route lineage disagree")
	}
	for _, evaluation := range surface.Evaluations {
		if evaluation.Surface.ID == surface.SelectedSurfaceID {
			if evaluation.Surface.ExecutorID == route.Decision.ExecutorID && evaluation.Surface.ProviderID == route.Decision.ProviderID {
				return nil
			}
			break
		}
	}
	return errors.New("surface and evidence route selected different executors")
}

func verifyRouteEventForSurfaceBridge(event eventstore.Event, route RouteRecord) error {
	if err := VerifyRouteRecord(route); err != nil {
		return err
	}
	if event.ID != "event:"+route.ID || event.AggregateID != routeAggregate(route.Request.SubjectAgentID) || event.AggregateType != "inference_routes" || event.Type != routeDecisionEvent || event.Version != routeEventVersions.CurrentVersion() || event.Actor != (contracts.PrincipalRef{ID: route.Eligibility.AuthorityID, Kind: "authority"}) || event.CommandID != "route:"+route.ID || event.CorrelationID != route.Request.RunID || event.CausationID != "" || event.Trust != contracts.TrustPolicy || !event.CreatedAt.Equal(route.DecidedAt) {
		return errors.New("evidence route event metadata is invalid for surface binding")
	}
	return nil
}

func (l *RouteLedger) SurfaceDecision(ctx context.Context, subject, requestID string) (*SurfaceRoutingDecision, error) {
	decisions, err := l.SurfaceDecisions(ctx, subject)
	if err != nil {
		return nil, err
	}
	for index := range decisions {
		if decisions[index].Request.ID == requestID {
			decision := decisions[index]
			return &decision, nil
		}
	}
	return nil, nil
}

func (l *RouteLedger) UnifiedRoute(ctx context.Context, subject, requestID string) (*UnifiedRouteRecord, error) {
	records, err := l.UnifiedRoutes(ctx, subject)
	if err != nil {
		return nil, err
	}
	for index := range records {
		if records[index].Decision.Request.ID == requestID || records[index].Decision.Request.RouteRequest.ID == requestID {
			copy := records[index]
			return &copy, nil
		}
	}
	return nil, nil
}

func (l *RouteLedger) UnifiedExecution(ctx context.Context, subject, requestID string) (*UnifiedRouteRecord, *RouteOutcome, error) {
	record, err := l.UnifiedRoute(ctx, subject, requestID)
	if err != nil || record == nil {
		return record, nil, err
	}
	outcomes, err := l.Outcomes(ctx, subject)
	if err != nil {
		return nil, nil, err
	}
	for index := range outcomes {
		if outcomes[index].RouteRecordID == record.ID {
			copy := outcomes[index]
			return record, &copy, nil
		}
	}
	return record, nil, nil
}
