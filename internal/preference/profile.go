package preference

import (
	"errors"
	"time"
)

type EvidenceClass string

const (
	ProfileDeclared  EvidenceClass = "declared"
	ProfileInherited EvidenceClass = "inherited"
	ProfileObserved  EvidenceClass = "observed"
	ProfileMeasured  EvidenceClass = "measured"
	ProfileConfirmed EvidenceClass = "confirmed"
)

type Dimension struct {
	Name        string
	Value       float64
	Evidence    EvidenceClass
	Confidence  float64
	Provenance  string
	WindowStart time.Time
	WindowEnd   time.Time
	UpdatedAt   time.Time
}

func (d Dimension) Validate() error {
	if d.Name == "" || d.Provenance == "" || d.UpdatedAt.IsZero() {
		return errors.New("profile dimension name, provenance, and updated time are required")
	}
	if d.Value < 0 || d.Value > 1 || d.Confidence < 0 || d.Confidence > 1 {
		return errors.New("profile value/confidence must be in [0,1]")
	}
	switch d.Evidence {
	case ProfileDeclared, ProfileInherited, ProfileObserved, ProfileMeasured, ProfileConfirmed:
		return nil
	default:
		return errors.New("unknown behavioral profile evidence class")
	}
}

type Profile struct {
	SubjectID  string
	Dimensions map[string]Dimension
}

// Distance computes simple weighted absolute distance after hard eligibility
// has already been evaluated elsewhere. It is advisory ranking only.
func Distance(candidate Profile, desired map[string]float64) (float64, error) {
	var distance float64
	for name, want := range desired {
		if want < 0 || want > 1 {
			return 0, errors.New("desired profile value must be in [0,1]")
		}
		dim, ok := candidate.Dimensions[name]
		if !ok {
			distance += 1
			continue
		}
		if err := dim.Validate(); err != nil {
			return 0, err
		}
		delta := dim.Value - want
		if delta < 0 { delta = -delta }
		distance += delta
	}
	return distance, nil
}
