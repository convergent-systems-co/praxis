package adaptation

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

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

type Score struct {
	Value float64      `json:"value"`
	Range NumericRange `json:"range"`
}

func (score Score) Validate() error {
	if math.IsNaN(score.Value) || math.IsInf(score.Value, 0) || score.Range.Maximum <= score.Range.Minimum || score.Value < score.Range.Minimum || score.Value > score.Range.Maximum {
		return errors.New("normalized profile score requires a finite value and explicit containing range")
	}
	return nil
}

type ProfileFact struct {
	ID                      string        `json:"id"`
	Version                 string        `json:"version"`
	SubjectAgentID          string        `json:"subject_agent_id"`
	Dimension               string        `json:"dimension"`
	Score                   Score         `json:"score"`
	EvidenceClass           EvidenceClass `json:"evidence_class"`
	Confidence              Score         `json:"confidence"`
	Provenance              string        `json:"provenance"`
	ConfirmationAuthorityID string        `json:"confirmation_authority_id,omitempty"`
	ConfirmationEvidenceRef string        `json:"confirmation_evidence_ref,omitempty"`
	Context                 string        `json:"context"`
	SourceObservationIDs    []string      `json:"source_observation_ids,omitempty"`
	SourceMeasurementIDs    []string      `json:"source_measurement_ids,omitempty"`
	RecordedAt              time.Time     `json:"recorded_at"`
}

func FreezeProfileFact(fact ProfileFact) (ProfileFact, error) {
	fact.Version = profileFactEventContract.CurrentVersion()
	fact.ID = ""
	fact.SourceObservationIDs = append([]string(nil), fact.SourceObservationIDs...)
	fact.SourceMeasurementIDs = append([]string(nil), fact.SourceMeasurementIDs...)
	sort.Strings(fact.SourceObservationIDs)
	sort.Strings(fact.SourceMeasurementIDs)
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
	if fact.Version != profileFactEventContract.CurrentVersion() || fact.SubjectAgentID == "" || fact.Dimension == "" || fact.Provenance == "" || fact.Context == "" || fact.RecordedAt.IsZero() {
		return errors.New("behavioral profile fact scope, provenance, and time are required")
	}
	if err := fact.Score.Validate(); err != nil {
		return err
	}
	if err := fact.Confidence.Validate(); err != nil {
		return err
	}
	switch fact.EvidenceClass {
	case Declared, Inherited, Confirmed:
		if len(fact.SourceObservationIDs) != 0 || len(fact.SourceMeasurementIDs) != 0 {
			return errors.New("declared, inherited, and confirmed facts cannot masquerade as execution evidence")
		}
		if fact.EvidenceClass == Confirmed && (fact.ConfirmationAuthorityID == "" || fact.ConfirmationEvidenceRef == "") {
			return errors.New("confirmed profile fact requires authority and confirmation evidence")
		}
		if fact.EvidenceClass != Confirmed && (fact.ConfirmationAuthorityID != "" || fact.ConfirmationEvidenceRef != "") {
			return errors.New("non-confirmed profile fact cannot claim confirmation authority")
		}
	case Observed:
		if fact.ConfirmationAuthorityID != "" || fact.ConfirmationEvidenceRef != "" {
			return errors.New("observed profile fact cannot claim confirmation authority")
		}
		if len(fact.SourceObservationIDs) == 0 || len(fact.SourceMeasurementIDs) != 0 {
			return errors.New("observed profile fact requires only source observations")
		}
	case Measured:
		if fact.ConfirmationAuthorityID != "" || fact.ConfirmationEvidenceRef != "" {
			return errors.New("measured profile fact cannot claim confirmation authority")
		}
		if len(fact.SourceMeasurementIDs) == 0 || len(fact.SourceObservationIDs) != 0 {
			return errors.New("measured profile fact requires only source measurements")
		}
	default:
		return errors.New("unknown behavioral profile evidence class")
	}
	return nil
}

