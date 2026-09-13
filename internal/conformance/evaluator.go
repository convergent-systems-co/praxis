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

type ClaimClass string
type Criticality string
type EvidenceStage string

const (
	Satisfied        Status        = "satisfied"
	Unsupported      Status        = "unsupported"
	Contradicted     Status        = "contradicted"
	Indeterminate    Status        = "indeterminate"
	Behavioral       ClaimClass    = "behavioral"
	Structural       ClaimClass    = "structural"
	Critical         Criticality   = "critical"
	Important        Criticality   = "important"
	Advisory         Criticality   = "advisory"
	StageContract    EvidenceStage = "contract"
	StageBehavior    EvidenceStage = "behavior"
	StageIntegration EvidenceStage = "integration"
	StageLifecycle   EvidenceStage = "lifecycle"
)

type Claim struct {
	ID               string        `json:"id"`
	Statement        string        `json:"statement"`
	RequiredEvidence []string      `json:"required_evidence"`
	Behavioral       bool          `json:"behavioral"`
	Critical         bool          `json:"critical"`
	Class            ClaimClass    `json:"class,omitempty"`
	Criticality      Criticality   `json:"criticality,omitempty"`
	RequiredStage    EvidenceStage `json:"required_stage,omitempty"`
	SourceRef        string        `json:"source_ref"`
}

type Evidence struct {
	ID          string        `json:"id"`
	ClaimID     string        `json:"claim_id"`
	Kind        string        `json:"kind"`
	Subject     string        `json:"subject"`
	Ref         string        `json:"ref"`
	Digest      string        `json:"digest,omitempty"`
	Stage       EvidenceStage `json:"stage,omitempty"`
	Supports    bool          `json:"supports"`
	Contradicts bool          `json:"contradicts"`
}

