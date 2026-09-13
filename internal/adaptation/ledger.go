package adaptation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	observationEvent = "adaptive.observation.recorded"
	measurementEvent = "adaptive.measurement.recorded"
	profileFactEvent = "adaptive.profile_fact.recorded"
	analysisEvent    = "adaptive.analysis.recorded"
)

type ConfirmationAuthorizer interface {
	AuthorizeEvidenceConfirmation(ctx context.Context, subjectAgentID, authorityID, evidenceRef string) error
}

type Ledger struct {
	store      eventstore.Store
	authorizer ConfirmationAuthorizer
}

func NewLedger(store eventstore.Store, authorizers ...ConfirmationAuthorizer) (*Ledger, error) {
	if store == nil {
		return nil, errors.New("adaptive event store is required")
	}
	if len(authorizers) > 1 {
		return nil, errors.New("at most one confirmation authorizer may be configured")
	}
	ledger := &Ledger{store: store}
	if len(authorizers) == 1 {
		ledger.authorizer = authorizers[0]
	}
	return ledger, nil
}

func ledgerAggregate(subjectAgentID string) string { return "adaptive-behavior:" + subjectAgentID }

func (l *Ledger) Record(ctx context.Context, observation Observation) error {
	if err := VerifyObservation(observation); err != nil {
		return err
	}
	if observation.Trust == contracts.TrustUserConfirmed {
		if l.authorizer == nil {
			return errors.New("confirmed observation requires deterministic confirmation authorizer")
		}
		if err := l.authorizer.AuthorizeEvidenceConfirmation(ctx, observation.SubjectAgentID, observation.ConfirmationAuthorityID, observation.ConfirmationEvidenceRef); err != nil {
			return fmt.Errorf("authorize confirmed observation: %w", err)
		}
	}
	events, err := l.load(ctx, observation.SubjectAgentID)
	if err != nil {
		return err
	}
	if eventIdentityExists(events, observation.ID) {
		return nil
	}
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	return l.append(ctx, observation.SubjectAgentID, events, eventstore.Event{ID: "event:" + observation.ID, AggregateType: "adaptive_behavior", Type: observationEvent, Version: observationEventContract.CurrentVersion(), Actor: observationActor(observation), CommandID: "record:" + observation.ID, CorrelationID: observation.RunID, CausationID: observation.CausationRoot, Trust: observation.Trust, Payload: payload, CreatedAt: observation.ObservedAt})
}

func (l *Ledger) RecordMeasurement(ctx context.Context, measurement Measurement) error {
	if err := VerifyMeasurement(measurement); err != nil {
		return err
	}
	events, err := l.load(ctx, measurement.SubjectAgentID)
	if err != nil {
		return err
	}
	if eventIdentityExists(events, measurement.ID) {
		return nil
	}
	observations, err := observationsFromEvents(events, measurement.SubjectAgentID)
	if err != nil {
		return err
	}
	available := map[string]Observation{}
	for _, observation := range observations {
		available[observation.ID] = observation
	}
	for _, sourceID := range measurement.Measure.SourceObservationIDs {
		source, ok := available[sourceID]
		if !ok {
			return fmt.Errorf("measurement source observation %s is unavailable in subject ledger", sourceID)
		}
		if !sameMeasurementScope(measurement, source) {
			return errors.New("measurement source observation scope mismatch")
		}
	}
	payload, err := json.Marshal(measurement)
	if err != nil {
		return err
	}
	return l.append(ctx, measurement.SubjectAgentID, events, eventstore.Event{ID: "event:" + measurement.ID, AggregateType: "adaptive_behavior", Type: measurementEvent, Version: measurementEventContract.CurrentVersion(), Actor: contracts.PrincipalRef{ID: measurement.SubjectAgentID, Kind: "agent"}, CommandID: "record:" + measurement.ID, CorrelationID: measurement.ID, Trust: contracts.TrustDerived, Payload: payload, CreatedAt: measurement.Measure.MeasuredAt})
}

