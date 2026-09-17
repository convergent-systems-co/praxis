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
		if event.Type == surfaceRouteDecisionEvent {
			surface, decodeErr := decodeSurfaceDecisionEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if surface.Request.RouteRequest.ID == record.Request.ID {
				if err := verifySurfaceRouteBinding(surface, record); err != nil {
					return err
				}
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
		out = append(out, record)
	}
	return out, nil
}

func (l *RouteLedger) RecordOutcome(ctx context.Context, subject string, outcome RouteOutcome) error {
	if err := VerifyRouteOutcome(outcome); err != nil {
		return err
	}
	records, err := l.Records(ctx, subject)
	if err != nil {
		return err
	}
	var route *RouteRecord
	for index := range records {
		if records[index].ID == outcome.RouteRecordID {
			route = &records[index]
			break
		}
	}
	if route == nil || route.Request.ID != outcome.RequestID || route.Decision.ExecutorID != outcome.ExecutorID || route.Decision.ProviderID != outcome.ProviderID || route.Request.SubjectAgentID != outcome.Observation.SubjectAgentID || route.Request.RunID != outcome.Observation.RunID || route.Request.GoalClass != outcome.Observation.GoalClass || route.Request.Domain != outcome.Observation.Domain || route.Request.BehaviorKey != outcome.Observation.BehaviorKey || route.Request.Context != outcome.Observation.Context || string(route.Request.Tier) != outcome.Observation.ReasoningTier {
		return errors.New("route outcome does not match its selected execution")
	}
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ID == "event:"+outcome.ID {
			return nil
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
	_, err = l.store.Append(ctx, routeAggregate(subject), int64(len(events)), []eventstore.Event{{ID: "event:" + outcome.ID, AggregateType: "inference_routes", Type: routeOutcomeEvent, Version: routeOutcomeEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: subject, Kind: "agent"}, CommandID: "route-outcome:" + outcome.ID, CorrelationID: route.Request.RunID, CausationID: route.ID, Trust: contracts.TrustObserved, Payload: payload, CreatedAt: outcome.ObservedAt}})
	return err
}

func (l *RouteLedger) Outcomes(ctx context.Context, subject string) ([]RouteOutcome, error) {
	records, err := l.Records(ctx, subject)
	if err != nil {
		return nil, err
	}
	byID := map[string]RouteRecord{}
	for _, record := range records {
		byID[record.ID] = record
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
		route, ok := byID[outcome.RouteRecordID]
		if !ok || route.Request.ID != outcome.RequestID || route.Decision.ExecutorID != outcome.ExecutorID || route.Decision.ProviderID != outcome.ProviderID || event.ID != "event:"+outcome.ID || event.Actor != (contracts.PrincipalRef{ID: subject, Kind: "agent"}) || event.CorrelationID != route.Request.RunID || event.CausationID != route.ID || event.Trust != contracts.TrustObserved || seenRequests[outcome.RequestID] {
			return nil, errors.New("route outcome event metadata or lineage is invalid")
		}
		seenRequests[outcome.RequestID] = true
		out = append(out, outcome)
	}
	return out, nil
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

// RecordSurfaceDecision appends provider-neutral surface routing evidence to
// the existing per-agent inference-route aggregate.
func (l *RouteLedger) RecordSurfaceDecision(ctx context.Context, decision SurfaceRoutingDecision) error {
	if err := VerifySurfaceRoutingDecision(decision); err != nil {
		return err
	}
	events, err := l.store.LoadAggregate(ctx, routeAggregate(decision.Request.RouteRequest.SubjectAgentID), 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ID == "event:"+decision.ID {
			existing, decodeErr := decodeSurfaceDecisionEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if existing.ID != decision.ID || inferenceDigest(existing) != inferenceDigest(decision) {
				return errors.New("surface route event identity is already bound to different payload")
			}
			return verifySurfaceEventMetadata(event, existing, decision.Request.RouteRequest.SubjectAgentID)
		}
		if event.Type == routeDecisionEvent {
			payload, _, canonicalErr := routeEventVersions.Canonicalize(event.Version, event.Payload)
			if canonicalErr != nil {
				return canonicalErr
			}
			var route RouteRecord
			if err := json.Unmarshal(payload, &route); err != nil {
				return err
			}
			if err := verifyRouteEventForSurfaceBridge(event, route); err != nil {
				return err
			}
			if route.Request.ID == decision.Request.RouteRequest.ID {
				if err := verifySurfaceRouteBinding(decision, route); err != nil {
					return err
				}
			}
		}
		if event.Type == surfaceRouteDecisionEvent {
			existing, decodeErr := decodeSurfaceDecisionEvent(event)
			if decodeErr != nil {
				return decodeErr
			}
			if existing.Request.ID == decision.Request.ID || existing.Request.RouteRequest.ID == decision.Request.RouteRequest.ID {
				return errors.New("surface route request already has a different decision")
			}
		}
	}
	payload, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, routeAggregate(decision.Request.RouteRequest.SubjectAgentID), int64(len(events)), []eventstore.Event{{
		ID: "event:" + decision.ID, AggregateType: "inference_routes", Type: surfaceRouteDecisionEvent,
		Version: surfaceRouteEventVersions.CurrentVersion(), Actor: decision.Evaluator,
		CommandID: "surface-route:" + decision.ID, CorrelationID: decision.Request.RouteRequest.RunID, CausationID: decision.Request.RouteRequest.ID, Trust: contracts.TrustPolicy,
		Payload: payload, CreatedAt: decision.DecidedAt,
	}})
	return err
}

func (l *RouteLedger) SurfaceDecisions(ctx context.Context, subject string) ([]SurfaceRoutingDecision, error) {
	events, err := l.store.LoadAggregate(ctx, routeAggregate(subject), 0)
	if err != nil {
		return nil, err
	}
	out := []SurfaceRoutingDecision{}
	seenRequests := map[string]bool{}
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
		if route, ok := routes[decision.Request.RouteRequest.ID]; ok {
			if err := verifySurfaceRouteBinding(decision, route); err != nil {
				return nil, err
			}
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
