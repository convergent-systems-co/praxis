package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestEffectLifecyclePersistsDispatchAndUnknownOutcomeAcrossRestart(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 14, 7, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db)
	if _, err := db.ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES('cmd-effect','test','1','a','agent','scope','corr','{}','committed',?)`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO effects(effect_id,command_id,action_intent_digest,target_adapter,state,request_payload,created_at,updated_at,idempotency_key) VALUES('effect-1','cmd-effect','sha256:intent','fixture','pending','{}',?,?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), "effect-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkEffectDispatched(ctx, "effect-1", now); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkEffectOutcome(ctx, "effect-1", EffectUnknown, nil, []byte(`{"reason":"connection-lost"}`), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = New(db)
	recoverable, err := store.RecoverableEffects(ctx)
	if err != nil || len(recoverable) != 1 || recoverable[0].State != EffectUnknown {
		t.Fatalf("unknown outcome was not durable across restart: %+v %v", recoverable, err)
	}
	// The recovery classifier is independent of process-local dispatcher state
	// and refuses to treat an unknown result as success.
	if action, err := ClassifyEffectRecovery(RecoverableEffect{ID: "effect-1", State: EffectUnknown}); err == nil || action != RecoveryFailClosed {
		t.Fatalf("unknown outcome must fail closed without reconciliation: %s %v", action, err)
	}
	if err := store.ReconcileEffect(ctx, "effect-1", EffectSucceeded, nil, now.Add(2*time.Second)); err == nil {
		t.Fatal("reconciliation without evidence must fail closed")
	}
	if err := store.ReconcileEffect(ctx, "effect-1", EffectSucceeded, []byte(`{"external_id":"x"}`), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
}
