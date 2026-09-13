package learning

import (
	"testing"
	"time"
	"github.com/convergent-systems-co/praxis/internal/conformance"
)

func TestCandidateFromConformanceRequiresFrozenCriticalFailure(t *testing.T){
	if _,err:=CandidateFromConformance("c","graph@1","eval","rollback",conformance.Result{});err==nil{t.Fatal("unfrozen result must fail")}
	r,err:=conformance.Evaluate("sha256:g",[]conformance.Claim{{ID:"x",Statement:"behavior",Behavioral:true,Critical:true,SourceRef:"goal"}},nil,time.Date(2026,9,13,12,0,0,0,time.UTC));if err!=nil{t.Fatal(err)}
	c,err:=CandidateFromConformance("candidate-1","graph@1","replay-suite","graph@1",r);if err!=nil{t.Fatal(err)}
	if c.State!=CandidateProposed||len(c.SourceObservations)!=1||c.RollbackRef!="graph@1"{t.Fatalf("unexpected candidate %#v",c)}
	if CanTransition(c.State,CandidatePromoted){t.Fatal("candidate must not jump directly from proposed to promoted")}
}
