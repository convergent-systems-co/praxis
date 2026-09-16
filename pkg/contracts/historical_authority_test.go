package contracts

import (
	"strings"
	"testing"
	"time"
)

func TestExpiredHistoricalAuthorityEvidenceIsDistinctAndDeterministic(t *testing.T) {
	d := "sha256:" + strings.Repeat("a", 64)
	exp := ExpiredHistoricalAuthorityEvidence{
		ObjectKind: "authority request", ObjectID: "authority-request:test", ObjectVersion: "1", ObjectDigest: d,
		InstallationDigest: d, Principal: PrincipalRef{Kind: "human", ID: "owner"}, RequestDigest: d,
		IntentID: "intent:test", IntentDigest: d, DecisionRef: "decision:test", DecisionVersion: "1", DecisionDigest: d,
		GenerationRef: "generation:test", GenerationVersion: "1", GenerationDigest: d, ExecutionID: "execution:test",
		EffectIDs: []string{"execution:test:effect"}, EffectiveAt: time.Unix(100, 0).UTC(), ExpiresAt: time.Unix(200, 0).UTC(),
		HistoricalValidityEstablished: true, Historical: true, NonExecutable: true, Status: HistoricalAuthorityExpired,
	}
	digest, err := exp.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	exp.Digest = digest
	if err := exp.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := exp.VerifyDigest(); err != nil {
		t.Fatal(err)
	}
	changed := exp
	changed.ExecutionID = "execution:other"
	changed.Digest = exp.Digest
	if err := changed.VerifyDigest(); err == nil {
		t.Fatal("projection digest must bind execution identity")
	}
}

func TestExpiredHistoricalAuthorityEvidenceRejectsExecutableStatus(t *testing.T) {
	d := "sha256:" + strings.Repeat("b", 64)
	exp := ExpiredHistoricalAuthorityEvidence{ObjectKind: "authority request", ObjectID: "r", ObjectVersion: "1", ObjectDigest: d, InstallationDigest: d, Principal: PrincipalRef{Kind: "human", ID: "o"}, RequestDigest: d, IntentID: "i", IntentDigest: d, DecisionRef: "d", DecisionVersion: "1", DecisionDigest: d, GenerationRef: "g", GenerationVersion: "1", GenerationDigest: d, ExecutionID: "e", EffectIDs: []string{"x"}, EffectiveAt: time.Unix(1, 0), ExpiresAt: time.Unix(2, 0), HistoricalValidityEstablished: true, Historical: true, NonExecutable: true, Status: "active", Digest: d}
	if exp.Validate() == nil {
		t.Fatal("active status must not validate as historical evidence")
	}
}
