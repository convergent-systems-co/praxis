package goals

import (
	"errors"
	"testing"
)

func TestApplicabilityReusesFreshMatchingBaseline(t *testing.T) {
	a,err:=ClassifyApplicability(ApplicabilityInput{BaselinePresent:true,DigestValid:true,GoalMatches:true,ValidityPredicatesSatisfied:true}); if err!=nil { t.Fatal(err) }; if a!=ApplicabilityReuse { t.Fatalf("got %s",a) }
}

func TestApplicabilityUsesDeltaForChangedArtifacts(t *testing.T) {
	a,err:=ClassifyApplicability(ApplicabilityInput{BaselinePresent:true,DigestValid:true,GoalMatches:true,ValidityPredicatesSatisfied:true,ChangedArtifacts:[]string{"spec-auth"}}); if err!=nil { t.Fatal(err) }; if a!=ApplicabilityDelta { t.Fatalf("got %s",a) }
}

func TestApplicabilityReplansOnInvalidPredicate(t *testing.T) {
	a,err:=ClassifyApplicability(ApplicabilityInput{BaselinePresent:true,DigestValid:true,GoalMatches:true,ValidityPredicatesSatisfied:false}); if err!=nil { t.Fatal(err) }; if a!=ApplicabilityReplan { t.Fatalf("got %s",a) }
}

func TestApplicabilityFailsClosedOnDigestMismatch(t *testing.T) {
	a,err:=ClassifyApplicability(ApplicabilityInput{BaselinePresent:true,DigestValid:false,GoalMatches:true,ValidityPredicatesSatisfied:true}); if a!=ApplicabilityReplan || !errors.Is(err,ErrBaselineDigestMismatch) { t.Fatalf("got %s err=%v",a,err) }
}
