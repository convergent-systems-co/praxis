package research

import (
	"fmt"

	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

func NewExecutor(delegate kernel.NodeExecutor) (kernel.NodeExecutor,error) {
	if delegate==nil { return nil,fmt.Errorf("research node executor is required") }
	goalsGraph:=goals.Graph()
	registry,err:=kernel.NewGraphRegistry(goalsGraph); if err!=nil { return nil,err }
	return kernel.ComposingExecutor{
		Delegate:delegate,
		Subgraphs:kernel.LocalSubgraphRuntime{Registry:registry,Executor:delegate},
		Outcomes:map[string]kernel.SubgraphOutcomeContract{
			goalsGraph.ID+"@"+goalsGraph.Version:{Succeeded:"baseline",Failed:"blocked",Cancelled:"blocked"},
		},
	},nil
}