func ProfileFactFromNormalizedMeasurement(measurement Measurement, dimension string, confidence Score, provenance string, recordedAt time.Time) (ProfileFact, error) {
	if err := VerifyMeasurement(measurement); err != nil {
		return ProfileFact{}, err
	}
	if measurement.Measure.Kind != NormalizedScore || measurement.Measure.NormalizedRange == nil {
		return ProfileFact{}, errors.New("measured profile fact requires an explicit normalized-score measurement")
	}
	return FreezeProfileFact(ProfileFact{SubjectAgentID: measurement.SubjectAgentID, Dimension: dimension, Score: Score{Value: measurement.Measure.Value, Range: *measurement.Measure.NormalizedRange}, EvidenceClass: Measured, Confidence: confidence, Provenance: provenance, Context: measurement.Context, SourceMeasurementIDs: []string{measurement.ID}, RecordedAt: recordedAt})
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

type ProfileObservationEvaluator interface {
	ID() string
	Version() string
	EvaluateProfile(context.Context, []Observation) (Score, Score, error)
}

type ProfileDerivation struct {
	ID                   string       `json:"id"`
	Version              string       `json:"version"`
	FactID               string       `json:"fact_id"`
	Evaluator            EvaluatorRef `json:"evaluator"`
	TransformID          string       `json:"transform_id"`
	SourceObservationIDs []string     `json:"source_observation_ids"`
	DerivedAt            time.Time    `json:"derived_at"`
}

func DeriveObservedProfile(ctx context.Context, observations []Observation, dimension, transformID string, evaluator ProfileObservationEvaluator, at time.Time) (ProfileFact, ProfileDerivation, error) {
	if len(observations) == 0 || dimension == "" || transformID == "" || evaluator == nil || evaluator.ID() == "" || evaluator.Version() == "" || at.IsZero() {
		return ProfileFact{}, ProfileDerivation{}, errors.New("observed profile derivation requires observations, dimension, transform, evaluator, and time")
	}
	subject, contextID := observations[0].SubjectAgentID, observations[0].Context
	sourceIDs, seen := []string{}, map[string]bool{}
	for _, observation := range observations {
		if err := VerifyObservation(observation); err != nil {
			return ProfileFact{}, ProfileDerivation{}, err
		}
		if observation.SubjectAgentID != subject || observation.Context != contextID {
			return ProfileFact{}, ProfileDerivation{}, errors.New("profile derivation observations must share subject and context")
		}
		if seen[observation.ID] {
			return ProfileFact{}, ProfileDerivation{}, errors.New("duplicate profile derivation observation")
		}
		seen[observation.ID] = true
		sourceIDs = append(sourceIDs, observation.ID)
	}
	score, confidence, err := evaluator.EvaluateProfile(ctx, append([]Observation(nil), observations...))
	if err != nil {
		return ProfileFact{}, ProfileDerivation{}, err
	}
	fact, err := FreezeProfileFact(ProfileFact{SubjectAgentID: subject, Dimension: dimension, Score: score, EvidenceClass: Observed, Confidence: confidence, Provenance: "evaluator:" + evaluator.ID() + "@" + evaluator.Version(), Context: contextID, SourceObservationIDs: sourceIDs, RecordedAt: at.UTC()})
	if err != nil {
		return ProfileFact{}, ProfileDerivation{}, err
	}
	derivation := ProfileDerivation{Version: profileDerivationEventContract.CurrentVersion(), FactID: fact.ID, Evaluator: EvaluatorRef{ID: evaluator.ID(), Version: evaluator.Version()}, TransformID: transformID, SourceObservationIDs: append([]string(nil), fact.SourceObservationIDs...), DerivedAt: at.UTC()}
	derivation.ID = profileDigest("profile-derivation", derivation)
	return fact, derivation, nil
}

func VerifyProfileDerivation(derivation ProfileDerivation) error {
	if derivation.ID == "" || derivation.Version != profileDerivationEventContract.CurrentVersion() || derivation.FactID == "" || derivation.Evaluator.ID == "" || derivation.Evaluator.Version == "" || derivation.TransformID == "" || len(derivation.SourceObservationIDs) == 0 || derivation.DerivedAt.IsZero() {
		return errors.New("profile derivation identity, evaluator, transform, sources, and time are required")
	}
	id := derivation.ID
	derivation.ID = ""
	if profileDigest("profile-derivation", derivation) != id {
		return errors.New("profile derivation digest mismatch")
	}
	return nil
}

func VerifyProfileDivergence(report ProfileDivergence) error {
	if report.ID == "" || report.Version != profileDivergenceEventContract.CurrentVersion() || report.SubjectAgentID == "" || report.ReferenceFactID == "" || report.CurrentFactID == "" || report.EvaluatedAt.IsZero() || math.IsNaN(report.AbsoluteDelta) || math.IsInf(report.AbsoluteDelta, 0) || report.AbsoluteDelta < 0 {
		return errors.New("profile divergence identity, facts, delta, and time are required")
	}
	id := report.ID
	report.ID = ""
	if profileDigest("profile-divergence", report) != id {
		return errors.New("profile divergence digest mismatch")
	}
	return nil
}

type ProfileDivergencePolicy struct {
	ID               string          `json:"id"`
	Version          string          `json:"version"`
	PackageID        string          `json:"package_id"`
	Dimension        string          `json:"dimension"`
	Context          string          `json:"context"`
	ReferenceClasses []EvidenceClass `json:"reference_classes"`
	CurrentClasses   []EvidenceClass `json:"current_classes"`
	MinimumDelta     float64         `json:"minimum_delta"`
}

type ProfileDivergence struct {
	ID              string                  `json:"id"`
	Version         string                  `json:"version"`
	Policy          ProfileDivergencePolicy `json:"policy"`
	SubjectAgentID  string                  `json:"subject_agent_id"`
	ReferenceFactID string                  `json:"reference_fact_id"`
	CurrentFactID   string                  `json:"current_fact_id"`
	AbsoluteDelta   float64                 `json:"absolute_delta"`
	Diverged        bool                    `json:"diverged"`
	EvaluatedAt     time.Time               `json:"evaluated_at"`
}

func FreezeProfileDivergencePolicy(policy ProfileDivergencePolicy) (ProfileDivergencePolicy, error) {
	policy.Version, policy.ID = profileDivergenceEventContract.CurrentVersion(), ""
	policy.ReferenceClasses = canonicalEvidenceClasses(policy.ReferenceClasses)
	policy.CurrentClasses = canonicalEvidenceClasses(policy.CurrentClasses)
	if policy.PackageID == "" || policy.Dimension == "" || policy.Context == "" || len(policy.ReferenceClasses) == 0 || len(policy.CurrentClasses) == 0 || math.IsNaN(policy.MinimumDelta) || math.IsInf(policy.MinimumDelta, 0) || policy.MinimumDelta < 0 {
		return ProfileDivergencePolicy{}, errors.New("profile divergence policy requires package, scope, evidence classes, and non-negative threshold")
	}
	policy.ID = profileDigest("profile-divergence-policy", policy)
	return policy, nil
}

func EvaluateProfileDivergence(history []ProfileFact, policy ProfileDivergencePolicy, at time.Time) (ProfileDivergence, error) {
	id := policy.ID
	verified, err := FreezeProfileDivergencePolicy(policy)
	if err != nil || verified.ID != id {
		return ProfileDivergence{}, errors.New("profile divergence policy digest mismatch")
	}
	if at.IsZero() {
		return ProfileDivergence{}, errors.New("profile divergence evaluation time is required")
	}
	referenceAllowed, currentAllowed := evidenceClassSet(policy.ReferenceClasses), evidenceClassSet(policy.CurrentClasses)
	var reference, current ProfileFact
	for _, fact := range history {
		if err := VerifyProfileFact(fact); err != nil {
			return ProfileDivergence{}, err
		}
		if fact.Dimension != policy.Dimension || fact.Context != policy.Context || fact.RecordedAt.After(at) {
			continue
		}
		if referenceAllowed[fact.EvidenceClass] && (reference.ID == "" || fact.RecordedAt.After(reference.RecordedAt)) {
			reference = fact
		}
		if currentAllowed[fact.EvidenceClass] && (current.ID == "" || fact.RecordedAt.After(current.RecordedAt)) {
			current = fact
		}
	}
	if reference.ID == "" || current.ID == "" || reference.SubjectAgentID != current.SubjectAgentID || reference.Score.Range != current.Score.Range {
		return ProfileDivergence{}, errors.New("comparable reference and current profile facts are required")
	}
	delta := math.Abs(current.Score.Value - reference.Score.Value)
	report := ProfileDivergence{Version: profileDivergenceEventContract.CurrentVersion(), Policy: policy, SubjectAgentID: reference.SubjectAgentID, ReferenceFactID: reference.ID, CurrentFactID: current.ID, AbsoluteDelta: delta, Diverged: delta >= policy.MinimumDelta, EvaluatedAt: at.UTC()}
	report.ID = profileDigest("profile-divergence", report)
	return report, nil
}

func canonicalEvidenceClasses(values []EvidenceClass) []EvidenceClass {
	out := append([]EvidenceClass(nil), values...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	seen := map[EvidenceClass]bool{}
	for _, value := range out {
		if value != Declared && value != Inherited && value != Observed && value != Measured && value != Confirmed {
			return nil
		}
		if seen[value] {
			return nil
		}
		seen[value] = true
	}
	return out
}
func evidenceClassSet(values []EvidenceClass) map[EvidenceClass]bool {
	out := map[EvidenceClass]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}
func profileDigest(prefix string, value any) string {
	digest, _ := digestValue(struct {
		Prefix string `json:"prefix"`
		Value  any    `json:"value"`
	}{prefix, value})
	return "sha256:" + digest
}
