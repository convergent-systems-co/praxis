package kernel

import (
	"context"
	"testing"
)

type stubNodeExecutor struct{ calls int }
func (s *stubNodeExecutor) ExecuteNode(context.Context, GraphDef, NodeDef, *RunExecution) (NodeResult,error) { s.calls++; return NodeResult{Outcome:"done"},nil }

type stubSubgraphRuntime struct {
	state RunState
	evidence []string
	suspension *Suspension
	calls int
	ref SubgraphRef
}
func (s *stubSubgraphRuntime) ExecuteSubgraph(_ context.Context,_ GraphDef,_ NodeDef,_ *RunExecution,ref SubgraphRef) (RunState,[]string,*Suspension,error) { s.calls++; s.ref=ref; return s.state,append([]string(nil),s.evidence...),s.suspension,nil }

func TestComposingExecutorMapsChildTerminalStateThroughExplicitContract(t *testing.T) {
	delegate:=&stubNodeExecutor{}
	sub:=&stubSubgraphRuntime{state:RunSucceeded,evidence:[]string{"baseline:sha256:x"}}
	ref:=&SubgraphRef{GraphID:"praxis.package.goals.default",GraphVersion:"0.1.0",EntryPointID:"goals"}
	executor:=ComposingExecutor{Delegate:delegate,Subgraphs:sub,Outcomes:map[string]SubgraphOutcomeContract{"praxis.package.goals.default@0.1.0":{Succeeded:"baseline",Failed:"blocked",Cancelled:"blocked"}}}
	result,err:=executor.ExecuteNode(context.Background(),GraphDef{ID:"parent",Version:"1"},NodeDef{ID:"goals",Class:NodeSubgraph,Subgraph:ref},&RunExecution{RunID:"r1"}); if err!=nil { t.Fatal(err) }
	if result.Outcome!="baseline" || len(result.Evidence)!=1 { t.Fatalf("unexpected result %+v",result) }
	if sub.calls!=1 || delegate.calls!=0 { t.Fatalf("subgraph=%d delegate=%d",sub.calls,delegate.calls) }
	if sub.ref.GraphID!=ref.GraphID || sub.ref.GraphVersion!=ref.GraphVersion { t.Fatal("exact subgraph identity was not preserved") }
}

func TestComposingExecutorNeverInfersOutcomeWithoutContract(t *testing.T) {
	sub:=&stubSubgraphRuntime{state:RunSucceeded}
	ref:=&SubgraphRef{GraphID:"child",GraphVersion:"1"}
	executor:=ComposingExecutor{Delegate:&stubNodeExecutor{},Subgraphs:sub,Outcomes:map[string]SubgraphOutcomeContract{}}
	if _,err:=executor.ExecuteNode(context.Background(),GraphDef{ID:"p",Version:"1"},NodeDef{ID:"whatever",Class:NodeSubgraph,Subgraph:ref},&RunExecution{RunID:"r1"}); err==nil { t.Fatal("subgraph outcome must not be inferred from node or graph names") }
}

func TestComposingExecutorPropagatesChildSuspension(t *testing.T) {
	wait:=&Suspension{Kind:WaitHumanDecision,Ref:"decision:1"}
	sub:=&stubSubgraphRuntime{state:RunSuspended,suspension:wait}
	ref:=&SubgraphRef{GraphID:"child",GraphVersion:"1"}
	executor:=ComposingExecutor{Delegate:&stubNodeExecutor{},Subgraphs:sub,Outcomes:map[string]SubgraphOutcomeContract{"child@1":{Succeeded:"ok",Failed:"failed",Cancelled:"cancelled"}}}
	result,err:=executor.ExecuteNode(context.Background(),GraphDef{ID:"p",Version:"1"},NodeDef{ID:"child",Class:NodeSubgraph,Subgraph:ref},&RunExecution{RunID:"r1"}); if err!=nil { t.Fatal(err) }
	if result.Suspension==nil || result.Suspension.Ref!="decision:1" { t.Fatalf("child wait was not propagated: %+v",result) }
}

func TestComposingExecutorDelegatesOrdinaryNodes(t *testing.T) {
	delegate:=&stubNodeExecutor{}
	executor:=ComposingExecutor{Delegate:delegate}
	result,err:=executor.ExecuteNode(context.Background(),GraphDef{ID:"p",Version:"1"},NodeDef{ID:"a",Class:NodeDeterministic},&RunExecution{RunID:"r1"}); if err!=nil { t.Fatal(err) }
	if result.Outcome!="done" || delegate.calls!=1 { t.Fatalf("ordinary node was not delegated: %+v calls=%d",result,delegate.calls) }
}
