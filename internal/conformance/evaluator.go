package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"
)

type Status string

const (
	Satisfied Status = "satisfied"
	Unsupported Status = "unsupported"
	Contradicted Status = "contradicted"
	Indeterminate Status = "indeterminate"
)

type Claim struct {
	ID string `json:"id"`
	Statement string `json:"statement"`
	RequiredEvidence []string `json:"required_evidence"`
	Behavioral bool `json:"behavioral"`
	Critical bool `json:"critical"`
	SourceRef string `json:"source_ref"`
}

type Evidence struct {
	ID string `json:"id"`
	ClaimID string `json:"claim_id"`
	Kind string `json:"kind"`
	Subject string `json:"subject"`
	Ref string `json:"ref"`
	Supports bool `json:"supports"`
	Contradicts bool `json:"contradicts"`
}

type Finding struct {
	ClaimID string `json:"claim_id"`
	Status Status `json:"status"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Reason string `json:"reason"`
	Critical bool `json:"critical"`
}

type Result struct {
	Version string `json:"version"`
	GoalDigest string `json:"goal_digest"`
	EvidenceDigest string `json:"evidence_digest"`
	Findings []Finding `json:"findings"`
	Conformant bool `json:"conformant"`
	FrozenAt time.Time `json:"frozen_at"`
	Digest string `json:"digest"`
}

var proseKinds = map[string]bool{"adr":true,"spec":true,"plan":true,"documentation":true}

func Evaluate(goalDigest string, claims []Claim, evidence []Evidence, now time.Time) (Result, error) {
	if goalDigest == "" || len(claims) == 0 || now.IsZero() { return Result{}, errors.New("goal digest, claims, and freeze time are required") }
	cs:=append([]Claim(nil),claims...); sort.Slice(cs,func(i,j int)bool{return cs[i].ID<cs[j].ID})
	es:=append([]Evidence(nil),evidence...); sort.Slice(es,func(i,j int)bool{return es[i].ID<es[j].ID})
	seen:=map[string]bool{}; findings:=make([]Finding,0,len(cs)); conformant:=true
	for _,c:=range cs {
		if c.ID==""||c.Statement==""||c.SourceRef==""||seen[c.ID] { return Result{},errors.New("claims require unique id, statement, and source reference") }; seen[c.ID]=true
		matched:=[]Evidence{}; ids:=[]string{}
		for _,e:=range es { if e.ClaimID==c.ID { matched=append(matched,e); ids=append(ids,e.ID) } }
		status:=Unsupported; reason:="required admissible evidence is absent"
		for _,e:=range matched { if e.Contradicts { status=Contradicted; reason="admissible evidence contradicts the goal claim"; break } }
		if status!=Contradicted {
			admissible:=false
			for _,e:=range matched { if e.Supports && (!c.Behavioral || !proseKinds[e.Kind]) && requiredKind(c.RequiredEvidence,e.Kind) { admissible=true; break } }
			if admissible { status=Satisfied; reason="admissible evidence supports the goal claim" } else if len(matched)>0 { status=Indeterminate; reason="evidence exists but does not establish the required behavior" }
		}
		if c.Critical && status!=Satisfied { conformant=false }
		findings=append(findings,Finding{ClaimID:c.ID,Status:status,EvidenceIDs:ids,Reason:reason,Critical:c.Critical})
	}
	evidenceDigest,err:=digest(es); if err!=nil{return Result{},err}
	r:=Result{Version:"v1",GoalDigest:goalDigest,EvidenceDigest:evidenceDigest,Findings:findings,Conformant:conformant,FrozenAt:now.UTC()}
	unsigned:=r; unsigned.Digest=""; d,err:=digest(unsigned); if err!=nil{return Result{},err}; r.Digest="sha256:"+d
	return r,nil
}

func requiredKind(required []string, kind string) bool { if len(required)==0{return true}; for _,k:=range required{if k==kind{return true}}; return false }
func digest(v any)(string,error){ b,err:=json.Marshal(v); if err!=nil{return "",err}; h:=sha256.Sum256(b); return hex.EncodeToString(h[:]),nil }
