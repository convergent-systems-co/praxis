package main

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Repair 4 / I12 (deletion-monotonicity). Review #4 showed that a keyless
// writer who can only DELETE rows of the SQLite file could widen authority
// (N8: restore a revoked decision; N9: erase a safety classification). These
// regressions are the preserved Review #4 probes with the assertions inverted
// to the required fail-closed behaviour, plus a whole-store single-row
// deletion sweep.

// revokedGateFixture drives the fixture to a pending gate, approves it, then
// revokes the decision. It returns the open handles and the gate request.
func repair4RevokedGate(t *testing.T) (*gateHarness, contracts.AuthorityRequest, contracts.AuthorityDecision) {
	t.Helper()
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	raw, err := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil || len(raw) != 1 {
		t.Fatalf("fixture producer: %v %+v", err, raw)
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if err := ledger.RecordCompletion(h.ctx, raw[0]); err != nil {
		t.Fatal(err)
	}
	request := h.pendingGate(ledger, "repair4-pending")
	digest, _ := request.Digest()
	if _, err := h.decide(digest, "approve", "finish and reconcile", "DECIDE-APPROVE "+digest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatal(err)
	}
	decision, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	decisionDigest, _ := decision.Digest()
	now := time.Now().UTC()
	rev := contracts.AuthorityRevocation{RequestID: request.ID, RequestVersion: request.Version, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest, RevocationRef: "repair4-revoke", RevocationVersion: "1", RevokedBy: h.root.Principal, AuthorityDigest: h.root.AuthorityModelDigest, EffectiveAt: now, Reason: "owner withdrew gate authority"}
	if err := repo.SaveAuthorityRevocation(h.ctx, request.ID, request.Version, rev, now, nil); err != nil {
		t.Fatal(err)
	}
	return h, request, decision
}

// N8: deleting the revocation row must not restore the revoked authority, and
// the fresh controller must not mint a new gate completion.
func TestRepair4N8RevocationRowDeletionDoesNotRestoreAuthority(t *testing.T) {
	h, request, original := repair4RevokedGate(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, goalstore.ErrAuthorityDecisionRevoked) {
		t.Fatalf("control: revoked decision must refuse: %v", err)
	}
	result, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace='authority_revocation' AND object_id=? AND object_version=?`, request.ID, request.Version)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("delete: rows=%d err=%v", n, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	repo, db, err = openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, goalstore.ErrAuthorityDecisionNotLive) || !errors.Is(err, goalstore.ErrAuthorityDecisionRevoked) {
		t.Fatalf("revocation deletion restored authority (or the wrong refusal): %v", err)
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	baseline, err := repo.Load(h.ctx, repairGoalID, "2", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	h.successor = baseline
	if _, err := h.drive(ledger, "repair4-after-delete"); errors.Is(err, goaldrive.ErrHumanAuthorityGate) || err == nil {
		t.Fatalf("a fresh controller acted on restored authority: %v", err)
	}
	after, err := ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil || len(after) != 1 {
		t.Fatalf("a new completion was minted after revocation deletion: %v %d", err, len(after))
	}
	// Idempotent re-save of the same decision must not resurrect it either.
	if err := repo.SaveAuthorityDecision(h.ctx, request.ID, request.Version, original, time.Now().UTC(), nil); err == nil {
		t.Fatal("re-saving a retired decision must refuse")
	}
}

// N8 (positive liveness): deleting the decision liveness row alone, without any
// revocation, fails closed: a decision whose positive record is gone grants
// nothing.
func TestRepair4DecisionLivenessRowDeletionFailsClosed(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	raw, err := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil || len(raw) != 1 {
		t.Fatalf("fixture: %v %+v", err, raw)
	}
	if err := ledger.RecordCompletion(h.ctx, raw[0]); err != nil {
		t.Fatal(err)
	}
	request := h.pendingGate(ledger, "repair4-live")
	digest, _ := request.Digest()
	if _, err := h.decide(digest, "approve", "finish and reconcile", "DECIDE-APPROVE "+digest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("control: approved decision must load: %v", err)
	}
	result, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace='authority_decision_live' AND object_id=? AND object_version=?`, request.ID, request.Version)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("delete: rows=%d err=%v", n, err)
	}
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, goalstore.ErrAuthorityDecisionNotLive) {
		t.Fatalf("decision without a liveness record must not grant authority: %v", err)
	}
	_ = db.Close()
}

