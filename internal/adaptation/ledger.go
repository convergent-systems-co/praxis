package adaptation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type EvidenceClass string

const (
	Declared  EvidenceClass = "declared"
	Inherited EvidenceClass = "inherited"
	Observed  EvidenceClass = "observed"
	Measured  EvidenceClass = "measured"
	Confirmed EvidenceClass = "confirmed"
)

type Observation struct {
	ID                 string               `json:"id"`
	Version            string               `json:"version"`
	SubjectAgentID     string               `json:"subject_agent_id"`
	RunID              string               `json:"run_id"`
	GoalClass          string               `json:"goal_class"`
	Domain             string               `json:"domain"`
	BehaviorKey        string               `json:"behavior_key"`
	Context            string               `json:"context"`
	CausationRoot      string               `json:"causation_root"`
	Trust              contracts.TrustClass `json:"trust"`
	ReasoningTier      string               `json:"reasoning_tier"`
	ProviderID         string               `json:"provider_id,omitempty"`
	Successful         bool                 `json:"successful"`
	Quality            float64              `json:"quality"`
	InferenceTokens    int                  `json:"inference_tokens"`
	PathID             string               `json:"path_id"`
	PolicyViolations   int                  `json:"policy_violations"`
	SecurityViolations int                  `json:"security_violations"`
	Measures           map[string]float64   `json:"measures,omitempty"`
	ObservedAt         time.Time            `json:"observed_at"`
}

func FreezeObservation(observation Observation) (Observation, error) {
	observation.Version = "v1"
	observation.ID = ""
	if err := validateObservation(observation, false); err != nil {
		return Observation{}, err
	}
	digest, err := digestValue(observation)
	if err != nil {
		return Observation{}, err
	}
	observation.ID = "sha256:" + digest
	return observation, nil
}

func VerifyObservation(observation Observation) error {
	if err := validateObservation(observation, true); err != nil {
		return err
	}
	id := observation.ID
	observation.ID = ""
	digest, err := digestValue(observation)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("adaptive observation digest mismatch")
	}
	return nil
}

func validateObservation(observation Observation, requireID bool) error {
	if requireID && observation.ID == "" {
		return errors.New("adaptive observation identity is required")
	}
	if observation.Version != "v1" || observation.SubjectAgentID == "" || observation.RunID == "" || observation.GoalClass == "" || observation.Domain == "" || observation.BehaviorKey == "" || observation.Context == "" || observation.CausationRoot == "" || observation.ReasoningTier == "" || observation.PathID == "" || observation.ObservedAt.IsZero() {
		return errors.New("adaptive observation scope, behavior, path, and time are required")
	}
	if observation.Quality < 0 || observation.Quality > 1 || observation.InferenceTokens < 0 || observation.PolicyViolations < 0 || observation.SecurityViolations < 0 {
		return errors.New("adaptive observation metrics are invalid")
	}
	switch observation.Trust {
	case contracts.TrustObserved, contracts.TrustDerived, contracts.TrustUserConfirmed:
	default:
		return errors.New("adaptive observation requires observed, derived, or user-confirmed trust")
	}
	for name, value := range observation.Measures {
		if name == "" || value < 0 || value > 1 {
			return errors.New("adaptive observation measures require names and values in [0,1]")
		}
	}
	return nil
}

type Ledger struct {
	store eventstore.Store
}

func NewLedger(store eventstore.Store) (*Ledger, error) {
	if store == nil {
		return nil, errors.New("adaptive event store is required")
	}
	return &Ledger{store: store}, nil
}

func ledgerAggregate(subjectAgentID string) string { return "adaptive-behavior:" + subjectAgentID }

