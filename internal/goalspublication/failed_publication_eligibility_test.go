package goalspublication

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestFailedPublicationEligibilitySurvivesDurableReadBackAfterRestart proves
// adversarial case 8's "restart/durable read-back preserves the same
// eligibility result" requirement: an effect recorded exactly the way the
// canonical dispatchFailure path records a local pre-dispatch failure
// (CommitTransition to create the pending effect, then MarkEffectOutcome
// exactly as Execute's error handling calls it) must still evaluate as
// eligible after the database connection is closed and reopened — i.e. the
// eligibility predicate reads only what is actually durable, not anything
// transient (in-memory Go struct fields, driver caching, etc.).
func TestFailedPublicationEligibilitySurvivesDurableReadBackAfterRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "eligibility.db")
	db, err := state.OpenSQLite(ctx, path)
	must(t, err)
	store := state.New(db)

	at := time.Now().UTC()
	actor := contracts.PrincipalRef{ID: "publisher:praxis-first-party", Kind: "publisher"}
	const effectID = "goals-publication-recovery:sha256:test:publish"
	cmd := state.CommandRecord{ID: effectID, Type: "goals-publication-recovery.step", Version: "1", Actor: actor, Scope: "goal:test", CorrelationID: "goals-publication-recovery:sha256:test", Payload: []byte(`{}`), CreatedAt: at}
	ev := state.EventRecord{ID: effectID, AggregateID: "goals-publication-recovery:sha256:test", AggregateType: "goals-publication-recovery", AggregateVersion: 1, Type: "goals-publication-recovery.step-admitted", Version: "1", Actor: actor, CommandID: effectID, CorrelationID: "goals-publication-recovery:sha256:test", TrustClass: contracts.TrustPolicy, Payload: []byte(`{}`), CreatedAt: at}
	ef := state.EffectRecord{ID: effectID, CommandID: effectID, ActionIntentDigest: "sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"[:64], TargetAdapter: "goals-recovery-github", TargetPrincipal: actor.ID, State: string(state.EffectPending), RequestPayload: []byte(`{}`), CreatedAt: at, UpdatedAt: at}
	must(t, store.CommitTransition(ctx, cmd, 0, ev, "", "", &ef))

	// Exactly the claim step Execute performs before Coordinator.Commit:
	// state='dispatched', attempts=attempts+1.
	res, err := db.ExecContext(ctx, `UPDATE effects SET state='dispatched',attempts=attempts+1,updated_at=? WHERE effect_id=? AND state='pending'`, at.Format(time.RFC3339Nano), effectID)
	must(t, err)
	n, err := res.RowsAffected()
	must(t, err)
	if n != 1 {
		t.Fatalf("expected exactly one claimed effect, got %d", n)
	}

	// Exactly the evidence shape dispatchFailure.Evidence() + Execute's
	// error-handling MarkEffectOutcome call produce for a not_started
	// pre-dispatch revalidation rejection.
	evidence := []byte(`{"version":"1","process":"not_started","class":"local_pre_dispatch_failure"}`)
	must(t, store.MarkEffectOutcome(ctx, effectID, state.EffectFailed, evidence, nil, at))
	must(t, db.Close())

	// Restart: reopen the database as a fresh process would.
	db2, err := state.OpenSQLite(ctx, path)
	must(t, err)
	defer db2.Close()
	var st string
	var attempts int
	var observed, reconciliation []byte
	must(t, db2.QueryRowContext(ctx, `SELECT state,attempts,COALESCE(observed_result,''),COALESCE(reconciliation_evidence,'') FROM effects WHERE effect_id=?`, effectID).Scan(&st, &attempts, &observed, &reconciliation))
	if st != string(state.EffectFailed) || attempts != 1 {
		t.Fatalf("unexpected durable state/attempts after restart: state=%s attempts=%d", st, attempts)
	}
	if err := contracts.ValidateExactLocalPreDispatchFailureEvidence(observed, reconciliation); err != nil {
		t.Fatalf("eligibility did not survive durable read-back after restart: %v", err)
	}
}