// N9: deleting the classification row must not turn a safety-bearing Goal back
// into a legacy one: the stripped import stays refused and the classification
// is re-derived from the surviving safety-bearing generations and proposals.
func TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	path := writeBaselineImportDocument(t, h.dir, "repair4-stripped.json", strippedGeneration(h, "3"), "")
	h.removeActivation()
	if _, err := h.run("goals-lifecycle", "--operation=import", "--input="+path); err == nil {
		t.Fatal("control: import allowed before delete")
	}
	result, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace='goal_safety_classification' AND object_id=?`, repairGoalID)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("delete rows=%d err=%v", n, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.run("goals-lifecycle", "--operation=import", "--input="+path); err == nil {
		t.Fatal("classification deletion allowed a stripped import")
	}
	repo, db, err = openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if classified, err := repo.GoalSafetyClassified(h.ctx, repairGoalID); err != nil || !classified {
		t.Fatalf("classification must be re-derived after deletion: %v %v", classified, err)
	}
	if _, err := repo.Load(h.ctx, repairGoalID, "3", time.Now().UTC()); err == nil {
		t.Fatal("no stripped generation may exist")
	}
}

// deletionSweepRows returns the identities of every sealed row of a fresh
// fixture, in stable order.
func repair4Rows(t *testing.T, h *gateHarness) []string {
	t.Helper()
	_, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.QueryContext(h.ctx, `SELECT namespace||'|'||object_id||'|'||object_version FROM secure_blobs ORDER BY namespace,object_id,object_version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	return keys
}

func repair4DeleteRow(t *testing.T, h *gateHarness, index int) string {
	t.Helper()
	keys := repair4Rows(t, h)
	if index >= len(keys) {
		return ""
	}
	_, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var ns, id, ver string
	if err := db.QueryRowContext(h.ctx, `SELECT namespace,object_id,object_version FROM secure_blobs ORDER BY namespace,object_id,object_version LIMIT 1 OFFSET ?`, index).Scan(&ns, &id, &ver); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, ns, id, ver); err != nil {
		t.Fatal(err)
	}
	return ns + "|" + id + "|" + ver
}

// I12 sweep, revocation: with a revoked decision, deleting ANY single sealed
// row (keyless SQL DELETE) never makes the decision loadable again.
func TestRepair4SweepSingleRowDeletionNeverRestoresRevokedDecision(t *testing.T) {
	probe, _, _ := repair4RevokedGate(t)
	total := len(repair4Rows(t, probe))
	if total < 8 {
		t.Fatalf("fixture too small to be a meaningful sweep: %d rows", total)
	}
	for i := 0; i < total; i++ {
		i := i
		t.Run(fmt.Sprintf("row-%02d", i), func(t *testing.T) {
			h, request, _ := repair4RevokedGate(t)
			deleted := repair4DeleteRow(t, h, i)
			repo, db, err := openGovernedRepository(h.ctx, h.env)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); err == nil {
				t.Fatalf("deleting %s restored a revoked decision", deleted)
			}
		})
	}
}

// I12 sweep, classification: deleting ANY single sealed row never makes the
// classified Goal read as unclassified.
func TestRepair4SweepSingleRowDeletionNeverUnclassifiesGoal(t *testing.T) {
	probe := newGateHarness(t)
	total := len(repair4Rows(t, probe))
	for i := 0; i < total; i++ {
		i := i
		t.Run(fmt.Sprintf("row-%02d", i), func(t *testing.T) {
			h := newGateHarness(t)
			deleted := repair4DeleteRow(t, h, i)
			repo, db, err := openGovernedRepository(h.ctx, h.env)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			classified, err := repo.GoalSafetyClassified(h.ctx, repairGoalID)
			if err == nil && !classified {
				t.Fatalf("deleting %s un-classified the Goal", deleted)
			}
		})
	}
}

