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
var surfaceRouteEventVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "inference.surface_route_decision_event", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)

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
	events, err := l.store.LoadAggregate(ctx, routeAggregate(decision.Request.SubjectAgentID), 0)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.ID == "event:"+decision.ID {
			return nil
		}
		if event.Type != surfaceRouteDecisionEvent {
			continue
		}
		payload, _, canonicalErr := surfaceRouteEventVersions.Canonicalize(event.Version, event.Payload)
		if canonicalErr != nil {
			return canonicalErr
		}
		var existing SurfaceRoutingDecision
		if err := json.Unmarshal(payload, &existing); err != nil {
			return err
		}
		if existing.Request.ID == decision.Request.ID {
			return errors.New("surface route request already has a different decision")
		}
	}
	payload, err := json.Marshal(decision)
	if err != nil {
		return err
	}
	_, err = l.store.Append(ctx, routeAggregate(decision.Request.SubjectAgentID), int64(len(events)), []eventstore.Event{{
		ID: "event:" + decision.ID, AggregateType: "inference_routes", Type: surfaceRouteDecisionEvent,
		Version: surfaceRouteEventVersions.CurrentVersion(), Actor: contracts.PrincipalRef{ID: decision.Request.Target.Target.SourceAuthority, Kind: "authority"},
		CommandID: "surface-route:" + decision.ID, CorrelationID: decision.Request.RunID, Trust: contracts.TrustPolicy,
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
	for _, event := range events {
		if event.Type != surfaceRouteDecisionEvent {
			continue
		}
		payload, _, err := surfaceRouteEventVersions.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var decision SurfaceRoutingDecision
		if err := json.Unmarshal(payload, &decision); err != nil {
			return nil, err
		}
		if err := VerifySurfaceRoutingDecision(decision); err != nil {
			return nil, err
		}
		if decision.Request.SubjectAgentID != subject || event.ID != "event:"+decision.ID || event.Actor != (contracts.PrincipalRef{ID: decision.Request.Target.Target.SourceAuthority, Kind: "authority"}) || event.CorrelationID != decision.Request.RunID || event.Trust != contracts.TrustPolicy || seenRequests[decision.Request.ID] {
			return nil, errors.New("surface route event metadata or request lineage is invalid")
		}
		seenRequests[decision.Request.ID] = true
		out = append(out, decision)
	}
	return out, nil
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
