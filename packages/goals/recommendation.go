package goals

import "errors"

type RecommendationDecision string

const (
	RecommendationAutoResolve RecommendationDecision = "auto_resolve"
	RecommendationAskUser    RecommendationDecision = "ask_user"
	RecommendationLeaveOpen  RecommendationDecision = "leave_open"
)

type RecommendationInput struct {
	Mode                  RecommendationMode
	HasDominantRecommendation bool
	HasEvidence           bool
	HumanRequired         bool
	ExecutionAuthorization bool
	ExplicitPreferenceNeeded bool
	Material              bool
	Reversible            bool
	ConfidenceSufficient  bool
}

// DecideRecommendationInterruption determines whether a recommendation may be
// resolved without interrupting the user. It never grants execution authority.
func DecideRecommendationInterruption(in RecommendationInput) (RecommendationDecision,error) {
	if in.ExecutionAuthorization || in.HumanRequired || in.ExplicitPreferenceNeeded {
		return RecommendationAskUser,nil
	}
	if !in.HasDominantRecommendation || !in.ConfidenceSufficient {
		if in.Material && !in.Reversible { return RecommendationAskUser,nil }
		return RecommendationLeaveOpen,nil
	}
	if !in.HasEvidence { return RecommendationLeaveOpen,nil }
	if in.Mode != RecommendationDelegated { return RecommendationAskUser,nil }
	if in.Material && !in.Reversible { return RecommendationAskUser,nil }
	return RecommendationAutoResolve,nil
}

func (d RecommendationDecision) Validate() error {
	switch d { case RecommendationAutoResolve,RecommendationAskUser,RecommendationLeaveOpen: return nil; default: return errors.New("invalid recommendation decision") }
}
