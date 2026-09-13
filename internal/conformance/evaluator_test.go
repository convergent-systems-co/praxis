package conformance

import (
	"testing"
	"time"
)

func TestBehavioralClaimRejectsProseOnlyEvidence(t *testing.T){
	now:=time.Date(2026,9,13,12,0,0,0,time.UTC)
	claims:=[]Claim{{ID:"c1",Statement:"system exhibits behavior",RequiredEvidence:[]string{"runtime_test"},Behavioral:true,Critical:true,SourceRef:"goal:1"}}
	evidence:=[]Evidence{{ID:"e1",ClaimID:"c1",Kind:"spec",Ref:"SPEC",Supports:true}}
	r,err:=Evaluate("sha256:goal",claims,evidence,now); if err!=nil{t.Fatal(err)}
	if r.Conformant||r.Findings[0].Status!=Indeterminate{t.Fatalf("prose must not satisfy behavior: %#v",r)}
}

func TestExecutableEvidenceSatisfiesBehavior(t *testing.T){
	now:=time.Date(2026,9,13,12,0,0,0,time.UTC)
	claims:=[]Claim{{ID:"c1",Statement:"system exhibits behavior",RequiredEvidence:[]string{"runtime_test"},Behavioral:true,Critical:true,SourceRef:"goal:1"}}
	evidence:=[]Evidence{{ID:"e1",ClaimID:"c1",Kind:"runtime_test",Ref:"test:1",Supports:true}}
	r,err:=Evaluate("sha256:goal",claims,evidence,now); if err!=nil{t.Fatal(err)}
	if !r.Conformant||r.Findings[0].Status!=Satisfied{t.Fatalf("runtime evidence should satisfy: %#v",r)}
}

func TestFreezeDigestOrderStable(t *testing.T){
	now:=time.Date(2026,9,13,12,0,0,0,time.UTC)
	claims:=[]Claim{{ID:"b",Statement:"b",SourceRef:"g"},{ID:"a",Statement:"a",SourceRef:"g"}}
	evidence:=[]Evidence{{ID:"2",ClaimID:"b",Kind:"test",Ref:"2",Supports:true},{ID:"1",ClaimID:"a",Kind:"test",Ref:"1",Supports:true}}
	a,_:=Evaluate("sha256:g",claims,evidence,now); b,_:=Evaluate("sha256:g",[]Claim{claims[1],claims[0]},[]Evidence{evidence[1],evidence[0]},now)
	if a.Digest!=b.Digest{t.Fatalf("digest depends on input order: %s %s",a.Digest,b.Digest)}
}

func TestOracleOnlyScoresFrozenResult(t *testing.T){
	if _,err:=ScoreFrozen(Result{},[]OracleExpectation{{ClaimID:"c1",Expected:Unsupported}}); err==nil{t.Fatal("unfrozen result must fail")}
	now:=time.Date(2026,9,13,12,0,0,0,time.UTC)
	r,_:=Evaluate("sha256:g",[]Claim{{ID:"c1",Statement:"missing behavior",Behavioral:true,Critical:true,SourceRef:"g"}},nil,now)
	s,err:=ScoreFrozen(r,[]OracleExpectation{{ClaimID:"c1",Expected:Unsupported}}); if err!=nil{t.Fatal(err)}
	if s.TruePositive!=1||s.FalseNegative!=0{t.Fatalf("unexpected oracle score %#v",s)}
}
