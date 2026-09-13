package develop

import "github.com/convergent-systems-co/praxis/internal/inference"

type WorkSignals struct {
	FilesLikelyAffected   int
	KnownAcceptanceTests  bool
	ArchitectureChange    bool
	SecuritySensitive     bool
	AmbiguousGoal         bool
	NovelDomain           bool
	DeterministicFixKnown bool
	CurrentGoalBaseline   bool
	BaselineApplicable    bool
}

type Classification struct {
	Outcome string
	Tier    inference.Tier
	Reason  string
}

// ClassifyWork chooses the lowest-cost reliable path. Architecturally material
// work is routed through the reusable Goals baseline rather than re-planned
// independently by each implementation slice.
func ClassifyWork(s WorkSignals) Classification {
	material := s.ArchitectureChange || s.SecuritySensitive || s.AmbiguousGoal || s.NovelDomain
	if material {
		if s.CurrentGoalBaseline && s.BaselineApplicable {
			return Classification{Outcome: "plan", Tier: inference.D1, Reason: "applicable Goal Baseline exists; perform only development-local delta planning"}
		}
		return Classification{Outcome: "goals", Tier: inference.D2, Reason: "material ambiguity, novelty, architecture, or security risk requires reusable Goal Baseline"}
	}
	if s.DeterministicFixKnown && s.KnownAcceptanceTests && s.FilesLikelyAffected <= 3 {
		return Classification{Outcome: "fast", Tier: inference.D0, Reason: "deterministic narrow change with known validation"}
	}
	if s.KnownAcceptanceTests && s.FilesLikelyAffected <= 8 {
		return Classification{Outcome: "fast", Tier: inference.D1, Reason: "bounded change with objective validation"}
	}
	return Classification{Outcome: "plan", Tier: inference.D1, Reason: "bounded development-local planning needed before mutation"}
}
