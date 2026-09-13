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
}

type Classification struct {
	Outcome string
	Tier    inference.Tier
	Reason  string
}

// ClassifyWork deliberately biases toward action when the task is narrow,
// testable, and low-risk. Open reasoning is reserved for actual ambiguity/risk.
func ClassifyWork(s WorkSignals) Classification {
	if s.ArchitectureChange || s.SecuritySensitive || s.AmbiguousGoal || s.NovelDomain {
		return Classification{Outcome: "plan", Tier: inference.D2, Reason: "material ambiguity, novelty, architecture, or security risk"}
	}
	if s.DeterministicFixKnown && s.KnownAcceptanceTests && s.FilesLikelyAffected <= 3 {
		return Classification{Outcome: "fast", Tier: inference.D0, Reason: "deterministic narrow change with known validation"}
	}
	if s.KnownAcceptanceTests && s.FilesLikelyAffected <= 8 {
		return Classification{Outcome: "fast", Tier: inference.D1, Reason: "bounded change with objective validation"}
	}
	return Classification{Outcome: "plan", Tier: inference.D1, Reason: "bounded planning needed before mutation"}
}
