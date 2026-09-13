package goals

import "testing"

func TestDelegatedClearRecommendationAutoResolves(t *testing.T) {
	d,err:=DecideRecommendationInterruption(RecommendationInput{Mode:RecommendationDelegated,HasDominantRecommendation:true,HasEvidence:true,ConfidenceSufficient:true,Reversible:true}); if err!=nil { t.Fatal(err) }; if d!=RecommendationAutoResolve { t.Fatalf("got %s",d) }
}

func TestExecutionAuthorizationAlwaysAsksUser(t *testing.T) {
	d,_:=DecideRecommendationInterruption(RecommendationInput{Mode:RecommendationDelegated,HasDominantRecommendation:true,HasEvidence:true,ConfidenceSufficient:true,Reversible:true,ExecutionAuthorization:true}); if d!=RecommendationAskUser { t.Fatalf("got %s",d) }
}

func TestMaterialIrreversibleRecommendationAlwaysAsksUser(t *testing.T) {
	d,_:=DecideRecommendationInterruption(RecommendationInput{Mode:RecommendationDelegated,HasDominantRecommendation:true,HasEvidence:true,ConfidenceSufficient:true,Material:true,Reversible:false}); if d!=RecommendationAskUser { t.Fatalf("got %s",d) }
}

func TestWeakRecommendationCanRemainOpenWithoutInterrupt(t *testing.T) {
	d,_:=DecideRecommendationInterruption(RecommendationInput{Mode:RecommendationDelegated,HasDominantRecommendation:false,HasEvidence:true,ConfidenceSufficient:false,Material:false,Reversible:true}); if d!=RecommendationLeaveOpen { t.Fatalf("got %s",d) }
}

func TestReviewAllModeDoesNotAutoResolve(t *testing.T) {
	d,_:=DecideRecommendationInterruption(RecommendationInput{Mode:RecommendationReviewAll,HasDominantRecommendation:true,HasEvidence:true,ConfidenceSufficient:true,Reversible:true}); if d!=RecommendationAskUser { t.Fatalf("got %s",d) }
}
