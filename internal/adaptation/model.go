package adaptation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type MeasureKind string

const (
	RawMeasure      MeasureKind = "raw"
	DerivedMeasure  MeasureKind = "derived"
	NormalizedScore MeasureKind = "normalized_score"
)

type NumericRange struct {
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
}

type EvaluatorRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Measure struct {
	Name                 string        `json:"name"`
	Value                float64       `json:"value"`
	Kind                 MeasureKind   `json:"kind"`
	Unit                 string        `json:"unit,omitempty"`
	NormalizedRange      *NumericRange `json:"normalized_range,omitempty"`
	Evaluator            *EvaluatorRef `json:"evaluator,omitempty"`
	TransformID          string        `json:"transform_id,omitempty"`
	Provenance           string        `json:"provenance"`
	SourceObservationIDs []string      `json:"source_observation_ids,omitempty"`
	MeasuredAt           time.Time     `json:"measured_at"`
}

func (measure Measure) Validate() error {
	if measure.Name == "" || measure.Provenance == "" || measure.MeasuredAt.IsZero() || math.IsNaN(measure.Value) || math.IsInf(measure.Value, 0) {
		return errors.New("measure name, finite value, provenance, and time are required")
	}
	switch measure.Kind {
	case RawMeasure:
		if measure.NormalizedRange != nil || measure.Evaluator != nil || measure.TransformID != "" || len(measure.SourceObservationIDs) != 0 {
			return errors.New("raw measure cannot claim derivation or normalization metadata")
		}
	case DerivedMeasure:
		if measure.NormalizedRange != nil {
			return errors.New("derived measure cannot imply a normalized range")
		}
		if err := validateDerivation(measure); err != nil {
			return err
		}
	case NormalizedScore:
		if err := validateDerivation(measure); err != nil {
			return err
		}
		if measure.NormalizedRange == nil || math.IsNaN(measure.NormalizedRange.Minimum) || math.IsNaN(measure.NormalizedRange.Maximum) || measure.NormalizedRange.Maximum <= measure.NormalizedRange.Minimum || measure.Value < measure.NormalizedRange.Minimum || measure.Value > measure.NormalizedRange.Maximum {
			return errors.New("normalized score requires an explicit range containing its value")
		}
	default:
		return errors.New("unknown measure semantic kind")
	}
	return nil
}

func validateDerivation(measure Measure) error {
	if measure.Evaluator == nil || measure.Evaluator.ID == "" || measure.Evaluator.Version == "" || measure.TransformID == "" || len(measure.SourceObservationIDs) == 0 {
		return errors.New("derived measure requires evaluator, transform, and source observations")
	}
	return nil
}

type InvariantClass string

const (
	PolicyInvariant   InvariantClass = "policy"
	SecurityInvariant InvariantClass = "security"
)

type InvariantResult struct {
	Class       InvariantClass `json:"class"`
	ControlID   string         `json:"control_id"`
	Passed      bool           `json:"passed"`
	EvidenceRef string         `json:"evidence_ref"`
}

type Observation struct {
	ID                      string               `json:"id"`
	Version                 string               `json:"version"`
	SubjectAgentID          string               `json:"subject_agent_id"`
	RunID                   string               `json:"run_id"`
	GoalClass               string               `json:"goal_class"`
	Domain                  string               `json:"domain"`
	BehaviorKey             string               `json:"behavior_key"`
	Context                 string               `json:"context"`
	CausationRoot           string               `json:"causation_root"`
	Trust                   contracts.TrustClass `json:"trust"`
	ConfirmationAuthorityID string               `json:"confirmation_authority_id,omitempty"`
	ConfirmationEvidenceRef string               `json:"confirmation_evidence_ref,omitempty"`
	ReasoningTier           string               `json:"reasoning_tier,omitempty"`
	ProviderID              string               `json:"provider_id,omitempty"`
	Outcome                 string               `json:"outcome"`
	PathID                  string               `json:"path_id"`
	RawMeasures             []Measure            `json:"raw_measures,omitempty"`
	Invariants              []InvariantResult    `json:"invariants,omitempty"`
	ObservedAt              time.Time            `json:"observed_at"`
}

