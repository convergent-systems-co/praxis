package kernel

import (
	"errors"
	"fmt"
)

type NodeClass string

const (
	NodeDeterministic NodeClass = "deterministic"
	NodeCapability    NodeClass = "capability"
	NodeInference     NodeClass = "inference"
	NodeHuman         NodeClass = "human"
	NodeCondition     NodeClass = "condition"
	NodeJoin          NodeClass = "join"
	NodeSubgraph      NodeClass = "subgraph"
	NodeCheckpoint    NodeClass = "checkpoint"
	NodeTerminal      NodeClass = "terminal"
)

type NodeDef struct {
	ID            string
	Class         NodeClass
	TerminalState RunState
}

type TransitionDef struct {
	From    string
	Outcome string
	To      string
}

type GraphDef struct {
	ID             string
	Version        string
	EntryNode      string
	Nodes          []NodeDef
	Transitions    []TransitionDef
	MaxTransitions int
}

func (g GraphDef) Validate() error {
	if g.ID == "" || g.Version == "" || g.EntryNode == "" {
		return errors.New("graph id, version, and entry node are required")
	}
	nodes := make(map[string]NodeDef, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.ID == "" {
			return errors.New("node id is required")
		}
		if _, exists := nodes[n.ID]; exists {
			return fmt.Errorf("duplicate node %q", n.ID)
		}
		if !validNodeClass(n.Class) {
			return fmt.Errorf("unknown node class %q", n.Class)
		}
		if n.Class == NodeTerminal {
			if n.TerminalState != RunSucceeded && n.TerminalState != RunFailed && n.TerminalState != RunCancelled {
				return fmt.Errorf("terminal node %q requires explicit terminal state", n.ID)
			}
		} else if n.TerminalState != "" {
			return fmt.Errorf("non-terminal node %q cannot declare terminal state", n.ID)
		}
		nodes[n.ID] = n
	}
	if _, ok := nodes[g.EntryNode]; !ok {
		return fmt.Errorf("entry node %q does not exist", g.EntryNode)
	}

	outcomes := map[string]map[string]struct{}{}
	adj := map[string][]string{}
	for _, tr := range g.Transitions {
		if tr.From == "" || tr.Outcome == "" || tr.To == "" {
			return errors.New("transition from, outcome, and to are required")
		}
		from, ok := nodes[tr.From]
		if !ok {
			return fmt.Errorf("transition source %q does not exist", tr.From)
		}
		if from.Class == NodeTerminal {
			return fmt.Errorf("terminal node %q cannot have outgoing transitions", tr.From)
		}
		if _, ok := nodes[tr.To]; !ok {
			return fmt.Errorf("transition destination %q does not exist", tr.To)
		}
		if outcomes[tr.From] == nil {
			outcomes[tr.From] = map[string]struct{}{}
		}
		if _, dup := outcomes[tr.From][tr.Outcome]; dup {
			return fmt.Errorf("duplicate outcome %q from node %q", tr.Outcome, tr.From)
		}
		outcomes[tr.From][tr.Outcome] = struct{}{}
		adj[tr.From] = append(adj[tr.From], tr.To)
	}

	if hasCycle(g.EntryNode, adj) && g.MaxTransitions <= 0 {
		return errors.New("graph contains an execution cycle but no max transition bound")
	}
	return nil
}

func validNodeClass(c NodeClass) bool {
	switch c {
	case NodeDeterministic, NodeCapability, NodeInference, NodeHuman, NodeCondition, NodeJoin, NodeSubgraph, NodeCheckpoint, NodeTerminal:
		return true
	default:
		return false
	}
}

func hasCycle(entry string, adj map[string][]string) bool {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var dfs func(string) bool
	dfs = func(n string) bool {
		if visiting[n] {
			return true
		}
		if visited[n] {
			return false
		}
		visiting[n] = true
		for _, next := range adj[n] {
			if dfs(next) {
				return true
			}
		}
		visiting[n] = false
		visited[n] = true
		return false
	}
	return dfs(entry)
}