func (l *Ledger) Record(ctx context.Context, observation Observation) error {
	if err := VerifyObservation(observation); err != nil {
		return err
	}
	events, err := l.store.LoadAggregate(ctx, ledgerAggregate(observation.SubjectAgentID), 0)
	if err != nil {
		return err
	}
	existing, err := observationsFromEvents(events, observation.SubjectAgentID)
	if err != nil {
		return err
	}
	for _, recorded := range existing {
		if recorded.ID == observation.ID {
			return nil
		}
	}
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	event := eventstore.Event{
		ID:            "event:" + observation.ID,
		AggregateType: "adaptive_behavior",
		Type:          "adaptive.observation.recorded",
		Version:       "v1",
		Actor:         contracts.PrincipalRef{ID: observation.SubjectAgentID, Kind: "agent"},
		CommandID:     "record:" + observation.ID,
		CorrelationID: observation.RunID,
		CausationID:   observation.CausationRoot,
		Trust:         observation.Trust,
		Payload:       payload,
		CreatedAt:     observation.ObservedAt,
	}
	_, err = l.store.Append(ctx, ledgerAggregate(observation.SubjectAgentID), int64(len(events)), []eventstore.Event{event})
	return err
}

func (l *Ledger) Observations(ctx context.Context, subjectAgentID string) ([]Observation, error) {
	if l == nil || l.store == nil || subjectAgentID == "" {
		return nil, errors.New("adaptive ledger and subject agent are required")
	}
	events, err := l.store.LoadAggregate(ctx, ledgerAggregate(subjectAgentID), 0)
	if err != nil {
		return nil, err
	}
	return observationsFromEvents(events, subjectAgentID)
}

func observationsFromEvents(events []eventstore.Event, subjectAgentID string) ([]Observation, error) {
	observations := make([]Observation, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type == "adaptive.profile_fact.recorded" {
			continue
		}
		if event.Type != "adaptive.observation.recorded" || event.Version != "v1" {
			return nil, fmt.Errorf("unknown adaptive event %q@%s", event.Type, event.Version)
		}
		var observation Observation
		if err := json.Unmarshal(event.Payload, &observation); err != nil {
			return nil, err
		}
		if err := VerifyObservation(observation); err != nil {
			return nil, err
		}
		if observation.SubjectAgentID != subjectAgentID || event.ID != "event:"+observation.ID || event.CorrelationID != observation.RunID || event.CausationID != observation.CausationRoot || event.Trust != observation.Trust {
			return nil, errors.New("adaptive event metadata does not bind observation")
		}
		if seen[observation.ID] {
			return nil, errors.New("duplicate adaptive observation event")
		}
		seen[observation.ID] = true
		observations = append(observations, observation)
	}
	return observations, nil
}

type ProfileFact struct {
	ID                   string        `json:"id"`
	Version              string        `json:"version"`
	SubjectAgentID       string        `json:"subject_agent_id"`
	Dimension            string        `json:"dimension"`
	Value                float64       `json:"value"`
	EvidenceClass        EvidenceClass `json:"evidence_class"`
	Confidence           float64       `json:"confidence"`
	SampleSize           int           `json:"sample_size"`
	EvaluatorVersion     string        `json:"evaluator_version,omitempty"`
	Provenance           string        `json:"provenance"`
	Context              string        `json:"context"`
	SourceObservationIDs []string      `json:"source_observation_ids,omitempty"`
	RecordedAt           time.Time     `json:"recorded_at"`
}

func FreezeProfileFact(fact ProfileFact) (ProfileFact, error) {
	fact.Version = "v1"
	fact.ID = ""
	fact.SourceObservationIDs = append([]string(nil), fact.SourceObservationIDs...)
	sort.Strings(fact.SourceObservationIDs)
	if err := validateProfileFact(fact, false); err != nil {
		return ProfileFact{}, err
	}
	digest, err := digestValue(fact)
	if err != nil {
		return ProfileFact{}, err
	}
	fact.ID = "sha256:" + digest
	return fact, nil
}

func VerifyProfileFact(fact ProfileFact) error {
	if err := validateProfileFact(fact, true); err != nil {
		return err
	}
	id := fact.ID
	fact.ID = ""
	digest, err := digestValue(fact)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("behavioral profile fact digest mismatch")
	}
	return nil
}

