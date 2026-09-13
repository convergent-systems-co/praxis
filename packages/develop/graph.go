package develop

import "github.com/convergent-systems-co/praxis/internal/kernel"

// Graph returns the initial domain graph for the software-development proving
// package. Development terminology stays here rather than entering Praxis core.
func Graph() kernel.GraphDef {
	return kernel.GraphDef{
		ID:             "praxis.package.develop.default",
		Version:        "0.1.0",
		EntryNode:      "discover",
		MaxTransitions: 48,
		Nodes: []kernel.NodeDef{
			{ID: "discover", Class: kernel.NodeCapability},
			{ID: "classify", Class: kernel.NodeCondition},
			{ID: "plan", Class: kernel.NodeInference},
			{ID: "prepare", Class: kernel.NodeCapability},
			{ID: "implement", Class: kernel.NodeInference},
			{ID: "validate", Class: kernel.NodeCapability},
			{ID: "review", Class: kernel.NodeInference},
			{ID: "repair", Class: kernel.NodeInference},
			{ID: "integrate", Class: kernel.NodeCapability},
			{ID: "complete", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded},
			{ID: "failed", Class: kernel.NodeTerminal, TerminalState: kernel.RunFailed},
		},
		Transitions: []kernel.TransitionDef{
			{From: "discover", Outcome: "ready", To: "classify"},
			{From: "discover", Outcome: "blocked", To: "failed"},
			{From: "classify", Outcome: "fast", To: "prepare"},
			{From: "classify", Outcome: "plan", To: "plan"},
			{From: "plan", Outcome: "ready", To: "prepare"},
			{From: "plan", Outcome: "blocked", To: "failed"},
			{From: "prepare", Outcome: "ready", To: "implement"},
			{From: "prepare", Outcome: "blocked", To: "failed"},
			{From: "implement", Outcome: "done", To: "validate"},
			{From: "implement", Outcome: "blocked", To: "failed"},
			{From: "validate", Outcome: "pass", To: "review"},
			{From: "validate", Outcome: "repair", To: "repair"},
			{From: "validate", Outcome: "blocked", To: "failed"},
			{From: "review", Outcome: "pass", To: "integrate"},
			{From: "review", Outcome: "repair", To: "repair"},
			{From: "review", Outcome: "blocked", To: "failed"},
			{From: "repair", Outcome: "done", To: "validate"},
			{From: "repair", Outcome: "blocked", To: "failed"},
			{From: "integrate", Outcome: "done", To: "complete"},
			{From: "integrate", Outcome: "handoff", To: "complete"},
			{From: "integrate", Outcome: "blocked", To: "failed"},
		},
	}
}