func (l *Ledger) RecordProfileFact(ctx context.Context, fact ProfileFact) error {
	if err := VerifyProfileFact(fact); err != nil {
		return err
	}
	if fact.EvidenceClass == Confirmed {
		if l.authorizer == nil {
			return errors.New("confirmed profile fact requires deterministic confirmation authorizer")
		}
		if err := l.authorizer.AuthorizeEvidenceConfirmation(ctx, fact.SubjectAgentID, fact.ConfirmationAuthorityID, fact.ConfirmationEvidenceRef); err != nil {
			return fmt.Errorf("authorize confirmed profile fact: %w", err)
		}
	}
	events, err := l.load(ctx, fact.SubjectAgentID)
	if err != nil {
		return err
	}
	if eventIdentityExists(events, fact.ID) {
		return nil
	}
	if err := verifyProfileSources(events, fact); err != nil {
		return err
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return err
	}
	return l.append(ctx, fact.SubjectAgentID, events, eventstore.Event{ID: "event:" + fact.ID, AggregateType: "adaptive_behavior", Type: profileFactEvent, Version: profileFactEventContract.CurrentVersion(), Actor: profileFactActor(fact), CommandID: "record:" + fact.ID, CorrelationID: fact.ID, Trust: profileFactTrust(fact.EvidenceClass), Payload: payload, CreatedAt: fact.RecordedAt})
}

func (l *Ledger) RecordAnalysis(ctx context.Context, record AnalysisRecord) error {
	if err := VerifyAnalysisRecord(record); err != nil {
		return err
	}
	events, err := l.load(ctx, record.Report.SubjectAgentID)
	if err != nil {
		return err
	}
	if eventIdentityExists(events, record.Report.ID) {
		return nil
	}
	observations, err := observationsFromEvents(events, record.Report.SubjectAgentID)
	if err != nil {
		return err
	}
	measurements, err := measurementsFromEvents(events, record.Report.SubjectAgentID)
	if err != nil {
		return err
	}
	availableObservations, availableMeasurements := map[string]Observation{}, map[string]Measurement{}
	for _, observation := range observations {
		availableObservations[observation.ID] = observation
	}
	for _, measurement := range measurements {
		availableMeasurements[measurement.ID] = measurement
	}
	if err := verifyAnalysisDerivation(record, availableObservations, availableMeasurements); err != nil {
		return err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return l.append(ctx, record.Report.SubjectAgentID, events, eventstore.Event{ID: "event:" + record.Report.ID, AggregateType: "adaptive_behavior", Type: analysisEvent, Version: analysisEventContract.CurrentVersion(), Actor: contracts.PrincipalRef{ID: record.Report.SubjectAgentID, Kind: "agent"}, CommandID: "record:" + record.Report.ID, CorrelationID: record.Report.ID, Trust: contracts.TrustDerived, Payload: payload, CreatedAt: record.Report.EvaluatedAt})
}

func (l *Ledger) Observations(ctx context.Context, subjectAgentID string) ([]Observation, error) {
	events, err := l.load(ctx, subjectAgentID)
	if err != nil {
		return nil, err
	}
	return observationsFromEvents(events, subjectAgentID)
}

func (l *Ledger) Measurements(ctx context.Context, subjectAgentID string) ([]Measurement, error) {
	events, err := l.load(ctx, subjectAgentID)
	if err != nil {
		return nil, err
	}
	return measurementsFromEvents(events, subjectAgentID)
}

func (l *Ledger) ProfileHistory(ctx context.Context, subjectAgentID string) ([]ProfileFact, error) {
	events, err := l.load(ctx, subjectAgentID)
	if err != nil {
		return nil, err
	}
	return profileFactsFromEvents(events, subjectAgentID)
}

func (l *Ledger) AnalysisHistory(ctx context.Context, subjectAgentID string) ([]AnalysisRecord, error) {
	events, err := l.load(ctx, subjectAgentID)
	if err != nil {
		return nil, err
	}
	return analysisRecordsFromEvents(events, subjectAgentID)
}

func (l *Ledger) load(ctx context.Context, subjectAgentID string) ([]eventstore.Event, error) {
	if l == nil || l.store == nil || subjectAgentID == "" {
		return nil, errors.New("adaptive ledger and subject agent are required")
	}
	return l.store.LoadAggregate(ctx, ledgerAggregate(subjectAgentID), 0)
}

func (l *Ledger) append(ctx context.Context, subject string, events []eventstore.Event, event eventstore.Event) error {
	_, err := l.store.Append(ctx, ledgerAggregate(subject), int64(len(events)), []eventstore.Event{event})
	return err
}

func eventIdentityExists(events []eventstore.Event, identity string) bool {
	for _, event := range events {
		if event.ID == "event:"+identity {
			return true
		}
	}
	return false
}

func observationsFromEvents(events []eventstore.Event, subject string) ([]Observation, error) {
	out := []Observation{}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != observationEvent {
			continue
		}
		payload, _, err := observationEventContract.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var observation Observation
		if err := json.Unmarshal(payload, &observation); err != nil {
			return nil, err
		}
		if err := VerifyObservation(observation); err != nil {
			return nil, err
		}
		if observation.SubjectAgentID != subject || event.ID != "event:"+observation.ID || event.CorrelationID != observation.RunID || event.CausationID != observation.CausationRoot || event.Trust != observation.Trust || event.Actor != observationActor(observation) {
			return nil, errors.New("adaptive event metadata does not bind observation")
		}
		if seen[observation.ID] {
			return nil, errors.New("duplicate adaptive observation event")
		}
		seen[observation.ID] = true
		out = append(out, observation)
	}
	return out, nil
}

