package goals

import "errors"

type Applicability string

const (
	ApplicabilityReuse Applicability = "reuse"
	ApplicabilityDelta Applicability = "delta"
	ApplicabilityReplan Applicability = "replan"
)

type ApplicabilityInput struct {
	BaselinePresent bool
	DigestValid bool
	GoalMatches bool
	ValidityPredicatesSatisfied bool
	ChangedArtifacts []string
}

func ClassifyApplicability(in ApplicabilityInput) (Applicability,error) {
	if !in.BaselinePresent { return ApplicabilityReplan,nil }
	if !in.DigestValid { return ApplicabilityReplan,ErrBaselineDigestMismatch }
	if !in.GoalMatches || !in.ValidityPredicatesSatisfied { return ApplicabilityReplan,nil }
	if len(in.ChangedArtifacts)>0 { return ApplicabilityDelta,nil }
	return ApplicabilityReuse,nil
}

func (a Applicability) Validate() error {
	switch a { case ApplicabilityReuse,ApplicabilityDelta,ApplicabilityReplan: return nil; default: return errors.New("invalid applicability") }
}
