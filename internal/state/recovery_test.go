package state

import "testing"

func TestAmbiguousEffectFailsClosedWithoutReconciliation(t *testing.T) {
	action, err := ClassifyEffectRecovery(RecoverableEffect{ID: "e1", State: EffectUnknown})
	if err == nil || action != RecoveryFailClosed {
		t.Fatalf("expected fail-closed, got action=%s err=%v", action, err)
	}
}

func TestAmbiguousEffectReconcilesWhenSupported(t *testing.T) {
	action, err := ClassifyEffectRecovery(RecoverableEffect{ID: "e1", State: EffectDispatched, ReconcileCapable: true})
	if err != nil || action != RecoveryReconcile {
		t.Fatalf("expected reconcile, got action=%s err=%v", action, err)
	}
}

func TestAmbiguousEffectCanRedispatchOnlyWithVerifiedIdempotency(t *testing.T) {
	action, err := ClassifyEffectRecovery(RecoverableEffect{ID: "e1", State: EffectUnknown, IdempotencyKey: "k1", IdempotencyVerified: true})
	if err != nil || action != RecoveryDispatch {
		t.Fatalf("expected safe redispatch, got action=%s err=%v", action, err)
	}
}
