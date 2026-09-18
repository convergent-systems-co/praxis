package kernel

import "fmt"

// ResolveTransition selects the unique destination for a node outcome.
func ResolveTransition(g GraphDef, from, outcome string) (string, error) {
	if err := g.Validate(); err != nil {
		return "", fmt.Errorf("invalid graph: %w", err)
	}
	for _, tr := range g.Transitions {
		if tr.From == from && tr.Outcome == outcome {
			return tr.To, nil
		}
	}
	return "", fmt.Errorf("no transition from %q for outcome %q", from, outcome)
}