func validateProfileFact(fact ProfileFact, requireID bool) error {
	if requireID && fact.ID == "" {
		return errors.New("behavioral profile fact identity is required")
	}
	if fact.Version != "v1" || fact.SubjectAgentID == "" || fact.Dimension == "" || fact.Provenance == "" || fact.Context == "" || fact.RecordedAt.IsZero() {
		return errors.New("behavioral profile fact scope, provenance, and time are required")
	}
	if fact.Value < 0 || fact.Value > 1 || fact.Confidence < 0 || fact.Confidence > 1 || fact.SampleSize < 0 {
		return errors.New("behavioral profile fact metrics are invalid")
	}
	switch fact.EvidenceClass {
	case Declared, Inherited, Confirmed:
		if fact.SampleSize != 0 || fact.EvaluatorVersion != "" || len(fact.SourceObservationIDs) != 0 {
			return errors.New("declared, inherited, and confirmed profile facts cannot masquerade as execution measurement")
		}
	case Observed, Measured:
		if fact.SampleSize <= 0 || fact.EvaluatorVersion == "" || len(fact.SourceObservationIDs) == 0 {
			return errors.New("observed and measured profile facts require evaluator and source observations")
		}
	default:
		return errors.New("unknown behavioral profile evidence class")
	}
	return nil
}

func (l *Ledger) RecordProfileFact(ctx context.Context, fact ProfileFact) error {
	if err := VerifyProfileFact(fact); err != nil {
		return err
	}
	events, err := l.store.LoadAggregate(ctx, ledgerAggregate(fact.SubjectAgentID), 0)
	if err != nil {
		return err
	}
	facts, err := profileFactsFromEvents(events, fact.SubjectAgentID)
	if err != nil {
		return err
	}
	for _, existing := range facts {
		if existing.ID == fact.ID {
			return nil
		}
	}
	payload, err := json.Marshal(fact)
	if err != nil {
		return err
	}
	event := eventstore.Event{
		ID: "event:" + fact.ID, AggregateType: "adaptive_behavior", Type: "adaptive.profile_fact.recorded", Version: "v1",
		Actor: contracts.PrincipalRef{ID: fact.SubjectAgentID, Kind: "agent"}, CommandID: "record:" + fact.ID,
		CorrelationID: fact.ID, Trust: profileFactTrust(fact.EvidenceClass), Payload: payload, CreatedAt: fact.RecordedAt,
	}
	_, err = l.store.Append(ctx, ledgerAggregate(fact.SubjectAgentID), int64(len(events)), []eventstore.Event{event})
	return err
}

func (l *Ledger) ProfileHistory(ctx context.Context, subjectAgentID string) ([]ProfileFact, error) {
	if l == nil || l.store == nil || subjectAgentID == "" {
		return nil, errors.New("adaptive ledger and subject agent are required")
	}
	events, err := l.store.LoadAggregate(ctx, ledgerAggregate(subjectAgentID), 0)
	if err != nil {
		return nil, err
	}
	return profileFactsFromEvents(events, subjectAgentID)
}

func profileFactsFromEvents(events []eventstore.Event, subjectAgentID string) ([]ProfileFact, error) {
	facts := make([]ProfileFact, 0, len(events))
	seen := map[string]bool{}
	for _, event := range events {
		if event.Type == "adaptive.observation.recorded" {
			continue
		}
		if event.Type != "adaptive.profile_fact.recorded" || event.Version != "v1" {
			return nil, fmt.Errorf("unknown adaptive event %q@%s", event.Type, event.Version)
		}
		var fact ProfileFact
		if err := json.Unmarshal(event.Payload, &fact); err != nil {
			return nil, err
		}
		if err := VerifyProfileFact(fact); err != nil {
			return nil, err
		}
		if fact.SubjectAgentID != subjectAgentID || event.ID != "event:"+fact.ID || event.Trust != profileFactTrust(fact.EvidenceClass) {
			return nil, errors.New("adaptive event metadata does not bind profile fact")
		}
		if seen[fact.ID] {
			return nil, errors.New("duplicate behavioral profile fact event")
		}
		seen[fact.ID] = true
		facts = append(facts, fact)
	}
	return facts, nil
}

