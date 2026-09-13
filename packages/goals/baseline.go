package goals

import (
	"errors"
	"fmt"
)

type Rigor string

const (
	RigorDirect     Rigor = "direct"
	RigorStructured Rigor = "structured"
	RigorRigorous   Rigor = "rigorous"
)

type RecommendationMode string

const (
	RecommendationReviewAll RecommendationMode = "review_all"
	RecommendationDelegated RecommendationMode = "delegated_clear_recommendations"
)

type DecisionStatus string

const (
	DecisionUnresolved DecisionStatus = "unresolved"
	DecisionProvisional DecisionStatus = "provisional"
	DecisionResolved DecisionStatus = "resolved"
	DecisionSuperseded DecisionStatus = "superseded"
	DecisionInvalidated DecisionStatus = "invalidated"
)

type Decision struct {
	ID             string
	Statement      string
	Status         DecisionStatus
	Recommendation string
	Rationale      string
	EvidenceRefs   []string
	Reversible     bool
	Material       bool
	HumanRequired  bool
	AutoAccepted   bool
}

type ArtifactRef struct {
	ID     string
	Role   string
	Digest string
}

type GoalBaseline struct {
	ID                 string
	Version            string
	Digest             string
	OriginalIntent     string
	RefinedOutcome     string
	Scope              string
	NonGoals           []string
	Constraints        []string
	SuccessCriteria    []string
	EvidenceRefs       []string
	Assumptions        []string
	Decisions          []Decision
	Artifacts          []ArtifactRef
	PlanRef            string
	ValidityPredicates []string
	Rigor              Rigor
	RecommendationMode RecommendationMode
}

func (b GoalBaseline) Validate() error {
	if b.ID == "" || b.Version == "" || b.OriginalIntent == "" || b.RefinedOutcome == "" {
		return errors.New("baseline id, version, original intent, and refined outcome are required")
	}
	switch b.Rigor {
	case RigorDirect, RigorStructured, RigorRigorous:
	default:
		return fmt.Errorf("invalid rigor %q", b.Rigor)
	}
	switch b.RecommendationMode {
	case RecommendationReviewAll, RecommendationDelegated:
	default:
		return fmt.Errorf("invalid recommendation mode %q", b.RecommendationMode)
	}
	seen := map[string]struct{}{}
	for _, d := range b.Decisions {
		if d.ID == "" || d.Statement == "" { return errors.New("decision id and statement are required") }
		if _, ok := seen[d.ID]; ok { return fmt.Errorf("duplicate decision %q", d.ID) }
		seen[d.ID] = struct{}{}
		if d.AutoAccepted && b.RecommendationMode != RecommendationDelegated {
			return fmt.Errorf("decision %q cannot be auto-accepted outside delegated recommendation mode", d.ID)
		}
		if d.AutoAccepted && (d.HumanRequired || (d.Material && !d.Reversible && d.Recommendation == "")) {
			return fmt.Errorf("decision %q requires human interaction and cannot be auto-accepted", d.ID)
		}
	}
	return nil
}
