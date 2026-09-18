package goals

import (
	"errors"
	"fmt"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
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
	DecisionUnresolved  DecisionStatus = "unresolved"
	DecisionProvisional DecisionStatus = "provisional"
	DecisionResolved    DecisionStatus = "resolved"
	DecisionSuperseded  DecisionStatus = "superseded"
	DecisionInvalidated DecisionStatus = "invalidated"
)

type Decision struct {
	ID             string         `json:"id"`
	Statement      string         `json:"statement"`
	Status         DecisionStatus `json:"status"`
	Recommendation string         `json:"recommendation,omitempty"`
	Rationale      string         `json:"rationale,omitempty"`
	EvidenceRefs   []string       `json:"evidence_refs,omitempty"`
	Reversible     bool           `json:"reversible"`
	Material       bool           `json:"material"`
	HumanRequired  bool           `json:"human_required"`
	AutoAccepted   bool           `json:"auto_accepted"`
}

type ArtifactRef struct {
	ID     string `json:"id"`
	Role   string `json:"role"`
	Digest string `json:"digest"`
}

type GoalBaseline struct {
	ID                 string             `json:"id"`
	Version            string             `json:"version"`
	Digest             string             `json:"digest"`
	PredecessorDigest  string             `json:"predecessor_digest,omitempty"`
	OriginalIntent     string             `json:"original_intent"`
	RefinedOutcome     string             `json:"refined_outcome"`
	Scope              string             `json:"scope,omitempty"`
	NonGoals           []string           `json:"non_goals,omitempty"`
	Constraints        []string           `json:"constraints,omitempty"`
	SuccessCriteria    []string           `json:"success_criteria,omitempty"`
	EvidenceRefs       []string           `json:"evidence_refs,omitempty"`
	Assumptions        []string           `json:"assumptions,omitempty"`
	Decisions          []Decision         `json:"decisions,omitempty"`
	Artifacts          []ArtifactRef      `json:"artifacts,omitempty"`
	PlanRef            string             `json:"plan_ref,omitempty"`
	ValidityPredicates []string           `json:"validity_predicates,omitempty"`
	Rigor              Rigor              `json:"rigor"`
	RecommendationMode RecommendationMode `json:"recommendation_mode"`
	// Import provenance is recorded only when a canonical baseline crosses the
	// explicit evidence-to-authority import boundary.
	ImportSourceRef    string `json:"import_source_ref,omitempty"`
	ImportSourceDigest string `json:"import_source_digest,omitempty"`
	// WorkPlan is optional because a Goal may be newly created or still in
	// planning. When present it is the accepted executable decomposition; it is
	// never inferred from prose, PlanRef, or model output.
	WorkPlan *contracts.WorkPlan `json:"work_plan,omitempty"`
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
	if b.WorkPlan != nil {
		if err := b.WorkPlan.Validate(); err != nil {
			return fmt.Errorf("invalid accepted work plan: %w", err)
		}
	}
	seen := map[string]struct{}{}
	for _, d := range b.Decisions {
		if d.ID == "" || d.Statement == "" {
			return errors.New("decision id and statement are required")
		}
		if _, ok := seen[d.ID]; ok {
			return fmt.Errorf("duplicate decision %q", d.ID)
		}
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