func measurementsFromEvents(events []eventstore.Event, subject string) ([]Measurement, error) {
	out := []Measurement{}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != measurementEvent {
			continue
		}
		payload, _, err := measurementEventContract.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var measurement Measurement
		if err := json.Unmarshal(payload, &measurement); err != nil {
			return nil, err
		}
		if err := VerifyMeasurement(measurement); err != nil {
			return nil, err
		}
		if measurement.SubjectAgentID != subject || event.ID != "event:"+measurement.ID || event.Trust != contracts.TrustDerived {
			return nil, errors.New("adaptive event metadata does not bind measurement")
		}
		if seen[measurement.ID] {
			return nil, errors.New("duplicate adaptive measurement event")
		}
		seen[measurement.ID] = true
		out = append(out, measurement)
	}
	return out, nil
}

func profileFactsFromEvents(events []eventstore.Event, subject string) ([]ProfileFact, error) {
	out := []ProfileFact{}
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type != profileFactEvent {
			continue
		}
		payload, _, err := profileFactEventContract.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var fact ProfileFact
		if err := json.Unmarshal(payload, &fact); err != nil {
			return nil, err
		}
		if err := VerifyProfileFact(fact); err != nil {
			return nil, err
		}
		if fact.SubjectAgentID != subject || event.ID != "event:"+fact.ID || event.Trust != profileFactTrust(fact.EvidenceClass) || event.Actor != profileFactActor(fact) {
			return nil, errors.New("adaptive event metadata does not bind profile fact")
		}
		if seen[fact.ID] {
			return nil, errors.New("duplicate adaptive profile fact event")
		}
		seen[fact.ID] = true
		out = append(out, fact)
	}
	return out, nil
}

