package learning

import (
	"errors"
	"sort"

	"github.com/convergent-systems-co/praxis/internal/conformance"
)

// CandidateFromConformance converts frozen critical conformance failures into a
// governed candidate. It diagnoses nothing by itself and cannot promote itself.
func CandidateFromConformance(id, target, evaluationRef, rollbackRef string, result conformance.Result) (Candidate,error) {
	if result.Digest=="" { return Candidate{},errors.New("conformance result must be frozen") }
	observations:=[]string{}
	for _,f:=range result.Findings { if f.Critical && f.Status!=conformance.Satisfied { observations=append(observations,"conformance:"+result.Digest+":"+f.ClaimID+":"+string(f.Status)) } }
	if len(observations)==0{return Candidate{},errors.New("no critical conformance failure to learn from")}
	sort.Strings(observations)
	c:=Candidate{ID:id,Target:target,SourceObservations:observations,ExpectedBenefit:"increase original-goal conformance",RiskClass:"behavioral-change",RequiredEvaluation:evaluationRef,RollbackRef:rollbackRef,State:CandidateProposed}
	if err:=c.Validate();err!=nil{return Candidate{},err}; return c,nil
}