func profileFactTrust(class EvidenceClass) contracts.TrustClass {
	switch class {
	case Confirmed:
		return contracts.TrustUserConfirmed
	case Declared, Inherited:
		return contracts.TrustObserved
	default:
		return contracts.TrustDerived
	}
}

type ProfileRule struct {
	Dimension        string        `json:"dimension"`
	Measure          string        `json:"measure"`
	EvidenceClass    EvidenceClass `json:"evidence_class"`
	EvaluatorVersion string        `json:"evaluator_version"`
}

func DeriveProfileFacts(observations []Observation, rules []ProfileRule, recordedAt time.Time) ([]ProfileFact, error) {
	if len(observations) == 0 || len(rules) == 0 || recordedAt.IsZero() {
		return nil, errors.New("observations, profile rules, and record time are required")
	}
	subject, contextName := observations[0].SubjectAgentID, observations[0].Context
	for _, observation := range observations {
		if err := VerifyObservation(observation); err != nil {
			return nil, err
		}
		if observation.SubjectAgentID != subject {
			return nil, errors.New("profile derivation requires one subject agent")
		}
	}
	facts := make([]ProfileFact, 0, len(rules))
	for _, rule := range rules {
		if rule.Dimension == "" || rule.Measure == "" || rule.EvaluatorVersion == "" || (rule.EvidenceClass != Observed && rule.EvidenceClass != Measured) {
			return nil, errors.New("profile rule requires observed/measured class and evaluator")
		}
		var sum float64
		ids := []string{}
		for _, observation := range observations {
			value, ok := observation.Measures[rule.Measure]
			if !ok {
				continue
			}
			sum += value
			ids = append(ids, observation.ID)
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("profile measure %q is unavailable", rule.Measure)
		}
		fact, err := FreezeProfileFact(ProfileFact{SubjectAgentID: subject, Dimension: rule.Dimension, Value: sum / float64(len(ids)), EvidenceClass: rule.EvidenceClass, Confidence: float64(len(ids)) / float64(len(observations)), SampleSize: len(ids), EvaluatorVersion: rule.EvaluatorVersion, Provenance: "execution-ledger", Context: contextName, SourceObservationIDs: ids, RecordedAt: recordedAt})
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

type AnalysisPolicy struct {
	ID                        string  `json:"id"`
	MinimumIndependentRoots   int     `json:"minimum_independent_roots"`
	RegressionQualityDrop     float64 `json:"regression_quality_drop"`
	MaximumPathVariants       int     `json:"maximum_path_variants"`
	PortabilityMinimumSamples int     `json:"portability_minimum_samples"`
}

type BehaviorSignal struct {
	BehaviorKey string   `json:"behavior_key"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type LongitudinalReport struct {
	ID                  string           `json:"id"`
	PolicyID            string           `json:"policy_id"`
	SubjectAgentID      string           `json:"subject_agent_id"`
	EvidenceIDs         []string         `json:"evidence_ids"`
	RepeatedInference   []BehaviorSignal `json:"repeated_inference,omitempty"`
	Regressions         []BehaviorSignal `json:"regressions,omitempty"`
	PathVariance        []BehaviorSignal `json:"path_variance,omitempty"`
	PortabilityFailures []BehaviorSignal `json:"portability_failures,omitempty"`
	PolicyViolations    int              `json:"policy_violations"`
	SecurityViolations  int              `json:"security_violations"`
}

func VerifyLongitudinalReport(report LongitudinalReport) error {
	if report.ID == "" || report.PolicyID == "" || report.SubjectAgentID == "" || len(report.EvidenceIDs) == 0 {
		return errors.New("longitudinal report identity, policy, subject, and evidence are required")
	}
	id := report.ID
	report.ID = ""
	digest, err := digestValue(report)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("longitudinal report digest mismatch")
	}
	return nil
}

func Analyze(observations []Observation, policy AnalysisPolicy) (LongitudinalReport, error) {
	if policy.ID == "" || policy.MinimumIndependentRoots <= 0 || policy.RegressionQualityDrop < 0 || policy.RegressionQualityDrop > 1 || policy.MaximumPathVariants <= 0 || policy.PortabilityMinimumSamples <= 0 {
		return LongitudinalReport{}, errors.New("valid versioned adaptive analysis policy is required")
	}
	if len(observations) == 0 {
		return LongitudinalReport{}, errors.New("adaptive observations are required")
	}
	subject := observations[0].SubjectAgentID
	byBehavior := map[string][]Observation{}
	report := LongitudinalReport{PolicyID: policy.ID, SubjectAgentID: subject}
	for _, observation := range observations {
		if err := VerifyObservation(observation); err != nil {
			return LongitudinalReport{}, err
		}
		if observation.SubjectAgentID != subject {
			return LongitudinalReport{}, errors.New("longitudinal analysis requires one subject agent")
		}
		byBehavior[observation.BehaviorKey] = append(byBehavior[observation.BehaviorKey], observation)
		report.EvidenceIDs = append(report.EvidenceIDs, observation.ID)
		report.PolicyViolations += observation.PolicyViolations
		report.SecurityViolations += observation.SecurityViolations
	}
	sort.Strings(report.EvidenceIDs)
	keys := make([]string, 0, len(byBehavior))
	for key := range byBehavior {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		series := byBehavior[key]
		sort.SliceStable(series, func(i, j int) bool { return series[i].ObservedAt.Before(series[j].ObservedAt) })
		roots, paths, providers := map[string]bool{}, map[string]bool{}, map[string][]Observation{}
		ids := make([]string, 0, len(series))
		var priorQuality float64
		var priorCount int
		regression := false
		for _, observation := range series {
			ids = append(ids, observation.ID)
			paths[observation.PathID] = true
			providers[observation.ProviderID] = append(providers[observation.ProviderID], observation)
			if observation.ReasoningTier != "D0" && observation.InferenceTokens > 0 && observation.Successful && observation.PolicyViolations == 0 && observation.SecurityViolations == 0 {
				roots[observation.CausationRoot] = true
			}
			if priorCount > 0 && priorQuality/float64(priorCount)-observation.Quality >= policy.RegressionQualityDrop {
				regression = true
			}
			priorQuality += observation.Quality
			priorCount++
		}
		sort.Strings(ids)
		signal := BehaviorSignal{BehaviorKey: key, EvidenceIDs: ids}
		if len(roots) >= policy.MinimumIndependentRoots {
			report.RepeatedInference = append(report.RepeatedInference, signal)
		}
		if regression {
			report.Regressions = append(report.Regressions, signal)
		}
		if len(paths) > policy.MaximumPathVariants {
			report.PathVariance = append(report.PathVariance, signal)
		}
		providerSuccess, providerFailure := false, false
		for provider, samples := range providers {
			if provider == "" || len(samples) < policy.PortabilityMinimumSamples {
				continue
			}
			allSuccess := true
			for _, sample := range samples {
				if !sample.Successful {
					allSuccess = false
				}
			}
			if allSuccess {
				providerSuccess = true
			} else {
				providerFailure = true
			}
		}
		if providerSuccess && providerFailure {
			report.PortabilityFailures = append(report.PortabilityFailures, signal)
		}
	}
	payload := report
	payload.ID = ""
	digest, err := digestValue(payload)
	if err != nil {
		return LongitudinalReport{}, err
	}
	report.ID = "sha256:" + digest
	return report, nil
}

func digestValue(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}