type Finding struct {
	ClaimID     string   `json:"claim_id"`
	Statement   string   `json:"statement"`
	SourceRef   string   `json:"source_ref"`
	Status      Status   `json:"status"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Reason      string   `json:"reason"`
	Critical    bool     `json:"critical"`
}

type Result struct {
	Version         string     `json:"version"`
	GoalDigest      string     `json:"goal_digest"`
	SourceSetDigest string     `json:"source_set_digest"`
	ClaimSetDigest  string     `json:"claim_set_digest"`
	EvidenceDigest  string     `json:"evidence_digest"`
	Claims          []Claim    `json:"claims"`
	Evidence        []Evidence `json:"evidence"`
	Findings        []Finding  `json:"findings"`
	Conformant      bool       `json:"conformant"`
	FrozenAt        time.Time  `json:"frozen_at"`
	Digest          string     `json:"digest"`
}

var proseKinds = map[string]bool{"adr": true, "spec": true, "plan": true, "documentation": true}

func Evaluate(goalDigest string, claims []Claim, evidence []Evidence, now time.Time) (Result, error) {
	if goalDigest == "" || len(claims) == 0 || now.IsZero() {
		return Result{}, errors.New("goal digest, claims, and freeze time are required")
	}
	cs := append([]Claim(nil), claims...)
	for i := range cs {
		cs[i].RequiredEvidence = append([]string(nil), cs[i].RequiredEvidence...)
		sort.Strings(cs[i].RequiredEvidence)
		normalizeClaim(&cs[i])
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].ID < cs[j].ID })
	es := append([]Evidence(nil), evidence...)
	sort.Slice(es, func(i, j int) bool { return es[i].ID < es[j].ID })
	seen := map[string]bool{}
	evidenceSeen := map[string]bool{}
	for _, e := range es {
		if e.ID == "" || e.Kind == "" || e.Ref == "" || evidenceSeen[e.ID] {
			return Result{}, errors.New("evidence requires unique id, kind, and reference")
		}
		evidenceSeen[e.ID] = true
	}
	findings := make([]Finding, 0, len(cs))
	conformant := true
	for _, c := range cs {
		if c.ID == "" || c.Statement == "" || c.SourceRef == "" || seen[c.ID] {
			return Result{}, errors.New("claims require unique id, statement, and source reference")
		}
		seen[c.ID] = true
		matched := []Evidence{}
		ids := []string{}
		for _, e := range es {
			if e.ClaimID == c.ID {
				matched = append(matched, e)
				ids = append(ids, e.ID)
			}
		}
		status := Unsupported
		reason := "required admissible evidence is absent"
		for _, e := range matched {
			if e.Contradicts && admissible(c, e) {
				status = Contradicted
				reason = "admissible evidence contradicts the goal claim"
				break
			}
		}
		if status != Contradicted {
			covered := map[string]bool{}
			support := false
			for _, e := range matched {
				if e.Supports && admissible(c, e) {
					support = true
					covered[e.Kind] = true
				}
			}
			allRequired := support
			for _, kind := range c.RequiredEvidence {
				if !covered[kind] {
					allRequired = false
				}
			}
			if allRequired {
				status = Satisfied
				reason = "all required admissible evidence supports the goal claim"
			} else if len(matched) > 0 {
				status = Indeterminate
				reason = "evidence exists but does not establish every required behavior and lifecycle level"
			}
		}
		isCritical := c.Criticality == Critical
		if isCritical && status != Satisfied {
			conformant = false
		}
		findings = append(findings, Finding{ClaimID: c.ID, Statement: c.Statement, SourceRef: c.SourceRef, Status: status, EvidenceIDs: ids, Reason: reason, Critical: isCritical})
	}
	sources := make([]string, 0, len(cs))
	sourceSeen := map[string]bool{}
	for _, c := range cs {
		if !sourceSeen[c.SourceRef] {
			sources = append(sources, c.SourceRef)
			sourceSeen[c.SourceRef] = true
		}
	}
	sort.Strings(sources)
	sourceSetDigest, err := digest(sources)
	if err != nil {
		return Result{}, err
	}
	claimSetDigest, err := digest(cs)
	if err != nil {
		return Result{}, err
	}
	evidenceDigest, err := digest(es)
	if err != nil {
		return Result{}, err
	}
	r := Result{Version: "v2", GoalDigest: goalDigest, SourceSetDigest: "sha256:" + sourceSetDigest, ClaimSetDigest: "sha256:" + claimSetDigest, EvidenceDigest: "sha256:" + evidenceDigest, Claims: cs, Evidence: es, Findings: findings, Conformant: conformant, FrozenAt: now.UTC()}
	unsigned := r
	unsigned.Digest = ""
	d, err := digest(unsigned)
	if err != nil {
		return Result{}, err
	}
	r.Digest = "sha256:" + d
	return r, nil
}

func VerifyFrozen(r Result) error {
	if r.Digest == "" || r.FrozenAt.IsZero() {
		return errors.New("result must be frozen")
	}
	unsigned := r
	unsigned.Digest = ""
	d, err := digest(unsigned)
	if err != nil {
		return err
	}
	if r.Digest != "sha256:"+d {
		return errors.New("frozen result digest mismatch")
	}
	return nil
}

func normalizeClaim(c *Claim) {
	if c.Class == "" {
		if c.Behavioral {
			c.Class = Behavioral
		} else {
			c.Class = Structural
		}
	}
	c.Behavioral = c.Class == Behavioral
	if c.Criticality == "" {
		if c.Critical {
			c.Criticality = Critical
		} else {
			c.Criticality = Important
		}
	}
	c.Critical = c.Criticality == Critical
	if c.RequiredStage == "" {
		if c.Behavioral {
			c.RequiredStage = StageBehavior
		} else {
			c.RequiredStage = StageContract
		}
	}
}

func admissible(c Claim, e Evidence) bool {
	if c.Behavioral && proseKinds[e.Kind] {
		return false
	}
	if len(c.RequiredEvidence) > 0 {
		found := false
		for _, kind := range c.RequiredEvidence {
			if kind == e.Kind {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	stage := e.Stage
	if stage == "" {
		stage = inferStage(e.Kind)
	}
	return stageRank(stage) >= stageRank(c.RequiredStage)
}

func inferStage(kind string) EvidenceStage {
	switch kind {
	case "runtime_test", "runtime_trace", "restart_test", "recovery_test", "security_test", "integration_test", "conformance_test":
		return StageBehavior
	default:
		return StageContract
	}
}
func stageRank(stage EvidenceStage) int {
	switch stage {
	case StageContract:
		return 1
	case StageBehavior:
		return 2
	case StageIntegration:
		return 3
	case StageLifecycle:
		return 4
	default:
		return 0
	}
}
func digest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
