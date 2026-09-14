package develop

import (
	"errors"
	"fmt"
	"sort"
)

// Applicability is the deterministic decision made before planning a slice.
type Applicability string

const (
	ApplicabilityReuse     Applicability = "reuse"
	ApplicabilityDelta     Applicability = "delta"
	ApplicabilityReplan    Applicability = "replan"
	ApplicabilityArchitect Applicability = "architect"
)

// ArtifactDependency points from an upstream planning artifact to an artifact
// whose conclusions depend on it.
type ArtifactDependency struct {
	Upstream   string
	Downstream string
}

// WorkspaceSnapshot contains only the content identities used by baseline
// validity predicates. It is evidence, not execution authority.
type WorkspaceSnapshot struct {
	GoalBaselineID      string
	GoalBaselineVersion string
	RequirementsDigest  string
	EvidenceDigests     map[string]string
}

type ApplicabilityRequest struct {
	SliceID            string
	GoverningArtifacts []string
	MaterialChange     bool
}

type ApplicabilityResult struct {
	Decision             Applicability
	InvalidatedArtifacts []string
	Reason               string
}

func (r ApplicabilityResult) Validate() error {
	if r.Decision == "" || r.Reason == "" {
		return errors.New("applicability decision and reason are required")
	}
	return nil
}

// EvaluateApplicability compares a current workspace snapshot to an immutable
// baseline and propagates evidence changes through the smallest dependency
// closure. Unrelated artifacts remain reusable.
func EvaluateApplicability(b PlanningBaseline, current WorkspaceSnapshot, req ApplicabilityRequest) (ApplicabilityResult, error) {
	if err := b.Validate(); err != nil {
		return ApplicabilityResult{}, err
	}
	if req.SliceID == "" {
		return ApplicabilityResult{}, errors.New("slice identity is required")
	}
	if current.GoalBaselineID == "" || current.GoalBaselineVersion == "" {
		return ApplicabilityResult{Decision: ApplicabilityArchitect, Reason: "no applicable Goal Baseline identity"}, nil
	}
	if current.GoalBaselineID != b.GoalBaselineID || current.GoalBaselineVersion != b.GoalBaselineVersion {
		return ApplicabilityResult{Decision: ApplicabilityReplan, Reason: "Goal Baseline identity or version changed"}, nil
	}

	invalidated := map[string]bool{}
	knownArtifacts := map[string]bool{}
	for _, artifact := range b.Artifacts {
		knownArtifacts[artifact.ID] = true
		if digest, ok := current.EvidenceDigests[artifact.ID]; !ok && artifact.Digest != "" {
			invalidated[artifact.ID] = true
		} else if ok && artifact.Digest != "" && digest != artifact.Digest {
			invalidated[artifact.ID] = true
		}
	}
	for _, dependency := range b.Dependencies {
		if !knownArtifacts[dependency.Upstream] || !knownArtifacts[dependency.Downstream] {
			return ApplicabilityResult{}, fmt.Errorf("dependency references unknown artifact: %s -> %s", dependency.Upstream, dependency.Downstream)
		}
	}
	changed := true
	for changed {
		changed = false
		for _, edge := range b.Dependencies {
			if invalidated[edge.Upstream] && !invalidated[edge.Downstream] {
				invalidated[edge.Downstream] = true
				changed = true
			}
		}
	}
	ids := make([]string, 0, len(invalidated))
	for id := range invalidated {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if current.RequirementsDigest != "" && b.RequirementsDigest != "" && current.RequirementsDigest != b.RequirementsDigest {
		return ApplicabilityResult{Decision: ApplicabilityReplan, InvalidatedArtifacts: ids, Reason: "requirements changed"}, nil
	}
	if len(ids) == 0 && !req.MaterialChange {
		return ApplicabilityResult{Decision: ApplicabilityReuse, Reason: fmt.Sprintf("baseline applies to slice %s", req.SliceID)}, nil
	}
	intersects := false
	for _, requested := range req.GoverningArtifacts {
		if invalidated[requested] {
			intersects = true
			break
		}
	}
	if intersects || req.MaterialChange {
		return ApplicabilityResult{Decision: ApplicabilityDelta, InvalidatedArtifacts: ids, Reason: "refresh only the affected dependency closure"}, nil
	}
	return ApplicabilityResult{Decision: ApplicabilityReuse, InvalidatedArtifacts: ids, Reason: "changed evidence is outside this slice"}, nil
}
