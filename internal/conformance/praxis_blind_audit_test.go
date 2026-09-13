package conformance

import (
	"strings"
	"testing"
	"time"
)

func TestPraxisBlindGoalAuditHasIndependentComprehensiveDenominator(t *testing.T) {
	claims := PraxisOriginalIntentClaims()
	if len(claims) < 30 {
		t.Fatalf("original-intent denominator is too small: %d", len(claims))
	}
	wantSources := map[string]bool{"ADR-003": false, "ADR-009": false, "ADR-024": false, "ADR-038": false, "ADR-043": false, "ADR-045": false, "ADR-048": false}
	for _, c := range claims {
		for source := range wantSources {
			if strings.Contains(c.SourceRef, source) {
				wantSources[source] = true
			}
		}
		if strings.Contains(c.SourceRef, "ADR-049") || strings.Contains(c.SourceRef, "PLAN-") {
			t.Fatalf("remediation/plan contaminated denominator: %s", c.SourceRef)
		}
	}
	for source, found := range wantSources {
		if !found {
			t.Errorf("original-intent dimension absent: %s", source)
		}
	}
}

func TestPraxisBlindGoalAuditFreezesMachineReadableFailure(t *testing.T) {
	root := "../.."
	evidence, err := LoadEvidence(root, PraxisEvidenceInventory())
	if err != nil {
		t.Fatal(err)
	}
	goalDigest, err := SourceSetDigest(root, OriginalIntentSources)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Evaluate(goalDigest, PraxisOriginalIntentClaims(), evidence, time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if r.Digest == "" || r.SourceSetDigest == "" || r.ClaimSetDigest == "" || r.EvidenceDigest == "" {
		t.Fatalf("audit was not fully frozen: %#v", r)
	}
	if r.Conformant {
		t.Fatal("discovery audit must be capable of reporting current critical gaps")
	}
	unsupported := 0
	for _, f := range r.Findings {
		if f.Status == Unsupported || f.Status == Indeterminate {
			unsupported++
		}
	}
	if unsupported == 0 {
		t.Fatal("expected discovery findings, not a conformance assertion")
	}
}

func TestEvidenceInventoryDoesNotTreatTestSourceAsExecutedBehavior(t *testing.T) {
	evidence, err := LoadEvidence("../..", []InventoryArtifact{{ID: "test-source", Kind: "integration_test", Stage: StageIntegration, Ref: "packages/develop/runtime_test.go", ClaimIDs: []string{"OI-033"}}})
	if err != nil {
		t.Fatal(err)
	}
	if evidence[0].Stage != StageContract {
		t.Fatalf("test source self-attested behavior: %#v", evidence[0])
	}
}
