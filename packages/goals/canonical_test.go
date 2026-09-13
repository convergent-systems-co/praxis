package goals

import (
	"errors"
	"testing"
)

func baselineFixture() GoalBaseline {
	return GoalBaseline{ID:"g1",Version:"1",OriginalIntent:"goal",RefinedOutcome:"outcome",Rigor:RigorStructured,RecommendationMode:RecommendationReviewAll,Constraints:[]string{"b","a"},EvidenceRefs:[]string{"e2","e1"},Decisions:[]Decision{{ID:"d2",Statement:"two",Status:DecisionResolved},{ID:"d1",Statement:"one",Status:DecisionResolved}}}
}

func TestBaselineDigestIndependentOfSetOrdering(t *testing.T) {
	a:=baselineFixture(); b:=baselineFixture(); b.Constraints=[]string{"a","b"}; b.EvidenceRefs=[]string{"e1","e2"}; b.Decisions=[]Decision{b.Decisions[1],b.Decisions[0]}
	da,err:=a.ComputeDigest(); if err!=nil { t.Fatal(err) }; db,err:=b.ComputeDigest(); if err!=nil { t.Fatal(err) }
	if da!=db { t.Fatalf("canonical digest changed with ordering: %s != %s",da,db) }
}

func TestBaselineDigestChangesWithOutcome(t *testing.T) {
	a:=baselineFixture(); b:=baselineFixture(); b.RefinedOutcome="different"
	da,_:=a.ComputeDigest(); db,_:=b.ComputeDigest(); if da==db { t.Fatal("material baseline change must change digest") }
}

func TestVerifyBaselineDigestDetectsMutation(t *testing.T) {
	b:=baselineFixture(); d,_:=b.ComputeDigest(); b.Digest=d; b.RefinedOutcome="mutated"
	if err:=b.VerifyDigest(); !errors.Is(err,ErrBaselineDigestMismatch) { t.Fatalf("expected digest mismatch, got %v",err) }
}
