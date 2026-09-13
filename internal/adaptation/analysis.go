package adaptation

import (
	"errors"
	"math"
	"sort"
)

type ComparisonOperator string

const (
	LessThan           ComparisonOperator = "lt"
	LessThanOrEqual    ComparisonOperator = "lte"
	GreaterThan        ComparisonOperator = "gt"
	GreaterThanOrEqual ComparisonOperator = "gte"
)

type ThresholdRule struct {
	ID                      string             `json:"id"`
	Diagnosis               string             `json:"diagnosis"`
	MeasureName             string             `json:"measure_name"`
	MeasureKind             MeasureKind        `json:"measure_kind"`
	Unit                    string             `json:"unit,omitempty"`
	Operator                ComparisonOperator `json:"operator"`
	Threshold               float64            `json:"threshold"`
	MinimumIndependentRoots int                `json:"minimum_independent_roots"`
}

type AnalysisPolicy struct {
	ID      string          `json:"id"`
	Version string          `json:"version"`
	Rules   []ThresholdRule `json:"rules"`
}

func FreezeAnalysisPolicy(policy AnalysisPolicy) (AnalysisPolicy, error) {
	policy.Version = "v1"
	policy.ID = ""
	policy.Rules = append([]ThresholdRule(nil), policy.Rules...)
	sort.Slice(policy.Rules, func(i, j int) bool { return policy.Rules[i].ID < policy.Rules[j].ID })
	if err := validatePolicy(policy, false); err != nil {
		return AnalysisPolicy{}, err
	}
	digest, err := digestValue(policy)
	if err != nil {
		return AnalysisPolicy{}, err
	}
	policy.ID = "sha256:" + digest
	return policy, nil
}

func VerifyAnalysisPolicy(policy AnalysisPolicy) error {
	if err := validatePolicy(policy, true); err != nil {
		return err
	}
	id := policy.ID
	policy.ID = ""
	digest, err := digestValue(policy)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("analysis policy digest mismatch")
	}
	return nil
}

type Diagnosis struct {
	ID                   string   `json:"id"`
	RuleID               string   `json:"rule_id"`
	Code                 string   `json:"code"`
	MeasurementIDs       []string `json:"measurement_ids"`
	SourceObservationIDs []string `json:"source_observation_ids"`
}

type InvariantFailure struct {
	ObservationID string         `json:"observation_id"`
	Class         InvariantClass `json:"class"`
	ControlID     string         `json:"control_id"`
	EvidenceRef   string         `json:"evidence_ref"`
}

type LongitudinalReport struct {
	ID                string             `json:"id"`
	PolicyID          string             `json:"policy_id"`
	PolicyVersion     string             `json:"policy_version"`
	SubjectAgentID    string             `json:"subject_agent_id"`
	ObservationIDs    []string           `json:"observation_ids"`
	MeasurementIDs    []string           `json:"measurement_ids"`
	Diagnoses         []Diagnosis        `json:"diagnoses,omitempty"`
	InvariantFailures []InvariantFailure `json:"invariant_failures,omitempty"`
}

