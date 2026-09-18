package kernel

import "testing"

func TestGraphRejectsUnboundedCycle(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "a",
		Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}, {ID: "b", Class: NodeCondition}},
		Transitions: []TransitionDef{{From: "a", Outcome: "next", To: "b"}, {From: "b", Outcome: "again", To: "a"}},
	}
	if err := g.Validate(); err == nil { t.Fatal("unbounded execution cycle must be rejected") }
}

func TestGraphAllowsBoundedCycle(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "a", MaxTransitions: 10,
		Nodes: []NodeDef{{ID: "a", Class: NodeDeterministic}, {ID: "b", Class: NodeCondition}},
		Transitions: []TransitionDef{{From: "a", Outcome: "next", To: "b"}, {From: "b", Outcome: "again", To: "a"}},
	}
	if err := g.Validate(); err != nil { t.Fatalf("bounded cycle rejected: %v", err) }
}

func TestGraphRejectsDuplicateOutcome(t *testing.T) {
	g := GraphDef{
		ID: "g1", Version: "v1", EntryNode: "a",
		Nodes: []NodeDef{{ID: "a", Class: NodeCondition}, {ID: "b", Class: NodeTerminal, TerminalState: RunSucceeded}, {ID: "c", Class: NodeTerminal, TerminalState: RunSucceeded}},
		Transitions: []TransitionDef{{From: "a", Outcome: "ok", To: "b"}, {From: "a", Outcome: "ok", To: "c"}},
	}
	if err := g.Validate(); err == nil { t.Fatal("duplicate outcome must be rejected") }
}

func TestSubgraphNodeRequiresExplicitGraphIdentity(t *testing.T) {
	g := GraphDef{ID:"parent",Version:"1",EntryNode:"child",Nodes:[]NodeDef{{ID:"child",Class:NodeSubgraph},{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}},Transitions:[]TransitionDef{{From:"child",Outcome:"done",To:"done"}}}
	if err:=g.Validate(); err==nil { t.Fatal("subgraph node without graph identity must fail") }
}

func TestSubgraphReferenceValidates(t *testing.T) {
	g := GraphDef{ID:"parent",Version:"1",EntryNode:"child",Nodes:[]NodeDef{{ID:"child",Class:NodeSubgraph,Subgraph:&SubgraphRef{GraphID:"praxis.package.goals.default",GraphVersion:"0.1.0",EntryPointID:"goals"}},{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}},Transitions:[]TransitionDef{{From:"child",Outcome:"done",To:"done"}}}
	if err:=g.Validate(); err!=nil { t.Fatalf("explicit subgraph identity rejected: %v",err) }
}

func TestNonSubgraphCannotDeclareSubgraphReference(t *testing.T) {
	g := GraphDef{ID:"parent",Version:"1",EntryNode:"a",Nodes:[]NodeDef{{ID:"a",Class:NodeDeterministic,Subgraph:&SubgraphRef{GraphID:"child",GraphVersion:"1"}},{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}},Transitions:[]TransitionDef{{From:"a",Outcome:"done",To:"done"}}}
	if err:=g.Validate(); err==nil { t.Fatal("non-subgraph node must not carry subgraph identity") }
}