func analysisRecordsFromEvents(events []eventstore.Event, subject string) ([]AnalysisRecord, error) {
	out := []AnalysisRecord{}
	seen := map[string]bool{}
	observations, err := observationsFromEvents(events, subject)
	if err != nil {
		return nil, err
	}
	measurements, err := measurementsFromEvents(events, subject)
	if err != nil {
		return nil, err
	}
	availableObservations, availableMeasurements := map[string]Observation{}, map[string]Measurement{}
	for _, observation := range observations {
		availableObservations[observation.ID] = observation
	}
	for _, measurement := range measurements {
		availableMeasurements[measurement.ID] = measurement
	}
	for _, event := range events {
		if event.Type != analysisEvent {
			continue
		}
		payload, _, err := analysisEventContract.Canonicalize(event.Version, event.Payload)
		if err != nil {
			return nil, err
		}
		var record AnalysisRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, err
		}
		if err := VerifyAnalysisRecord(record); err != nil {
			return nil, err
		}
		if record.Report.SubjectAgentID != subject || event.ID != "event:"+record.Report.ID || event.Trust != contracts.TrustDerived || event.Actor != (contracts.PrincipalRef{ID: subject, Kind: "agent"}) {
			return nil, errors.New("adaptive event metadata does not bind analysis record")
		}
		if err := verifyAnalysisDerivation(record, availableObservations, availableMeasurements); err != nil {
			return nil, err
		}
		if seen[record.Report.ID] {
			return nil, errors.New("duplicate adaptive analysis event")
		}
		seen[record.Report.ID] = true
		out = append(out, record)
	}
	return out, nil
}

func verifyAnalysisDerivation(record AnalysisRecord, availableObservations map[string]Observation, availableMeasurements map[string]Measurement) error {
	observations := make([]Observation, 0, len(record.Report.ObservationIDs))
	for _, id := range record.Report.ObservationIDs {
		observation, ok := availableObservations[id]
		if !ok {
			return errors.New("analysis report observation is unavailable in subject ledger")
		}
		observations = append(observations, observation)
	}
	measurements := make([]Measurement, 0, len(record.Report.MeasurementIDs))
	for _, id := range record.Report.MeasurementIDs {
		measurement, ok := availableMeasurements[id]
		if !ok {
			return errors.New("analysis report measurement is unavailable in subject ledger")
		}
		measurements = append(measurements, measurement)
	}
	recomputed, err := EvaluateLongitudinal(observations, measurements, record.Policy, record.Report.EvaluatedAt)
	if err != nil {
		return fmt.Errorf("recompute analysis report: %w", err)
	}
	if recomputed.ID != record.Report.ID {
		return errors.New("analysis report is not the deterministic result of its cited evidence and policy")
	}
	return nil
}

func verifyProfileSources(events []eventstore.Event, fact ProfileFact) error {
	observations, err := observationsFromEvents(events, fact.SubjectAgentID)
	if err != nil {
		return err
	}
	measurements, err := measurementsFromEvents(events, fact.SubjectAgentID)
	if err != nil {
		return err
	}
	availableObservations, availableMeasurements := map[string]Observation{}, map[string]Measurement{}
	for _, observation := range observations {
		availableObservations[observation.ID] = observation
	}
	for _, measurement := range measurements {
		availableMeasurements[measurement.ID] = measurement
	}
	for _, id := range fact.SourceObservationIDs {
		observation, ok := availableObservations[id]
		if !ok {
			return errors.New("profile source observation is unavailable")
		}
		if observation.Context != fact.Context {
			return errors.New("profile source observation context mismatch")
		}
	}
	for _, id := range fact.SourceMeasurementIDs {
		measurement, ok := availableMeasurements[id]
		if !ok {
			return errors.New("profile source measurement is unavailable")
		}
		if measurement.Context != fact.Context {
			return errors.New("profile source measurement context mismatch")
		}
	}
	return nil
}

func observationActor(observation Observation) contracts.PrincipalRef {
	if observation.Trust == contracts.TrustUserConfirmed {
		return contracts.PrincipalRef{ID: observation.ConfirmationAuthorityID, Kind: "human"}
	}
	return contracts.PrincipalRef{ID: observation.SubjectAgentID, Kind: "agent"}
}

func profileFactActor(fact ProfileFact) contracts.PrincipalRef {
	if fact.EvidenceClass == Confirmed {
		return contracts.PrincipalRef{ID: fact.ConfirmationAuthorityID, Kind: "human"}
	}
	return contracts.PrincipalRef{ID: fact.SubjectAgentID, Kind: "agent"}
}