func FreezeObservation(observation Observation) (Observation, error) {
	observation.Version = observationEventContract.CurrentVersion()
	observation.ID = ""
	observation.RawMeasures = append([]Measure(nil), observation.RawMeasures...)
	sort.Slice(observation.RawMeasures, func(i, j int) bool { return measureLess(observation.RawMeasures[i], observation.RawMeasures[j]) })
	observation.Invariants = append([]InvariantResult(nil), observation.Invariants...)
	sort.Slice(observation.Invariants, func(i, j int) bool {
		if observation.Invariants[i].Class != observation.Invariants[j].Class {
			return observation.Invariants[i].Class < observation.Invariants[j].Class
		}
		return observation.Invariants[i].ControlID < observation.Invariants[j].ControlID
	})
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
	if observation.Version != observationEventContract.CurrentVersion() || observation.SubjectAgentID == "" || observation.RunID == "" || observation.GoalClass == "" || observation.Domain == "" || observation.BehaviorKey == "" || observation.Context == "" || observation.CausationRoot == "" || observation.Outcome == "" || observation.PathID == "" || observation.ObservedAt.IsZero() {
		return errors.New("adaptive observation identity, scope, outcome, path, and time are required")
	}
	switch observation.Trust {
	case contracts.TrustUntrustedContent, contracts.TrustObserved, contracts.TrustUserConfirmed:
	default:
		return errors.New("raw observation trust must preserve untrusted, observed, or user-confirmed origin")
	}
	if observation.Trust == contracts.TrustUserConfirmed {
		if observation.ConfirmationAuthorityID == "" || observation.ConfirmationEvidenceRef == "" {
			return errors.New("user-confirmed observation requires authority and confirmation evidence")
		}
	} else if observation.ConfirmationAuthorityID != "" || observation.ConfirmationEvidenceRef != "" {
		return errors.New("non-confirmed observation cannot claim confirmation authority")
	}
	if observation.ReasoningTier != "" && observation.ReasoningTier != "D0" && observation.ReasoningTier != "D1" && observation.ReasoningTier != "D2" {
		return errors.New("unknown core reasoning tier")
	}
	seenMeasures := map[string]bool{}
	for _, measure := range observation.RawMeasures {
		if err := measure.Validate(); err != nil {
			return err
		}
		if measure.Kind != RawMeasure {
			return errors.New("observation can contain only raw measures")
		}
		key := measure.Name + "\x00" + measure.Unit
		if seenMeasures[key] {
			return errors.New("duplicate raw measure name/unit")
		}
		seenMeasures[key] = true
	}
	seenControls := map[string]bool{}
	for _, invariant := range observation.Invariants {
		if (invariant.Class != PolicyInvariant && invariant.Class != SecurityInvariant) || invariant.ControlID == "" || invariant.EvidenceRef == "" {
			return errors.New("invariant result requires known class, control, and evidence")
		}
		key := string(invariant.Class) + "\x00" + invariant.ControlID
		if seenControls[key] {
			return errors.New("duplicate invariant result")
		}
		seenControls[key] = true
	}
	return nil
}

type Measurement struct {
	ID             string  `json:"id"`
	Version        string  `json:"version"`
	SubjectAgentID string  `json:"subject_agent_id"`
	GoalClass      string  `json:"goal_class"`
	Domain         string  `json:"domain"`
	BehaviorKey    string  `json:"behavior_key"`
	Context        string  `json:"context"`
	Measure        Measure `json:"measure"`
}

func FreezeMeasurement(measurement Measurement) (Measurement, error) {
	measurement.Version = measurementEventContract.CurrentVersion()
	measurement.ID = ""
	measurement.Measure.SourceObservationIDs = append([]string(nil), measurement.Measure.SourceObservationIDs...)
	sort.Strings(measurement.Measure.SourceObservationIDs)
	if err := validateMeasurement(measurement, false); err != nil {
		return Measurement{}, err
	}
	digest, err := digestValue(measurement)
	if err != nil {
		return Measurement{}, err
	}
	measurement.ID = "sha256:" + digest
	return measurement, nil
}

func VerifyMeasurement(measurement Measurement) error {
	if err := validateMeasurement(measurement, true); err != nil {
		return err
	}
	id := measurement.ID
	measurement.ID = ""
	digest, err := digestValue(measurement)
	if err != nil {
		return err
	}
	if id != "sha256:"+digest {
		return errors.New("adaptive measurement digest mismatch")
	}
	return nil
}

func validateMeasurement(measurement Measurement, requireID bool) error {
	if requireID && measurement.ID == "" {
		return errors.New("adaptive measurement identity is required")
	}
	if measurement.Version != measurementEventContract.CurrentVersion() || measurement.SubjectAgentID == "" || measurement.GoalClass == "" || measurement.Domain == "" || measurement.BehaviorKey == "" || measurement.Context == "" {
		return errors.New("adaptive measurement scope is required")
	}
	if err := measurement.Measure.Validate(); err != nil {
		return err
	}
	if measurement.Measure.Kind == RawMeasure {
		return errors.New("raw facts belong in observations, not derived measurement records")
	}
	return nil
}

func sameMeasurementScope(measurement Measurement, observation Observation) bool {
	return measurement.SubjectAgentID == observation.SubjectAgentID && measurement.GoalClass == observation.GoalClass && measurement.Domain == observation.Domain && measurement.BehaviorKey == observation.BehaviorKey && measurement.Context == observation.Context
}

func measureLess(left, right Measure) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Unit != right.Unit {
		return left.Unit < right.Unit
	}
	return left.Kind < right.Kind
}

func digestValue(value any) (string, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), nil
}
