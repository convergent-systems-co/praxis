package develop

import (
	"errors"
	"fmt"
	"sort"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

type DevelopmentArtifactRole string

const (
	RoleADR              DevelopmentArtifactRole = "adr"
	RoleArchitectureView DevelopmentArtifactRole = "architecture_view"
	RoleSequenceView     DevelopmentArtifactRole = "sequence_view"
	RoleSecurityView     DevelopmentArtifactRole = "security_view"
	RoleSpecification    DevelopmentArtifactRole = "specification"
	RoleMasterPlan       DevelopmentArtifactRole = "master_plan"
)

type DevelopmentArtifact struct {
	ID         string
	Role       DevelopmentArtifactRole
	SourceGoal string
	Digest     string
}

type PlanningBaseline struct {
	GoalBaselineID      string
	GoalBaselineVersion string
	GoalBaselineDigest  string
	RequirementsDigest  string
	Artifacts           []DevelopmentArtifact
	Dependencies        []ArtifactDependency
	PlanRef             string
}

func (b PlanningBaseline) Validate() error {
	if b.GoalBaselineID == "" || b.GoalBaselineVersion == "" || b.GoalBaselineDigest == "" {
		return errors.New("exact Goal Baseline identity/version/digest is required")
	}
	seen := map[string]struct{}{}
	for _, a := range b.Artifacts {
		if a.ID == "" || a.Role == "" {
			return errors.New("development artifact id and role are required")
		}
		if _, ok := seen[a.ID]; ok {
			return fmt.Errorf("duplicate development artifact %q", a.ID)
		}
		seen[a.ID] = struct{}{}
	}
	knownDependencies := map[string]struct{}{}
	for _, dependency := range b.Dependencies {
		if dependency.Upstream == "" || dependency.Downstream == "" {
			return errors.New("planning artifact dependency endpoints are required")
		}
		key := dependency.Upstream + "\x00" + dependency.Downstream
		if _, ok := knownDependencies[key]; ok {
			return fmt.Errorf("duplicate planning artifact dependency %q -> %q", dependency.Upstream, dependency.Downstream)
		}
		knownDependencies[key] = struct{}{}
	}
	return nil
}

// MaterializeBaseline translates domain-neutral Goal artifacts into software-
// specific planning roles without changing the Goal Baseline itself.
func MaterializeBaseline(goal goals.GoalBaseline, artifacts []DevelopmentArtifact, planRef string) (PlanningBaseline, error) {
	if err := goal.Validate(); err != nil {
		return PlanningBaseline{}, fmt.Errorf("goal baseline: %w", err)
	}
	digest, err := goal.ComputeDigest()
	if err != nil {
		return PlanningBaseline{}, err
	}
	if goal.Digest != "" && goal.Digest != digest {
		return PlanningBaseline{}, goals.ErrBaselineDigestMismatch
	}
	out := PlanningBaseline{GoalBaselineID: goal.ID, GoalBaselineVersion: goal.Version, GoalBaselineDigest: digest, Artifacts: append([]DevelopmentArtifact(nil), artifacts...), PlanRef: planRef}
	sort.Slice(out.Artifacts, func(i, j int) bool { return out.Artifacts[i].ID < out.Artifacts[j].ID })
	if err := out.Validate(); err != nil {
		return PlanningBaseline{}, err
	}
	return out, nil
}

type SliceContract struct {
	ID                 string
	PlanningBaselineID string
	GoalBaselineDigest string
	GoverningArtifacts []string
	Scope              string
	NonGoals           []string
	AcceptanceEvidence []string
	UnresolvedVariance []string
}

func (s SliceContract) Validate(b PlanningBaseline) error {
	if err := b.Validate(); err != nil {
		return err
	}
	if s.ID == "" || s.Scope == "" || s.PlanningBaselineID == "" || s.GoalBaselineDigest == "" {
		return errors.New("slice identity, scope, planning baseline id, and goal digest are required")
	}
	if s.PlanningBaselineID != b.GoalBaselineID || s.GoalBaselineDigest != b.GoalBaselineDigest {
		return errors.New("slice is not bound to the supplied planning baseline")
	}
	known := map[string]struct{}{}
	for _, a := range b.Artifacts {
		known[a.ID] = struct{}{}
	}
	for _, id := range s.GoverningArtifacts {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("slice references unknown governing artifact %q", id)
		}
	}
	return nil
}
