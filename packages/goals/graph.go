package goals

import "github.com/convergent-systems-co/praxis/internal/kernel"

func Graph() kernel.GraphDef {
	return kernel.GraphDef{
		ID: "praxis.package.goals.default", Version: "0.3.0", EntryNode: "intent", MaxTransitions: 48,
		Nodes: []kernel.NodeDef{
			{ID: "intent", Class: kernel.NodeHuman},
			{ID: "frame", Class: kernel.NodeInference},
			{ID: "discover", Class: kernel.NodeCapability},
			{ID: "variance", Class: kernel.NodeInference},
			{ID: "calibrate", Class: kernel.NodeHuman},
			{ID: "decide", Class: kernel.NodeInference},
			{ID: "model", Class: kernel.NodeInference},
			{ID: "specify", Class: kernel.NodeInference},
			{ID: "plan", Class: kernel.NodeInference},
			{ID: "candidate_baseline", Class: kernel.NodeDeterministic},
			{ID: "architecture_review", Class: kernel.NodeDeterministic},
			{ID: "architecture_review_decision", Class: kernel.NodeHuman},
			{ID: "baseline", Class: kernel.NodeCheckpoint},
			{ID: "complete", Class: kernel.NodeTerminal, TerminalState: kernel.RunSucceeded},
			{ID: "failed", Class: kernel.NodeTerminal, TerminalState: kernel.RunFailed},
		},
		Transitions: []kernel.TransitionDef{
			{From: "intent", Outcome: "captured", To: "frame"}, {From: "intent", Outcome: "blocked", To: "failed"},
			{From: "frame", Outcome: "direct", To: "specify"}, {From: "frame", Outcome: "structured", To: "discover"}, {From: "frame", Outcome: "rigorous", To: "discover"},
			{From: "discover", Outcome: "ready", To: "variance"}, {From: "discover", Outcome: "blocked", To: "failed"},
			{From: "variance", Outcome: "calibrate", To: "calibrate"}, {From: "variance", Outcome: "decide", To: "decide"}, {From: "variance", Outcome: "blocked", To: "failed"},
			{From: "calibrate", Outcome: "review_all", To: "decide"}, {From: "calibrate", Outcome: "delegate_clear", To: "decide"}, {From: "calibrate", Outcome: "blocked", To: "failed"},
			{From: "decide", Outcome: "ready", To: "model"}, {From: "decide", Outcome: "needs_human", To: "calibrate"}, {From: "decide", Outcome: "blocked", To: "failed"},
			{From: "model", Outcome: "ready", To: "specify"}, {From: "model", Outcome: "skip", To: "specify"},
			{From: "specify", Outcome: "ready", To: "plan"},
			{From: "plan", Outcome: "ready", To: "candidate_baseline"}, {From: "plan", Outcome: "no_execution", To: "candidate_baseline"},
			{From: "candidate_baseline", Outcome: "digested", To: "architecture_review"}, {From: "candidate_baseline", Outcome: "blocked", To: "failed"},
			{From: "architecture_review", Outcome: "ready", To: "baseline"}, {From: "architecture_review", Outcome: "review_required", To: "architecture_review_decision"}, {From: "architecture_review", Outcome: "blocked", To: "failed"},
			{From: "architecture_review_decision", Outcome: "resolved", To: "baseline"}, {From: "architecture_review_decision", Outcome: "blocked", To: "failed"},
			{From: "baseline", Outcome: "stored", To: "complete"}, {From: "baseline", Outcome: "ephemeral", To: "complete"}, {From: "baseline", Outcome: "blocked", To: "failed"},
		},
	}
}
