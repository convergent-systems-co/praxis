package conformance

import (
	"testing"
	"time"
)

func TestBehavioralClaimRejectsProseOnlyEvidence(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	claims := []Claim{{ID: "c1", Statement: "system exhibits behavior", RequiredEvidence: []string{"runtime_test"}, Behavioral: true, Critical: true, SourceRef: "goal:1"}}
	evidence := []Evidence{{ID: "e1", ClaimID: "c1", Kind: "spec", Ref: "SPEC", Supports: true}}
	r, err := Evaluate("sha256:goal", claims, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Conformant || r.Findings[0].Status != Indeterminate {
		t.Fatalf("prose must not satisfy behavior: %#v", r)
	}
}

func TestExecutableEvidenceSatisfiesBehavior(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	claims := []Claim{{ID: "c1", Statement: "system exhibits behavior", RequiredEvidence: []string{"runtime_test"}, Behavioral: true, Critical: true, SourceRef: "goal:1"}}
	evidence := []Evidence{{ID: "e1", ClaimID: "c1", Kind: "runtime_test", Ref: "test:1", Supports: true}}
	r, err := Evaluate("sha256:goal", claims, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Conformant || r.Findings[0].Status != Satisfied {
		t.Fatalf("runtime evidence should satisfy: %#v", r)
	}
}

func TestFreezeDigestOrderStable(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	claims := []Claim{{ID: "b", Statement: "b", SourceRef: "g"}, {ID: "a", Statement: "a", SourceRef: "g"}}
	evidence := []Evidence{{ID: "2", ClaimID: "b", Kind: "test", Ref: "2", Supports: true}, {ID: "1", ClaimID: "a", Kind: "test", Ref: "1", Supports: true}}
	a, _ := Evaluate("sha256:g", claims, evidence, now)
	b, _ := Evaluate("sha256:g", []Claim{claims[1], claims[0]}, []Evidence{evidence[1], evidence[0]}, now)
	if a.Digest != b.Digest {
		t.Fatalf("digest depends on input order: %s %s", a.Digest, b.Digest)
	}
}

func TestAllRequiredEvidenceClassesAndLifecycleAreRequired(t *testing.T) {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	claim := Claim{ID: "c1", Statement: "behavior survives restart", RequiredEvidence: []string{"runtime_test", "restart_test"}, Class: Behavioral, Criticality: Critical, RequiredStage: StageLifecycle, SourceRef: "ADR-011"}
	evidence := []Evidence{{ID: "e1", ClaimID: "c1", Kind: "runtime_test", Ref: "test:runtime", Stage: StageIntegration, Supports: true}}
	r, err := Evaluate("sha256:g", []Claim{claim}, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	if r.Conformant || r.Findings[0].Status != Indeterminate {
		t.Fatalf("partial or pre-lifecycle evidence must fail closed: %#v", r)
	}
	evidence = append(evidence, Evidence{ID: "e2", ClaimID: "c1", Kind: "restart_test", Ref: "test:restart", Stage: StageLifecycle, Supports: true})
	evidence[0].Stage = StageLifecycle
	r, err = Evaluate("sha256:g", []Claim{claim}, evidence, now)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Conformant {
		t.Fatalf("all required lifecycle evidence should satisfy: %#v", r)
	}
}

func TestFrozenResultCarriesEverySetDigest(t *testing.T) {
	r, err := Evaluate("sha256:g", []Claim{{ID: "c", Statement: "contract", SourceRef: "ADR-001"}}, []Evidence{{ID: "e", ClaimID: "c", Kind: "code", Ref: "x.go", Supports: true}}, time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.SourceSetDigest == "" || r.ClaimSetDigest == "" || r.EvidenceDigest == "" || r.Digest == "" {
		t.Fatalf("missing frozen digest: %#v", r)
	}
}

func TestOracleOnlyScoresFrozenResult(t *testing.T) {
	if _, err := ScoreFrozen(Result{}, []OracleExpectation{{ClaimID: "c1", Expected: Unsupported}}); err == nil {
		t.Fatal("unfrozen result must fail")
	}
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	r, _ := Evaluate("sha256:g", []Claim{{ID: "c1", Statement: "missing behavior", Behavioral: true, Critical: true, SourceRef: "g"}}, nil, now)
	s, err := ScoreFrozen(r, []OracleExpectation{{ClaimID: "c1", Expected: Unsupported}})
	if err != nil {
		t.Fatal(err)
	}
	if s.TruePositive != 1 || s.FalseNegative != 0 {
		t.Fatalf("unexpected oracle score %#v", s)
	}
}

func TestOracleRejectsMutationAfterFreeze(t *testing.T) {
	r, _ := Evaluate("sha256:g", []Claim{{ID: "c", Statement: "original", SourceRef: "ADR-001"}}, nil, time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))
	r.Findings[0].Statement = "mutated"
	if _, err := ScoreFrozen(r, []OracleExpectation{{ClaimID: "c", Expected: Unsupported}}); err == nil {
		t.Fatal("oracle must reject a mutated frozen report")
	}
}

func TestOracleCanMatchWithheldSemanticTermsAfterFreeze(t *testing.T) {
	r, _ := Evaluate("sha256:g", []Claim{{ID: "blind-id", Statement: "agents execute immutable operational graphs", SourceRef: "ADR-004", Behavioral: true, Critical: true}}, nil, time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC))
	score, err := ScoreFrozen(r, []OracleExpectation{{ID: "positive-control", RequiredTerms: []string{"agents", "operational graphs"}, Expected: Unsupported}})
	if err != nil {
		t.Fatal(err)
	}
	if score.TruePositive != 1 || score.Matches[0].ClaimID != "blind-id" {
		t.Fatalf("unexpected semantic score: %#v", score)
	}
}
