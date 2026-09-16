package goalspublication

import (
	"testing"
)

func TestObservationResolutionSemanticEqualityIgnoresProviderOrder(t *testing.T) {
	a := recoveryReadBackObservation([]int{0, 1, 2}, assetDigests)
	b := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	if err := observationSemanticallyEqual(a, b); err != nil {
		t.Fatalf("provider ordering changed semantic observation: %v", err)
	}

	b.AssetDigests[0], b.AssetDigests[1] = b.AssetDigests[1], b.AssetDigests[0]
	if err := observationSemanticallyEqual(a, b); err == nil {
		t.Fatal("swapped identity-bound content was accepted")
	}
}

func TestObservationResolutionDigestIsStableAcrossInvocations(t *testing.T) {
	a := recoveryReadBackObservation([]int{0, 2, 1}, assetDigests)
	b := recoveryReadBackObservation([]int{2, 1, 0}, assetDigests)
	x := observationResolutionSnapshot{RequestID: "request", EffectID: "effect", IntentDigest: "sha256:1111111111111111111111111111111111111111111111111111111111111111", AuthorityDigest: "sha256:2222222222222222222222222222222222222222222222222222222222222222", ExecutionID: "execution", Step: "verify-draft", Adapter: "goals-recovery-github", Contract: "goals-established-state-publication/4", Operation: "publish-goals-from-established-state", Attempts: 1, Original: a, Reconciliation: b, ReconciliationEventID: "event", ReconciliationPayloadDigest: "sha256:3333333333333333333333333333333333333333333333333333333333333333"}
	y := x
	y.Original = recoveryReadBackObservation([]int{1, 0, 2}, assetDigests)
	y.Reconciliation = recoveryReadBackObservation([]int{2, 0, 1}, assetDigests)
	d1, err := resolutionDigest(x)
	if err != nil {
		t.Fatal(err)
	}
	d2, err := resolutionDigest(y)
	if err != nil {
		t.Fatal(err)
	}
	if d1 != d2 {
		t.Fatalf("resolution digest changed with provider ordering: %s != %s", d1, d2)
	}
}
