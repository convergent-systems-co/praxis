package develop

import (
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// NewExecutor composes the software-development graph with the exact Goals
// graph/version. The caller supplies the ordinary node executor; subgraph
// execution remains deterministic and graph-identity bound.
func NewExecutor(delegate kernel.NodeExecutor) (kernel.NodeExecutor, error) {
	if delegate == nil {
		return nil, fmt.Errorf("develop node executor is required")
	}
	goalsGraph := goals.Graph()
	registry, err := kernel.NewGraphRegistry(goalsGraph)
	if err != nil {
		return nil, err
	}
	local := kernel.LocalSubgraphRuntime{Registry: registry, Executor: delegate}
	return kernel.ComposingExecutor{
		Delegate:  delegate,
		Subgraphs: local,
		Outcomes: map[string]kernel.SubgraphOutcomeContract{
			goalsGraph.ID + "@" + goalsGraph.Version: {
				Succeeded: "baseline",
				Failed:    "blocked",
				Cancelled: "blocked",
			},
		},
	}, nil
}
