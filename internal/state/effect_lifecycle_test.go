package state

import (
	"context"
	"errors"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
	"path/filepath"
	"testing"
	"time"
)

func TestCommitObservationResolutionIsAtomic(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := New(db)
	if _, err = db.ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES('effect-cmd','goals-publication-recovery.step','1','publisher','publisher','scope','execution','{}','committed',?)`, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO effects(effect_id,command_id,action_intent_digest,target_adapter,state,attempts,request_payload,observed_result,created_at,updated_at) VALUES('effect','effect-cmd','sha256:intent','goals-recovery-github','unknown',1,'{}','{}',?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "publisher", Kind: "publisher"}
	cmd := CommandRecord{ID: "resolution", Type: "resolve", Version: "1", Actor: actor, Scope: "effect", CorrelationID: "execution", Payload: []byte(`{"x":1}`), CreatedAt: now}
	ev := EventRecord{ID: "resolution", AggregateID: "resolution", AggregateType: "resolution", AggregateVersion: 1, Type: "resolved", Version: "1", Actor: actor, CommandID: "resolution", CorrelationID: "execution", Payload: []byte(`{"x":1}`), CreatedAt: now}
	if err := s.CommitObservationResolution(ctx, cmd, ev, "effect", []byte(`{"evidence":1}`), now); err != nil {
		t.Fatal(err)
	}
	var st string
	var obs []byte
	if err := db.QueryRowContext(ctx, `SELECT state,observed_result FROM effects WHERE effect_id='effect'`).Scan(&st, &obs); err != nil {
		t.Fatal(err)
	}
	if st != "succeeded" || string(obs) != "{}" {
		t.Fatalf("resolution did not preserve observation: %s %s", st, obs)
	}
}

func TestCommitObservationResolutionRollsBackAtEveryBoundary(t *testing.T) {
	for _, stage := range []string{"before-command", "after-command", "after-event", "before-effect", "after-effect", "before-commit"} {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now().UTC().Truncate(time.Second)
			db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s := New(db)
			if _, err = db.ExecContext(ctx, `INSERT INTO commands(command_id,command_type,command_version,actor_id,actor_kind,scope,correlation_id,payload,status,created_at) VALUES('effect-cmd','step','1','publisher','publisher','scope','execution','{}','committed',?)`, now.Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			if _, err = db.ExecContext(ctx, `INSERT INTO effects(effect_id,command_id,action_intent_digest,target_adapter,state,attempts,request_payload,observed_result,created_at,updated_at) VALUES('effect','effect-cmd','sha256:intent','goals-recovery-github','unknown',1,'{}','original',?,?)`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			actor := contracts.PrincipalRef{ID: "publisher", Kind: "publisher"}
			cmd := CommandRecord{ID: "resolution", Type: "resolve", Version: "1", Actor: actor, Scope: "effect", CorrelationID: "execution", Payload: []byte(`{}`), CreatedAt: now}
			ev := EventRecord{ID: "resolution", AggregateID: "resolution", AggregateType: "resolution", AggregateVersion: 1, Type: "resolved", Version: "1", Actor: actor, CommandID: "resolution", CorrelationID: "execution", Payload: []byte(`{}`), CreatedAt: now}
			s.observationResolutionFault = func(got string) error {
				if got == stage {
					return errors.New("injected failure")
				}
				return nil
			}
			if err := s.CommitObservationResolution(ctx, cmd, ev, "effect", []byte(`{"e":1}`), now); err == nil {
				t.Fatal("failure injection did not fail")
			}
			var n int
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM commands WHERE command_id='resolution'`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatal("resolution command survived rollback")
			}
			if err := db.QueryRowContext(ctx, `SELECT count(*) FROM events WHERE event_id='resolution'`).Scan(&n); err != nil {
				t.Fatal(err)
			}
			if n != 0 {
				t.Fatal("resolution event survived rollback")
			}
			var st string
			var attempts int
			var obs string
			if err := db.QueryRowContext(ctx, `SELECT state,attempts,observed_result FROM effects WHERE effect_id='effect'`).Scan(&st, &attempts, &obs); err != nil {
				t.Fatal(err)
			}
			if st != "unknown" || attempts != 1 || obs != "original" {
				t.Fatalf("effect changed after rollback: %s %d %s", st, attempts, obs)
			}
		})
	}
}

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
