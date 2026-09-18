package learning

import (
	"errors"
	"sort"
)

type Evidence struct {
	ID            string
	SourceID      string
	CausationRoot string
	TrustClass    string
	Outcome       string
}

// IndependentRoots returns unique causal roots. Multiple summaries, model
// restatements, or retrievals of the same underlying source count once.
func IndependentRoots(evidence []Evidence) ([]string, error) {
	roots := map[string]struct{}{}
	for _, e := range evidence {
		if e.ID == "" || e.SourceID == "" || e.CausationRoot == "" {
			return nil, errors.New("evidence id, source id, and causation root are required")
		}
		roots[e.CausationRoot] = struct{}{}
	}
	out := make([]string, 0, len(roots))
	for root := range roots {
		out = append(out, root)
	}
	sort.Strings(out)
	return out, nil
}

func MeetsIndependentEvidenceThreshold(evidence []Evidence, minimum int) (bool, error) {
	if minimum <= 0 {
		return false, errors.New("minimum independent evidence threshold must be positive")
	}
	roots, err := IndependentRoots(evidence)
	if err != nil {
		return false, err
	}
	return len(roots) >= minimum, nil
}
