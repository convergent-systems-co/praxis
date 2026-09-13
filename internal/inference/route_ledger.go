package inference

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const routeDecisionEvent = "inference.route.decided"

var routeEventVersions = contracts.MustVersionRegistry(contracts.ContractVersionPolicy{Contract: "inference.route_decision_event", CurrentVersion: "v1", Versions: []contracts.ContractVersionDefinition{{Version: "v1", Disposition: contracts.VersionCurrent}}}, nil)

type RouteRecord struct {
	ID          string              `json:"id"`
	Version     string              `json:"version"`
	Request     RouteRequest        `json:"request"`
	Policy      RoutingPolicy       `json:"policy"`
	Eligibility EligibilityEvidence `json:"eligibility"`
	Decision    EvidenceDecision    `json:"decision"`
	DecidedAt   time.Time           `json:"decided_at"`
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
