package adaptation

import (
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
	fact.Version = "v2"
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
	if fact.Version != "v2" || fact.SubjectAgentID == "" || fact.Dimension == "" || fact.Provenance == "" || fact.Context == "" || fact.RecordedAt.IsZero() {
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