// I12 sweep, plaintext ledger: with a revoked decision, deleting ANY single
// row of any non-sealed table (the event ledger, the admission fence, activity,
// settlement) never lets a fresh controller mint a new gate completion. Those
// tables are unauthenticated by design (the accepted trust model lets a
// keyless writer alter them); the property is that no such deletion widens
// authority, because every consequence they gate is re-established from sealed
// state at the point of effect (I10, I11).
func TestRepair4SweepSingleLedgerRowDeletionNeverMintsCompletionAfterRevocation(t *testing.T) {
	probe, _, _ := repair4RevokedGate(t)
	_, db, err := openGovernedRepository(probe.ctx, probe.env)
	if err != nil {
		t.Fatal(err)
	}
	type rowRef struct {
		table string
		rowid int64
	}
	var refs []rowRef
	tables, err := db.QueryContext(probe.ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name <> 'secure_blobs'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	tables.Close()
	for _, name := range names {
		rows, err := db.QueryContext(probe.ctx, `SELECT rowid FROM "`+name+`" ORDER BY rowid`)
		if err != nil {
			continue // WITHOUT ROWID or virtual table: not sweepable this way
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			refs = append(refs, rowRef{name, id})
		}
		rows.Close()
	}
	db.Close()
	if len(refs) == 0 {
		t.Fatal("fixture has no plaintext rows to sweep")
	}
	t.Logf("sweeping %d plaintext rows across %d tables", len(refs), len(names))
	for i := range refs {
		i := i
		t.Run(fmt.Sprintf("row-%02d", i), func(t *testing.T) {
			h, request, _ := repair4RevokedGate(t)
			_, db, err := openGovernedRepository(h.ctx, h.env)
			if err != nil {
				t.Fatal(err)
			}
			// Row identities are stable across identical fixtures; delete the
			// i-th plaintext row of the same table by ordinal position.
			var deleted string
			seen := 0
			for _, name := range names {
				rows, qerr := db.QueryContext(h.ctx, `SELECT rowid FROM "`+name+`" ORDER BY rowid`)
				if qerr != nil {
					continue
				}
				var ids []int64
				for rows.Next() {
					var id int64
					_ = rows.Scan(&id)
					ids = append(ids, id)
				}
				rows.Close()
				for _, id := range ids {
					if seen == i {
						if _, err := db.ExecContext(h.ctx, `DELETE FROM "`+name+`" WHERE rowid=?`, id); err != nil {
							t.Skipf("the store itself refuses deleting a %s row: %v", name, err)
						}
						deleted = name
					}
					seen++
				}
			}
			if deleted == "" {
				t.Skip("fixture row count differs between runs")
			}
			db.Close()
			repo, db, err := openGovernedRepository(h.ctx, h.env)
			if err != nil {
				// The installation refuses to open without that row: fail closed.
				t.Skipf("installation refuses to open after deleting a %s row: %v", deleted, err)
			}
			defer db.Close()
			if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); err == nil {
				t.Fatalf("deleting a %s row restored the revoked decision", deleted)
			}
			ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
			before, _ := ledger.LoadCompletions(h.ctx, repairGoalID, "2")
			baseline, err := repo.Load(h.ctx, repairGoalID, "2", time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			h.successor = baseline
			if _, err := h.drive(ledger, "repair4-ledger-sweep"); errors.Is(err, goaldrive.ErrHumanAuthorityGate) {
				t.Fatalf("deleting a %s row let a fresh controller act on the revoked gate", deleted)
			}
			after, _ := ledger.LoadCompletions(h.ctx, repairGoalID, "2")
			if len(after) > len(before) {
				t.Fatalf("deleting a %s row let the controller mint a new completion (%d -> %d)", deleted, len(before), len(after))
			}
		})
	}
}

// I12 sweep, pairs: with the classification row already deleted, deleting any
// ONE further sealed row still leaves the Goal classified, because the
// classification is derived from at least two independent surviving
// safety-bearing artifacts (the accepted-plan generation and the proposal).
// Deleting all of them is total erasure of the Goal's governance history, the
// documented residual that needs an external monotonic anchor.
func TestRepair4SweepClassificationRowPlusAnyOtherRowStillClassified(t *testing.T) {
	probe := newGateHarness(t)
	total := len(repair4Rows(t, probe))
	for i := 0; i < total; i++ {
		i := i
		t.Run(fmt.Sprintf("row-%02d", i), func(t *testing.T) {
			h := newGateHarness(t)
			_, db, err := openGovernedRepository(h.ctx, h.env)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace='goal_safety_classification' AND object_id=?`, repairGoalID); err != nil {
				t.Fatal(err)
			}
			db.Close()
			deleted := repair4DeleteRow(t, h, i)
			if deleted == "" {
				t.Skip("fixture row count differs between runs")
			}
			repo, db, err := openGovernedRepository(h.ctx, h.env)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			classified, err := repo.GoalSafetyClassified(h.ctx, repairGoalID)
			if err == nil && !classified {
				t.Fatalf("deleting the classification row and %s un-classified the Goal", deleted)
			}
		})
	}
}

// N9 derivation, generation half: another Goal's safety-bearing generation does
// not classify this Goal; the Goal's own surviving generation does once the
// classification row and every proposal are gone; and an unreadable generation
// row of the Goal fails closed instead of reading as unclassified.
func TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed(t *testing.T) {
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Repair 4's in-store derivation is exercised on its own: with the anchored
	// classification fact present it would (correctly) short-circuit first.
	repo.FAA = nil
	own, err := repo.Load(h.ctx, repairGoalID, "2", time.Now().UTC())
	if err != nil || own.WorkPlan == nil || own.WorkPlan.Safety == nil {
		t.Fatalf("fixture must hold a safety-bearing generation: %v", err)
	}
	// Only generations may classify: remove the classification row and every proposal.
	// Every other evidence kind is removed too (the registry consumes them all),
	// so ONLY the surviving generation can classify the Goal.
	for _, q := range []string{`DELETE FROM secure_blobs WHERE namespace IN ('goal_safety_classification','work_plan_proposal','work_plan_review','work_plan_acceptance','authority_request','authority_decision','goal_completion_seal')`} {
		if _, err := db.ExecContext(h.ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if classified, err := repo.GoalSafetyClassified(h.ctx, repairGoalID); err != nil || !classified {
		t.Fatalf("the Goal's own surviving generation must classify it: %v %v", classified, err)
	}
	if classified, err := repo.GoalSafetyClassified(h.ctx, "goal:some-other-goal"); err != nil || classified {
		t.Fatalf("another Goal's generation classified an unrelated Goal: %v %v", classified, err)
	}
	if _, err := db.ExecContext(h.ctx, `UPDATE secure_blobs SET envelope_json=json_set(envelope_json,'$.Ciphertext','AAAA') WHERE namespace='goal_baseline' AND object_id=? AND object_version='2'`, repairGoalID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.GoalSafetyKernel(h.ctx, repairGoalID); err == nil {
		t.Fatal("an unreadable generation of the Goal read as unclassified")
	}
}