func EvaluateLongitudinal(observations []Observation, measurements []Measurement, policy AnalysisPolicy) (LongitudinalReport, error) {
	if err := VerifyAnalysisPolicy(policy); err != nil {
		return LongitudinalReport{}, err
	}
	if len(observations) == 0 {
		return LongitudinalReport{}, errors.New("observations are required")
	}
	subject := observations[0].SubjectAgentID
	observationByID := map[string]Observation{}
	report := LongitudinalReport{PolicyID: policy.ID, PolicyVersion: policy.Version, SubjectAgentID: subject}
	for _, observation := range observations {
		if err := VerifyObservation(observation); err != nil {
			return LongitudinalReport{}, err
		}
		if observation.SubjectAgentID != subject {
			return LongitudinalReport{}, errors.New("longitudinal analysis requires one subject agent")
		}
		if observationByID[observation.ID].ID != "" {
			return LongitudinalReport{}, errors.New("duplicate longitudinal observation")
		}
		observationByID[observation.ID] = observation
		report.ObservationIDs = append(report.ObservationIDs, observation.ID)
		for _, invariant := range observation.Invariants {
			if !invariant.Passed {
				report.InvariantFailures = append(report.InvariantFailures, InvariantFailure{ObservationID: observation.ID, Class: invariant.Class, ControlID: invariant.ControlID, EvidenceRef: invariant.EvidenceRef})
			}
		}
	}
	measurementByName := map[string][]Measurement{}
	measurementIDs := map[string]bool{}
	for _, measurement := range measurements {
		if err := VerifyMeasurement(measurement); err != nil {
			return LongitudinalReport{}, err
		}
		if measurement.SubjectAgentID != subject {
			return LongitudinalReport{}, errors.New("measurement subject differs from observation subject")
		}
		if measurementIDs[measurement.ID] {
			return LongitudinalReport{}, errors.New("duplicate longitudinal measurement")
		}
		measurementIDs[measurement.ID] = true
		for _, sourceID := range measurement.Measure.SourceObservationIDs {
			source := observationByID[sourceID]
			if source.ID == "" {
				return LongitudinalReport{}, errors.New("measurement cites unavailable source observation")
			}
			if !sameMeasurementScope(measurement, source) {
				return LongitudinalReport{}, errors.New("measurement source scope mismatch")
			}
		}
		measurementByName[measurement.Measure.Name] = append(measurementByName[measurement.Measure.Name], measurement)
		report.MeasurementIDs = append(report.MeasurementIDs, measurement.ID)
	}
	ruleIDs := map[string]bool{}
	for _, rule := range policy.Rules {
		if err := validateRule(rule); err != nil {
			return LongitudinalReport{}, err
		}
		if ruleIDs[rule.ID] {
			return LongitudinalReport{}, errors.New("duplicate threshold rule identity")
		}
		ruleIDs[rule.ID] = true
		matchedMeasurements, sourceIDs, roots := []string{}, map[string]bool{}, map[string]bool{}
		for _, measurement := range measurementByName[rule.MeasureName] {
			measure := measurement.Measure
			if measure.Kind != rule.MeasureKind || measure.Unit != rule.Unit || !compare(measure.Value, rule.Operator, rule.Threshold) {
				continue
			}
			matchedMeasurements = append(matchedMeasurements, measurement.ID)
			for _, sourceID := range measure.SourceObservationIDs {
				sourceIDs[sourceID] = true
				roots[observationByID[sourceID].CausationRoot] = true
			}
		}
		if len(matchedMeasurements) == 0 || len(roots) < rule.MinimumIndependentRoots {
			continue
		}
		sort.Strings(matchedMeasurements)
		sources := make([]string, 0, len(sourceIDs))
		for id := range sourceIDs {
			sources = append(sources, id)
		}
		sort.Strings(sources)
		diagnosis := Diagnosis{RuleID: rule.ID, Code: rule.Diagnosis, MeasurementIDs: matchedMeasurements, SourceObservationIDs: sources}
		digest, err := digestValue(diagnosis)
		if err != nil {
			return LongitudinalReport{}, err
		}
		diagnosis.ID = "sha256:" + digest
		report.Diagnoses = append(report.Diagnoses, diagnosis)
	}
	sort.Strings(report.ObservationIDs)
	sort.Strings(report.MeasurementIDs)
	sort.Slice(report.InvariantFailures, func(i, j int) bool {
		if report.InvariantFailures[i].ObservationID != report.InvariantFailures[j].ObservationID {
			return report.InvariantFailures[i].ObservationID < report.InvariantFailures[j].ObservationID
		}
		if report.InvariantFailures[i].Class != report.InvariantFailures[j].Class {
			return report.InvariantFailures[i].Class < report.InvariantFailures[j].Class
		}
		return report.InvariantFailures[i].ControlID < report.InvariantFailures[j].ControlID
	})
	payload := report
	payload.ID = ""
	digest, err := digestValue(payload)
	if err != nil {
		return LongitudinalReport{}, err
	}
	report.ID = "sha256:" + digest
	return report, nil
}

func VerifyLongitudinalReport(report LongitudinalReport) error {
	if report.ID == "" || report.PolicyID == "" || report.PolicyVersion == "" || report.SubjectAgentID == "" || len(report.ObservationIDs) == 0 {
		return errors.New("longitudinal report identity, policy, subject, and observations are required")
	}
	for _, diagnosis := range report.Diagnoses {
		if diagnosis.ID == "" || diagnosis.RuleID == "" || diagnosis.Code == "" || len(diagnosis.MeasurementIDs) == 0 || len(diagnosis.SourceObservationIDs) == 0 {
			return errors.New("longitudinal diagnosis is incomplete")
		}
		id := diagnosis.ID
		diagnosis.ID = ""
		digest, err := digestValue(diagnosis)
		if err != nil {
			return err
		}
		if id != "sha256:"+digest {
			return errors.New("longitudinal diagnosis digest mismatch")
		}
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

func TraceDiagnosis(report LongitudinalReport, diagnosisID string) ([]string, []string, error) {
	if err := VerifyLongitudinalReport(report); err != nil {
		return nil, nil, err
	}
	for _, diagnosis := range report.Diagnoses {
		if diagnosis.ID == diagnosisID {
			return append([]string(nil), diagnosis.MeasurementIDs...), append([]string(nil), diagnosis.SourceObservationIDs...), nil
		}
	}
	return nil, nil, errors.New("diagnosis is unavailable")
}

func validateRule(rule ThresholdRule) error {
	if rule.ID == "" || rule.Diagnosis == "" || rule.MeasureName == "" || (rule.MeasureKind != DerivedMeasure && rule.MeasureKind != NormalizedScore) || rule.MinimumIndependentRoots <= 0 || math.IsNaN(rule.Threshold) || math.IsInf(rule.Threshold, 0) {
		return errors.New("threshold rule identity, derived/normalized measure, diagnosis, and evidence threshold are required")
	}
	switch rule.Operator {
	case LessThan, LessThanOrEqual, GreaterThan, GreaterThanOrEqual:
	default:
		return errors.New("unknown comparison operator")
	}
	return nil
}

func validatePolicy(policy AnalysisPolicy, requireID bool) error {
	if requireID && policy.ID == "" {
		return errors.New("analysis policy identity is required")
	}
	if policy.Version != "v1" || len(policy.Rules) == 0 {
		return errors.New("versioned analysis policy rules are required")
	}
	seen := map[string]bool{}
	for _, rule := range policy.Rules {
		if err := validateRule(rule); err != nil {
			return err
		}
		if seen[rule.ID] {
			return errors.New("duplicate threshold rule identity")
		}
		seen[rule.ID] = true
	}
	return nil
}

func compare(value float64, operator ComparisonOperator, threshold float64) bool {
	switch operator {
	case LessThan:
		return value < threshold
	case LessThanOrEqual:
		return value <= threshold
	case GreaterThan:
		return value > threshold
	case GreaterThanOrEqual:
		return value >= threshold
	default:
		return false
	}
}
