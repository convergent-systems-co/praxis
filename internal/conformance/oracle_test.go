package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func oracleTestAuthority(scope string, claimSet, goal string) OracleAuthority {
	return OracleAuthority{
		SchemaVersion: OracleSchemaVersion, OracleID: "oracle-test", Generation: 1,
		Scope: scope, QualificationContext: "test-context", ClaimSetDigest: claimSet,
		GoalDigest: goal, EffectiveFrom: "2026-09-14T00:00:00Z",
		ProvenanceRefs: []string{"ADR-049", "SPEC-018"},
	}
}

func TestLegacyOracleRequiresImmutableAuthorityBinding(t *testing.T) {
	root := t.TempDir()
	oraclePath := "oracle.json"
	oracleBytes := []byte(`[{"id":"historical","claim_id":"OI-001","expected":"indeterminate","basis_ref":"ADR-003"}]`)
	if err := os.WriteFile(filepath.Join(root, oraclePath), oracleBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(oracleBytes)
	authority := oracleTestAuthority(OracleScopeHistorical, "sha256:claims", "sha256:goal")
	authority.OraclePath = oraclePath
	authority.OracleDigest = "sha256:" + hex.EncodeToString(sum[:])
	authorityBytes, _ := json.Marshal(authority)
	if err := os.WriteFile(filepath.Join(root, "authority.json"), authorityBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadOracle(root, oraclePath, "authority.json")
	if err != nil || loaded.OracleID != "oracle-test" || len(loaded.Expectations) != 1 {
		t.Fatalf("legacy authority binding failed: %#v %v", loaded, err)
	}
	if err := os.WriteFile(filepath.Join(root, oraclePath), []byte(`[]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOracle(root, oraclePath, "authority.json"); err == nil {
		t.Fatal("mutated historical oracle bytes must fail digest binding")
	}
}

func TestReleaseOracleBindsCurrentDenominatorAndRejectsAmbiguity(t *testing.T) {
	result := Result{ClaimSetDigest: "sha256:claims", GoalDigest: "sha256:goal", Findings: []Finding{{ClaimID: "OI-001"}}}
	oracle := VersionedOracle{OracleAuthority: oracleTestAuthority(OracleScopeRelease, result.ClaimSetDigest, result.GoalDigest), Expectations: []OracleExpectation{{ID: "e1", ClaimID: "OI-001", Expected: Satisfied, BasisRef: "ADR-003"}}}
	if err := oracle.ValidateFor(result); err != nil {
		t.Fatal(err)
	}
	oracle.Expectations = nil
	if err := oracle.ValidateFor(result); err == nil {
		t.Fatal("release oracle without expectations must fail closed")
	}
	oracle.Expectations = []OracleExpectation{{ID: "e1", ClaimID: "OI-001", Expected: Satisfied, BasisRef: "ADR-003"}}
	oracle.Supersedes = "historical"
	oracle.SupersessionReason = ""
	if err := oracle.ValidateFor(result); err == nil {
		t.Fatal("ambiguous supersession must fail closed")
	}
	oracle.SupersessionReason = "context changed"
	oracle.Expectations = []OracleExpectation{{ID: "e1", ClaimID: "OI-001", Expected: Satisfied, BasisRef: "ADR-003"}, {ID: "e2", ClaimID: "OI-001", Expected: Satisfied, BasisRef: "ADR-003"}}
	if err := oracle.ValidateFor(result); err == nil {
		t.Fatal("duplicate release claim coverage must fail closed")
	}
}
