package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
	"os/user"
)

// Repair 5: Review #5's N14 and N15 as permanent regressions (asserting
// refusal), whole-database rollback against the forward authority anchor, and
// the governed re-anchor command, all through the production constructors and
// the real gate controller.

// gateAtDecision drives the fixture to an approved gate decision and returns the
// handles the attacks need.
func gateAtDecision(t *testing.T) (*gateHarness, goaldrive.Ledger, contracts.AuthorityRequest, contracts.AuthorityDecision) {
	t.Helper()
	h := newGateHarness(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw, err := h.ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if err != nil || len(raw) != 1 {
		t.Fatalf("fixture producer: %v", err)
	}
	ledger := goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if err := ledger.RecordCompletion(h.ctx, raw[0]); err != nil {
		t.Fatal(err)
	}
	request := h.pendingGate(ledger, "repair5-pending")
	digest, _ := request.Digest()
	if _, err := h.decide(digest, "approve", "finish and reconcile", "DECIDE-APPROVE "+digest+" ALTERNATIVE finish and reconcile"); err != nil {
		t.Fatal(err)
	}
	decision, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return h, ledger, request, decision
}

func revokeDecision(t *testing.T, h *gateHarness, repo goalstore.Repository, request contracts.AuthorityRequest, decision contracts.AuthorityDecision) {
	t.Helper()
	dd, _ := decision.Digest()
	now := time.Now().UTC()
	rev := contracts.AuthorityRevocation{RequestID: request.ID, RequestVersion: request.Version, DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: dd, RevocationRef: "repair5-revoke", RevocationVersion: "1", RevokedBy: h.root.Principal, AuthorityDigest: h.root.AuthorityModelDigest, EffectiveAt: now, Reason: "owner withdrew gate authority"}
	if err := repo.SaveAuthorityRevocation(h.ctx, request.ID, request.Version, rev, now, nil); err != nil {
		t.Fatal(err)
	}
}

// N14 (permanent): delete the revocation and replay the byte-exact earlier
// liveness row, restart, and drive: no new completion.
func TestRepair5N14ReplayedLivenessDoesNotMintACompletionAfterRestart(t *testing.T) {
	h, ledger, request, decision := gateAtDecision(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	var ns, id, version, digest, sens, profile, created string
	var envelope []byte
	var expires *string
	if err := db.QueryRowContext(h.ctx, `SELECT namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace='authority_decision_live' AND object_id=? AND object_version=?`, request.ID, request.Version).Scan(&ns, &id, &version, &digest, &sens, &profile, &envelope, &created, &expires); err != nil {
		t.Fatal(err)
	}
	revokeDecision(t, h, repo, request, decision)
	before, _ := ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	if _, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE namespace='authority_revocation' AND object_id=? AND object_version=?`, request.ID, request.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(h.ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, ns, id, version, digest, sens, profile, envelope, created, expires); err != nil {
		t.Fatal(err)
	}
	db.Close()
	repo, db, err = openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, goalstore.ErrAuthorityRetired) {
		t.Fatalf("delete plus replay restored a revoked decision: %v", err)
	}
	baseline, _ := repo.Load(h.ctx, repairGoalID, "2", time.Now().UTC())
	h.successor = baseline
	ledger = goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	if _, err := h.drive(ledger, "repair5-after-replay"); errors.Is(err, goaldrive.ErrHumanAuthorityGate) || err == nil {
		t.Fatalf("a fresh controller acted on replayed authority: %v", err)
	}
	if after, _ := ledger.LoadCompletions(h.ctx, repairGoalID, "2"); len(after) != len(before) {
		t.Fatalf("a new completion was minted: %d -> %d", len(before), len(after))
	}
}

// N15 (permanent): delete the classification row, the safety proposal and the
// attached generation; the stripped import is refused and no worker runs.
func TestRepair5N15ErasingClassificationSourcesDoesNotDowngrade(t *testing.T) {
	h := newGateHarness(t)
	_, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	downgraded := strippedGeneration(h, "3")
	downgraded.PredecessorDigest = h.baseline.Digest
	downgraded.Digest = ""
	path := writeBaselineImportDocument(t, h.dir, "repair5-erasure.json", downgraded, "")
	h.removeActivation()
	for _, q := range []string{
		`DELETE FROM secure_blobs WHERE namespace='goal_safety_classification'`,
		`DELETE FROM secure_blobs WHERE namespace='work_plan_proposal'`,
		`DELETE FROM secure_blobs WHERE namespace='goal_baseline' AND object_version='2'`,
	} {
		if _, err := db.ExecContext(h.ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if _, err := h.run("goals-lifecycle", "--operation=import", "--input="+path); err == nil {
		t.Fatal("the stripped import succeeded after safety-history erasure")
	}
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if classified, err := repo.GoalSafetyClassified(h.ctx, repairGoalID); err != nil || !classified {
		t.Fatalf("the erased Goal read as legacy: %v %v", classified, err)
	}
}

// Whole-database rollback, restart, status, governed recovery.
func TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony(t *testing.T) {
	h, ledger, request, decision := gateAtDecision(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := h.dir + "/backup.db"
	if _, err := db.ExecContext(h.ctx, `VACUUM INTO '`+snapshot+`'`); err != nil {
		t.Fatal(err)
	}
	revokeDecision(t, h, repo, request, decision)
	db.Close()
	dbPath := h.env("PRAXIS_DB")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(dbPath + suffix)
	}
	in, _ := os.Open(snapshot)
	out, _ := os.Create(dbPath)
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()

	repo, db, err = openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, goalstore.ErrGovernanceRolledBack) {
		t.Fatalf("the restored (earlier, authentic) store was consumable: %v", err)
	}
	ledger = goaldrive.Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	before, _ := ledger.LoadCompletions(h.ctx, repairGoalID, "2")
	baseline, err := repo.Load(h.ctx, repairGoalID, "2", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	h.successor = baseline
	if _, err := h.drive(ledger, "repair5-rolled-back"); errors.Is(err, goaldrive.ErrHumanAuthorityGate) || err == nil {
		t.Fatalf("a controller acted on a rolled-back store: %v", err)
	}
	if after, _ := ledger.LoadCompletions(h.ctx, repairGoalID, "2"); len(after) != len(before) {
		t.Fatalf("a completion was minted from a rolled-back store")
	}

	// status is read-only and says so
	var statusOut bytes.Buffer
	if err := runGovernanceStatus(nil, h.env, &statusOut); err != nil {
		t.Fatal(err)
	}
	var status map[string]any
	if err := json.Unmarshal(statusOut.Bytes(), &status); err != nil || status["relation"] != "behind" || status["consumable"] != false {
		t.Fatalf("status: %s %v", statusOut.String(), err)
	}

	// the ceremony refuses: non-interactive, wrong typed text, wrong OS user.
	// Each is offered the CORRECT digest-bound confirmation where that is not
	// what is being tested, so only the rule under test can refuse it.
	plan, err := repo.PlanReanchor(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	correct := "REANCHOR " + plan.Digest() + "\n"
	if err := runGovernanceReanchorWithTerminal(nil, h.env, strings.NewReader(correct), io.Discard, false); err == nil {
		t.Fatal("a non-interactive re-anchor must refuse even with the correct confirmation")
	}
	if err := runGovernanceReanchorWithTerminal(nil, h.env, strings.NewReader("REANCHOR yes\n"), io.Discard, true); err == nil {
		t.Fatal("a re-anchor without the digest-bound confirmation must refuse")
	}
	original := authenticatedOSUser
	authenticatedOSUser = func() (*user.User, error) { return &user.User{Username: "someone-else"}, nil }
	if err := runGovernanceReanchorWithTerminal(nil, h.env, strings.NewReader(correct), io.Discard, true); err == nil {
		t.Fatal("a re-anchor by another OS user must refuse even with the correct confirmation")
	}
	authenticatedOSUser = original
	if st, err := repo.GovernanceStatus(h.ctx); err != nil || st.Relation != "behind" {
		t.Fatalf("no refused ceremony may change anything: %+v %v", st, err)
	}

	// the governed ceremony, with the exact typed confirmation
	var ceremonyOut bytes.Buffer
	if err := runGovernanceReanchorWithTerminal([]string{"--classify-goal", "goal:lost-in-the-interval"}, h.env, strings.NewReader("REANCHOR "+plan.Digest()+" CLASSIFY goal:lost-in-the-interval\n"), &ceremonyOut, true); err != nil {
		t.Fatalf("%v\n%s", err, ceremonyOut.String())
	}
	repo2, db2, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if _, err := repo2.LoadCurrentInstallationRoot(h.ctx, repo2.InstallationDigest, time.Now().UTC()); err != nil {
		t.Fatalf("the attested root must be current after recovery: %v", err)
	}
	if _, err := repo2.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); err == nil {
		t.Fatal("recovery made a pre-recovery decision current")
	}
	if ok, err := repo2.GoalSafetyClassified(h.ctx, "goal:lost-in-the-interval"); err != nil || !ok {
		t.Fatalf("an owner-named classification must hold: %v %v", ok, err)
	}
	statusOut.Reset()
	if err := runGovernanceStatus(nil, h.env, &statusOut); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(statusOut.Bytes(), &status); err != nil || status["relation"] != "consistent" || len(status["reanchors"].([]any)) != 1 {
		t.Fatalf("the recovery must be durable and inspectable: %s", statusOut.String())
	}
}

// A missing anchor refuses every governed consumption, and the same ceremony
// recovers it.
func TestRepair5MissingAnchorFailsClosedAndTheCeremonyRecoversIt(t *testing.T) {
	h, _, request, _ := gateAtDecision(t)
	repo, db, err := openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	installation := repo.InstallationDigest
	saved := testGovernanceAnchor.Snapshot(installation)
	t.Cleanup(func() { testGovernanceAnchor.Restore(installation, saved) })
	testGovernanceAnchor.Restore(installation, nil)
	repo, db, err = openGovernedRepository(h.ctx, h.env)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := repo.LoadAuthorityDecision(h.ctx, request.ID, request.Version, time.Now().UTC()); !errors.Is(err, goalstore.ErrGovernanceAnchorMissing) {
		t.Fatalf("a missing anchor must refuse: %v", err)
	}
	plan, err := repo.PlanReanchor(h.ctx)
	if err != nil || plan.Cause != "missing" {
		t.Fatalf("%+v %v", plan, err)
	}
	if err := runGovernanceReanchorWithTerminal(nil, h.env, strings.NewReader("REANCHOR "+plan.Digest()+"\n"), io.Discard, true); err != nil {
		t.Fatal(err)
	}
	if st, err := testGovernanceAnchor.Load(h.ctx, installation); err != nil || st == (faa.State{}) {
		t.Fatalf("the anchor was not recreated: %v", err)
	}
}

// Deterministic qualification artifact for the classification-erasure class:
// the FULL powerset over the eight Goal-attributable evidence rows of the gate
// fixture (256 states, keyless SQL deletes, restart each time).
//
//   - Anchor detached (the in-store derivation on its own): exactly ONE subset
//     makes the governed Goal read as legacy, deleting all eight. That state is
//     an honest pre-governance state; distinguishing it needs the anchor.
//   - Anchored: NO subset does. The anchored classification fact survives every
//     deletion of every evidence row.
//
// It is exhaustive for "which combinations of deletions of Goal-attributable
// governance rows convert a governed Goal into a legacy one". It is not a
// powerset over the whole database; rows that are not Goal-attributable evidence
// cannot change this Goal's classification.
var repair5EvidenceRows = []struct{ name, where string }{
	{"class", `namespace='goal_safety_classification'`},
	{"proposal", `namespace='work_plan_proposal'`},
	{"review", `namespace='work_plan_review'`},
	{"acceptance", `namespace='work_plan_acceptance'`},
	{"request", `namespace='authority_request'`},
	{"decision", `namespace='authority_decision'`},
	{"baseline2", `namespace='goal_baseline' AND object_version='2'`},
	{"seal", `namespace='goal_completion_seal'`},
}

func TestRepair5EvidencePowersetClassificationQualification(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("exhaustive 256-state enumeration: single-goroutine, run in the ordinary focused pass")
	}
	for _, mode := range []string{"anchor detached (in-store derivation only)", "anchored"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			n := len(repair5EvidenceRows)
			var legacy []string
			for mask := 0; mask < 1<<n; mask++ {
				h := newGateHarness(t)
				_, db, err := openGovernedRepository(h.ctx, h.env)
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for i := 0; i < n; i++ {
					if mask&(1<<i) != 0 {
						names = append(names, repair5EvidenceRows[i].name)
						if _, err := db.ExecContext(h.ctx, `DELETE FROM secure_blobs WHERE `+repair5EvidenceRows[i].where); err != nil {
							t.Fatal(err)
						}
					}
				}
				db.Close()
				repo, db, err := openGovernedRepository(h.ctx, h.env)
				if err != nil {
					t.Fatal(err)
				}
				if mode != "anchored" {
					repo.FAA = nil
				}
				classified, cerr := repo.GoalSafetyClassified(h.ctx, repairGoalID)
				db.Close()
				if cerr != nil {
					t.Fatalf("subset %v: %v", names, cerr)
				}
				if !classified {
					legacy = append(legacy, strings.Join(names, "+"))
				}
			}
			switch mode {
			case "anchored":
				if len(legacy) != 0 {
					t.Fatalf("%d of 256 subsets made a governed Goal read as legacy while anchored: %v", len(legacy), legacy)
				}
			default:
				if len(legacy) != 1 || legacy[0] != "class+proposal+review+acceptance+request+decision+baseline2+seal" {
					t.Fatalf("in-store derivation: want exactly the full-erasure subset, got %d: %v", len(legacy), legacy)
				}
			}
		})
	}
}
