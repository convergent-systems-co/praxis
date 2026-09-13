package develop

import (
	"testing"

	"github.com/convergent-systems-co/praxis/packages/goals"
)

func TestMaterializePlanningBaselineBindsExactGoalDigest(t *testing.T) {
	goal:=goals.GoalBaseline{ID:"goal-1",Version:"1",OriginalIntent:"build it",RefinedOutcome:"deliver secure service",Rigor:goals.RigorRigorous,RecommendationMode:goals.RecommendationReviewAll}
	b,err:=MaterializeBaseline(goal,[]DevelopmentArtifact{{ID:"adr-001",Role:RoleADR,SourceGoal:"decision-1"},{ID:"spec-001",Role:RoleSpecification,SourceGoal:"spec-1"}},"plan-001")
	if err!=nil { t.Fatal(err) }
	if b.GoalBaselineDigest=="" { t.Fatal("planning baseline must bind Goal Baseline digest") }
}

func TestMaterializationRejectsMutatedGoalBaseline(t *testing.T) {
	goal:=goals.GoalBaseline{ID:"goal-1",Version:"1",OriginalIntent:"build it",RefinedOutcome:"deliver secure service",Rigor:goals.RigorRigorous,RecommendationMode:goals.RecommendationReviewAll}
	d,_:=goal.ComputeDigest(); goal.Digest=d; goal.RefinedOutcome="silently changed"
	if _,err:=MaterializeBaseline(goal,nil,""); err==nil { t.Fatal("mutated Goal Baseline must not materialize") }
}

func TestSliceMustBindPlanningBaselineAndKnownArtifacts(t *testing.T) {
	goal:=goals.GoalBaseline{ID:"goal-1",Version:"1",OriginalIntent:"build it",RefinedOutcome:"deliver secure service",Rigor:goals.RigorRigorous,RecommendationMode:goals.RecommendationReviewAll}
	b,err:=MaterializeBaseline(goal,[]DevelopmentArtifact{{ID:"adr-001",Role:RoleADR},{ID:"spec-001",Role:RoleSpecification}},"plan-001"); if err!=nil { t.Fatal(err) }
	s:=SliceContract{ID:"slice-1",PlanningBaselineID:b.GoalBaselineID,GoalBaselineDigest:b.GoalBaselineDigest,GoverningArtifacts:[]string{"adr-001","spec-001"},Scope:"auth service",AcceptanceEvidence:[]string{"tests pass"}}
	if err:=s.Validate(b); err!=nil { t.Fatal(err) }
	s.GoverningArtifacts=[]string{"missing"}; if err:=s.Validate(b); err==nil { t.Fatal("slice cannot reference unknown planning artifact") }
}

func TestSliceCannotUseDifferentGoalDigest(t *testing.T) {
	goal:=goals.GoalBaseline{ID:"goal-1",Version:"1",OriginalIntent:"build it",RefinedOutcome:"deliver secure service",Rigor:goals.RigorStructured,RecommendationMode:goals.RecommendationReviewAll}
	b,err:=MaterializeBaseline(goal,nil,""); if err!=nil { t.Fatal(err) }
	s:=SliceContract{ID:"slice-1",PlanningBaselineID:b.GoalBaselineID,GoalBaselineDigest:"sha256:other",Scope:"auth service"}
	if err:=s.Validate(b); err==nil { t.Fatal("slice with a different Goal Baseline digest must fail") }
}
