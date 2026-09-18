package research

import (
	"github.com/convergent-systems-co/praxis/internal/kernel"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// Graph is a non-development proving domain. It reuses Goals for durable
// outcome shaping, then performs evidence-specific research work without
// introducing software-development semantics into the Praxis kernel.
func Graph() kernel.GraphDef {
	goalsGraph:=goals.Graph()
	return kernel.GraphDef{
		ID:"praxis.package.research.default",Version:"0.1.0",EntryNode:"classify",MaxTransitions:40,
		Nodes:[]kernel.NodeDef{
			{ID:"classify",Class:kernel.NodeCondition},
			{ID:"goals",Class:kernel.NodeSubgraph,Subgraph:&kernel.SubgraphRef{GraphID:goalsGraph.ID,GraphVersion:goalsGraph.Version,EntryPointID:"goals"}},
			{ID:"scope",Class:kernel.NodeInference},
			{ID:"discover",Class:kernel.NodeCapability},
			{ID:"evaluate",Class:kernel.NodeInference},
			{ID:"synthesize",Class:kernel.NodeInference},
			{ID:"challenge",Class:kernel.NodeInference},
			{ID:"refine",Class:kernel.NodeInference},
			{ID:"evidence",Class:kernel.NodeCheckpoint},
			{ID:"complete",Class:kernel.NodeTerminal,TerminalState:kernel.RunSucceeded},
			{ID:"failed",Class:kernel.NodeTerminal,TerminalState:kernel.RunFailed},
		},
		Transitions:[]kernel.TransitionDef{
			{From:"classify",Outcome:"baseline",To:"scope"},
			{From:"classify",Outcome:"goals",To:"goals"},
			{From:"classify",Outcome:"direct",To:"scope"},
			{From:"goals",Outcome:"baseline",To:"scope"},
			{From:"goals",Outcome:"blocked",To:"failed"},
			{From:"scope",Outcome:"ready",To:"discover"},
			{From:"scope",Outcome:"blocked",To:"failed"},
			{From:"discover",Outcome:"ready",To:"evaluate"},
			{From:"discover",Outcome:"insufficient",To:"scope"},
			{From:"discover",Outcome:"blocked",To:"failed"},
			{From:"evaluate",Outcome:"ready",To:"synthesize"},
			{From:"evaluate",Outcome:"more_evidence",To:"discover"},
			{From:"synthesize",Outcome:"ready",To:"challenge"},
			{From:"challenge",Outcome:"pass",To:"evidence"},
			{From:"challenge",Outcome:"revise",To:"refine"},
			{From:"refine",Outcome:"ready",To:"challenge"},
			{From:"refine",Outcome:"more_evidence",To:"discover"},
			{From:"evidence",Outcome:"stored",To:"complete"},
			{From:"evidence",Outcome:"ephemeral",To:"complete"},
			{From:"evidence",Outcome:"blocked",To:"failed"},
		},
	}
}
