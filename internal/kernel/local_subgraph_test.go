package kernel

import (
	"context"
	"testing"
)

type mapExecutor struct{ outcomes map[string]NodeResult }
func (m mapExecutor) ExecuteNode(_ context.Context,_ GraphDef,node NodeDef,_ *RunExecution) (NodeResult,error) { return m.outcomes[node.ID],nil }

func TestLocalSubgraphRuntimeExecutesExactRegisteredGraph(t *testing.T) {
	child:=GraphDef{ID:"child",Version:"1",EntryNode:"work",Nodes:[]NodeDef{{ID:"work",Class:NodeDeterministic},{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}},Transitions:[]TransitionDef{{From:"work",Outcome:"ok",To:"done"}}}
	registry,err:=NewGraphRegistry(child); if err!=nil { t.Fatal(err) }
	runtime:=LocalSubgraphRuntime{Registry:registry,Executor:mapExecutor{outcomes:map[string]NodeResult{"work":{Outcome:"ok",Evidence:[]string{"e1"}}}}}
	state,evidence,wait,err:=runtime.ExecuteSubgraph(context.Background(),GraphDef{ID:"parent",Version:"1"},NodeDef{ID:"child-node",Class:NodeSubgraph},&RunExecution{RunID:"parent-run"},SubgraphRef{GraphID:"child",GraphVersion:"1"}); if err!=nil { t.Fatal(err) }
	if state!=RunSucceeded || wait!=nil || len(evidence)!=1 || evidence[0]!="e1" { t.Fatalf("unexpected child result state=%s evidence=%v wait=%+v",state,evidence,wait) }
}

func TestLocalSubgraphRuntimeRejectsUnregisteredVersion(t *testing.T) {
	child:=GraphDef{ID:"child",Version:"1",EntryNode:"done",Nodes:[]NodeDef{{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}}}
	registry,_:=NewGraphRegistry(child)
	runtime:=LocalSubgraphRuntime{Registry:registry,Executor:mapExecutor{}}
	if _,_,_,err:=runtime.ExecuteSubgraph(context.Background(),GraphDef{ID:"p",Version:"1"},NodeDef{ID:"n",Class:NodeSubgraph},&RunExecution{RunID:"r"},SubgraphRef{GraphID:"child",GraphVersion:"2"}); err==nil { t.Fatal("unregistered child version must fail") }
}

func TestLocalSubgraphRuntimePropagatesExactChildWait(t *testing.T) {
	child:=GraphDef{ID:"child",Version:"1",EntryNode:"human",Nodes:[]NodeDef{{ID:"human",Class:NodeHuman},{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}},Transitions:[]TransitionDef{{From:"human",Outcome:"ok",To:"done"}}}
	registry,_:=NewGraphRegistry(child)
	runtime:=LocalSubgraphRuntime{Registry:registry,Executor:mapExecutor{outcomes:map[string]NodeResult{"human":{Suspension:&Suspension{Kind:WaitHumanDecision,Ref:"decision:abc"}}}}}
	state,_,wait,err:=runtime.ExecuteSubgraph(context.Background(),GraphDef{ID:"p",Version:"1"},NodeDef{ID:"n",Class:NodeSubgraph},&RunExecution{RunID:"r"},SubgraphRef{GraphID:"child",GraphVersion:"1"}); if err!=nil { t.Fatal(err) }
	if state!=RunSuspended || wait==nil || wait.Ref!="decision:abc" { t.Fatalf("unexpected suspension state=%s wait=%+v",state,wait) }
}

func TestGraphRegistryRejectsDuplicateIdentityVersion(t *testing.T) {
	g:=GraphDef{ID:"g",Version:"1",EntryNode:"done",Nodes:[]NodeDef{{ID:"done",Class:NodeTerminal,TerminalState:RunSucceeded}}}
	r,err:=NewGraphRegistry(g); if err!=nil { t.Fatal(err) }
	if err:=r.Register(g); err==nil { t.Fatal("duplicate graph identity/version must fail") }
}
