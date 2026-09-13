package goals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

var ErrBaselineDigestMismatch = errors.New("goal baseline digest mismatch")

type canonicalDecision struct {
	ID string `json:"id"`
	Statement string `json:"statement"`
	Status DecisionStatus `json:"status"`
	Recommendation string `json:"recommendation,omitempty"`
	Rationale string `json:"rationale,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Reversible bool `json:"reversible"`
	Material bool `json:"material"`
	HumanRequired bool `json:"human_required"`
	AutoAccepted bool `json:"auto_accepted"`
}

type canonicalBaseline struct {
	Version string `json:"version"`
	OriginalIntent string `json:"original_intent"`
	RefinedOutcome string `json:"refined_outcome"`
	Scope string `json:"scope,omitempty"`
	NonGoals []string `json:"non_goals,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
	SuccessCriteria []string `json:"success_criteria,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Assumptions []string `json:"assumptions,omitempty"`
	Decisions []canonicalDecision `json:"decisions,omitempty"`
	Artifacts []ArtifactRef `json:"artifacts,omitempty"`
	PlanRef string `json:"plan_ref,omitempty"`
	ValidityPredicates []string `json:"validity_predicates,omitempty"`
	Rigor Rigor `json:"rigor"`
	RecommendationMode RecommendationMode `json:"recommendation_mode"`
}

func (b GoalBaseline) CanonicalBytes() ([]byte,error) {
	if err:=b.Validate(); err!=nil { return nil,err }
	cloneStrings:=func(in []string) []string { out:=append([]string(nil),in...); sort.Strings(out); return out }
	decisions:=make([]canonicalDecision,0,len(b.Decisions))
	for _,d:=range b.Decisions {
		decisions=append(decisions,canonicalDecision{ID:d.ID,Statement:d.Statement,Status:d.Status,Recommendation:d.Recommendation,Rationale:d.Rationale,EvidenceRefs:cloneStrings(d.EvidenceRefs),Reversible:d.Reversible,Material:d.Material,HumanRequired:d.HumanRequired,AutoAccepted:d.AutoAccepted})
	}
	sort.Slice(decisions,func(i,j int) bool { return decisions[i].ID<decisions[j].ID })
	artifacts:=append([]ArtifactRef(nil),b.Artifacts...)
	sort.Slice(artifacts,func(i,j int) bool { return artifacts[i].ID<artifacts[j].ID })
	c:=canonicalBaseline{Version:b.Version,OriginalIntent:b.OriginalIntent,RefinedOutcome:b.RefinedOutcome,Scope:b.Scope,NonGoals:cloneStrings(b.NonGoals),Constraints:cloneStrings(b.Constraints),SuccessCriteria:cloneStrings(b.SuccessCriteria),EvidenceRefs:cloneStrings(b.EvidenceRefs),Assumptions:cloneStrings(b.Assumptions),Decisions:decisions,Artifacts:artifacts,PlanRef:b.PlanRef,ValidityPredicates:cloneStrings(b.ValidityPredicates),Rigor:b.Rigor,RecommendationMode:b.RecommendationMode}
	return json.Marshal(c)
}

func (b GoalBaseline) ComputeDigest() (string,error) {
	payload,err:=b.CanonicalBytes(); if err!=nil { return "",err }
	sum:=sha256.Sum256(payload)
	return "sha256:"+hex.EncodeToString(sum[:]),nil
}

func (b GoalBaseline) VerifyDigest() error {
	if b.Digest=="" { return nil }
	d,err:=b.ComputeDigest(); if err!=nil { return err }
	if d!=b.Digest { return ErrBaselineDigestMismatch }
	return nil
}
