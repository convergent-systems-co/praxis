package goalstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/internal/faa/faatest"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// Forward Authority Anchor tests (I13 temporal authority, I14 subject
// continuity). Each keyless attack below is performed with SQL against the file
// and the anchor is never touched, exactly the adversary of Review #4 and #5.

type anchored struct {
	repo   Repository
	store  *state.Store
	anchor *faatest.Memory
	path   string
}

func anchoredFixture(t *testing.T) anchored {
	t.Helper()
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, store := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	mem := faatest.NewMemory()
	repo.FAA = mem
	if err := repo.InitializeGovernanceAnchor(context.Background()); err != nil {
		t.Fatal(err)
	}
	return anchored{repo: repo, store: store, anchor: mem, path: path}
}

// withDecision drives an approved unprotected decision on an anchored repository.
func (a anchored) withDecision(t *testing.T) approvedDecision {
	t.Helper()
	d := approveUnprotectedAcceptanceOn(t, a.repo, a.store)
	return d
}

// reopen closes nothing: it opens a second repository over the same file and
// anchor, the state a restart presents.
func (a anchored) reopen(t *testing.T) Repository {
	t.Helper()
	repo, _ := repoFixtureAt(t, a.path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	repo.FAA = a.anchor
	return repo
}

func (a anchored) revoke(t *testing.T, d approvedDecision) {
	t.Helper()
	digest, _ := d.decision.Digest()
	revocation := contracts.AuthorityRevocation{RequestID: d.request.ID, RequestVersion: d.request.Version, DecisionRef: d.decision.DecisionRef, DecisionVersion: d.decision.DecisionVersion, DecisionDigest: digest, RevocationRef: "revocation-faa-1", RevocationVersion: "1", RevokedBy: d.decision.DecidedBy, AuthorityDigest: "sha256:operator-revocation", EffectiveAt: time.Now().UTC(), Reason: "withdrawn"}
	if err := a.repo.SaveAuthorityRevocation(context.Background(), d.request.ID, d.request.Version, revocation, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
}

func snapshotRow(t *testing.T, store *state.Store, namespace, id, version string) []any {
	t.Helper()
	var ns, oid, ver, digest, sens, profile, created string
	var envelope []byte
	var expires *string
	if err := store.DB().QueryRow(`SELECT namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at FROM secure_blobs WHERE namespace=? AND object_id=? AND object_version=?`, namespace, id, version).Scan(&ns, &oid, &ver, &digest, &sens, &profile, &envelope, &created, &expires); err != nil {
		t.Fatal(err)
	}
	return []any{ns, oid, ver, digest, sens, profile, envelope, created, expires}
}

func replayRow(t *testing.T, store *state.Store, row []any) {
	t.Helper()
	if _, err := store.DB().Exec(`INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, row...); err != nil {
		t.Fatal(err)
	}
}

// N14: delete the revocation row and replay the byte-exact earlier liveness row.
func TestFAAReplayOfACopiedLivenessRowDoesNotRestoreARevokedDecision(t *testing.T) {
	a := anchoredFixture(t)
	d := a.withDecision(t)
	ctx := context.Background()
	if _, err := a.repo.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("control: %v", err)
	}
	copied := snapshotRow(t, a.store, state.AuthorityDecisionLiveNamespace, d.request.ID, d.request.Version)
	a.revoke(t, d)
	keylessDelete(t, a.store, state.AuthorityRevocationNamespace, d.request.ID, d.request.Version)
	replayRow(t, a.store, copied)
	restarted := a.reopen(t)
	if _, err := restarted.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrAuthorityRetired) || !errors.Is(err, ErrAuthorityDecisionRevoked) {
		t.Fatalf("delete plus replay restored a revoked decision after restart: %v", err)
	}
}

// Whole-database rollback: the file is replaced with a byte-consistent earlier
// copy. The earlier state is internally authentic and shows no revocation, but
// it is behind the anchor and nothing governed is consumable.
func TestFAAWholeDatabaseRollbackFailsClosed(t *testing.T) {
	a := anchoredFixture(t)
	d := a.withDecision(t)
	ctx := context.Background()
	snapshot := filepath.Join(t.TempDir(), "earlier.db")
	if _, err := a.store.DB().Exec(`VACUUM INTO '` + snapshot + `'`); err != nil {
		t.Fatal(err)
	}
	a.revoke(t, d)
	if err := a.store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(a.path + suffix)
	}
	in, err := os.Open(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()
	restored := a.reopen(t)
	if _, err := restored.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) || !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("an earlier authentic store was consumable: %v", err)
	}
	if err := restored.ValidateAuthorityGeneration(ctx, d.decision, time.Now().UTC()); !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("generation validation consumed a rolled-back store: %v", err)
	}
	if _, _, err := restored.GoalSafetyKernel(ctx, "goal-1"); !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("classification consumed a rolled-back store: %v", err)
	}
}

// Prefix truncation, erasure, and damage to the fact chain.
func TestFAAFactChainTruncationErasureAndDamageFailClosed(t *testing.T) {
	ctx := context.Background()
	build := func(t *testing.T) (anchored, approvedDecision) {
		a := anchoredFixture(t)
		d := a.withDecision(t)
		a.revoke(t, d)
		if err := a.repo.anchorGoalClassified(ctx, "goal-1", contracts.WorkPlanSafetyKernelVersion, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		return a, d
	}
	cases := []struct {
		name string
		do   func(t *testing.T, a anchored)
		want error
	}{
		{"delete the last fact", func(t *testing.T, a anchored) {
			keylessDelete(t, a.store, state.GovernanceFactNamespace, state.GovernanceFactID(2), "1")
		}, ErrGovernanceRolledBack},
		{"delete every fact", func(t *testing.T, a anchored) {
			if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace='governance_fact'`); err != nil {
				t.Fatal(err)
			}
		}, ErrGovernanceRolledBack},
		{"delete the first fact (a gap)", func(t *testing.T, a anchored) {
			keylessDelete(t, a.store, state.GovernanceFactNamespace, state.GovernanceFactID(1), "1")
		}, ErrGovernanceChainInvalid},
		{"corrupt a fact's ciphertext", func(t *testing.T, a anchored) {
			if _, err := a.store.DB().Exec(`UPDATE secure_blobs SET envelope_json=json_set(envelope_json,'$.Ciphertext','AAAA') WHERE namespace='governance_fact' AND object_id=?`, state.GovernanceFactID(1)); err != nil {
				t.Fatal(err)
			}
		}, ErrGovernanceChainInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, d := build(t)
			if _, err := a.repo.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrAuthorityDecisionRevoked) {
				t.Fatalf("control: %v", err)
			}
			c.do(t, a)
			restarted := a.reopen(t)
			if _, err := restarted.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
			if _, _, err := restarted.GoalSafetyKernel(ctx, "goal-1"); !errors.Is(err, c.want) {
				t.Fatalf("classification: want %v, got %v", c.want, err)
			}
		})
	}
}

// Missing, unreadable, unavailable, foreign, stale, ahead and substituted anchor
// state all refuse.
func TestFAAAnchorStatesFailClosed(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	early := a.anchor.Snapshot(successionTestBootstrap) // the anchor at genesis, before anything happened
	d := a.withDecision(t)
	a.revoke(t, d)
	load := func(r Repository) error {
		_, err := r.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC())
		return err
	}
	good := a.anchor.Snapshot(successionTestBootstrap)
	if err := load(a.reopen(t)); !errors.Is(err, ErrAuthorityDecisionRevoked) {
		t.Fatalf("control: %v", err)
	}
	t.Run("missing", func(t *testing.T) {
		a.anchor.Restore(successionTestBootstrap, nil)
		defer a.anchor.Restore(successionTestBootstrap, good)
		if err := load(a.reopen(t)); !errors.Is(err, ErrGovernanceAnchorMissing) {
			t.Fatal(err)
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		a.anchor.CorruptOnLoad = true
		defer func() { a.anchor.CorruptOnLoad = false }()
		if err := load(a.reopen(t)); !errors.Is(err, ErrGovernanceAnchorUnavailable) {
			t.Fatal(err)
		}
	})
	t.Run("unavailable", func(t *testing.T) {
		a.anchor.FailLoad = true
		defer func() { a.anchor.FailLoad = false }()
		if err := load(a.reopen(t)); !errors.Is(err, ErrGovernanceAnchorUnavailable) {
			t.Fatal(err)
		}
	})
	t.Run("another installation's state", func(t *testing.T) {
		other := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		_, g, _ := faa.NewGenesis(other, "n", time.Now())
		body := []byte(`{"installation":"` + other + `","seq":0,"head":"` + g.Head + `"}`)
		a.anchor.Restore(successionTestBootstrap, body)
		defer a.anchor.Restore(successionTestBootstrap, good)
		if err := load(a.reopen(t)); !errors.Is(err, ErrGovernanceNotFresh) {
			t.Fatal(err)
		}
	})
	t.Run("stale anchor (the anchor itself rolled back)", func(t *testing.T) {
		// The storage-key trust root (A4): whoever can roll the anchor back can
		// do anything the key holder can. What matters is that the mismatch is
		// refused, not silently accepted.
		a.anchor.Restore(successionTestBootstrap, early)
		defer a.anchor.Restore(successionTestBootstrap, good)
		if err := load(a.reopen(t)); !errors.Is(err, ErrGovernanceAhead) {
			t.Fatalf("a store ahead of a stale anchor must refuse: %v", err)
		}
	})
	t.Run("same length, different head (substitution)", func(t *testing.T) {
		body := []byte(`{"installation":"` + successionTestBootstrap + `","seq":1,"head":"sha256:` + string(make([]byte, 0)) + `1111111111111111111111111111111111111111111111111111111111111111"}`)
		a.anchor.Restore(successionTestBootstrap, body)
		defer a.anchor.Restore(successionTestBootstrap, good)
		if err := load(a.reopen(t)); !errors.Is(err, ErrGovernanceUnrelated) {
			t.Fatalf("a different chain of the same length must refuse: %v", err)
		}
	})
	if err := load(a.reopen(t)); !errors.Is(err, ErrAuthorityDecisionRevoked) {
		t.Fatalf("the anchor was not restored after the subtests: %v", err)
	}
}

// N15 (in the store): with the classification fact anchored, no combination of
// deletions of the Goal's evidence un-classifies it; deleting the fact rows is
// itself a rollback of the anchored chain.
func TestFAAClassificationSurvivesErasureOfEveryEvidenceRow(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	if err := a.repo.anchorGoalClassified(ctx, "goal-1", contracts.WorkPlanSafetyKernelVersion, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	for _, namespace := range []string{"goal_safety_classification", "work_plan_proposal", "work_plan_review", "work_plan_acceptance", "authority_request", "authority_decision", "goal_baseline", "goal_completion_seal"} {
		if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=?`, namespace); err != nil {
			t.Fatal(err)
		}
	}
	kernel, classified, err := a.reopen(t).GoalSafetyKernel(ctx, "goal-1")
	if err != nil || !classified || kernel != contracts.WorkPlanSafetyKernelVersion {
		t.Fatalf("erasing every evidence row made a governed Goal legacy: %q %v %v", kernel, classified, err)
	}
	if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace='governance_fact'`); err != nil {
		t.Fatal(err)
	}
	if _, classified, err := a.reopen(t).GoalSafetyKernel(ctx, "goal-1"); classified || !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("erasing the fact chain must refuse, not read as legacy: classified=%v err=%v", classified, err)
	}
}

// Crash boundary 1: the fact committed but the effects did not run. The
// decision is retired by fact; the retry completes the effects.
func TestFAACrashBetweenFactAndEffectsLeavesTheDecisionRetired(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	digest, _ := d.decision.Digest()
	if err := a.repo.anchorDecisionRetired(ctx, d.request.ID, d.request.Version, digest, "revoked", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.reopen(t).LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrAuthorityRetired) {
		t.Fatalf("a retirement fact without its effects must still retire the decision: %v", err)
	}
	a.revoke(t, d) // the owner's retry
	if _, err := a.reopen(t).LoadAuthorityRevocation(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("the retry did not complete the revocation: %v", err)
	}
	facts, _ := a.store.ListGovernanceFactRecords(ctx)
	if len(facts) != 2 { // genesis + the one retirement
		t.Fatalf("the retry must not append a second retirement fact: %d", len(facts))
	}
}

// Crash boundary 2: the anchor advanced and the process died before the store
// committed. The anchor is ahead; the store refuses to be consumed until a
// governed re-anchor.
func TestFAACrashAfterAnchorAdvanceBeforeCommitStrandsFailClosed(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	// A writer that dies after Set: advance the anchor by hand.
	cur, err := a.anchor.Load(ctx, successionTestBootstrap)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := faa.Seal(faa.Fact{Seq: cur.Seq + 1, Kind: faa.KindGoalClassified, Installation: successionTestBootstrap, Prev: cur.Head, GoalID: "goal-x", KernelVersion: "k"})
	if err := a.anchor.Set(ctx, successionTestBootstrap, &cur, faa.State{Installation: successionTestBootstrap, Seq: f.Seq, Head: f.Head}); err != nil {
		t.Fatal(err)
	}
	restarted := a.reopen(t)
	if _, err := restarted.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("an anchor ahead of the store must refuse: %v", err)
	}
	if err := restarted.anchorDecisionRetired(ctx, "x", "1", "sha256:x", "r", time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("a stranded store must not accept new facts: %v", err)
	}
}

// Crash boundary 3: the commit fails after the anchor advanced. The writer
// reverts the anchor, the store and anchor agree again, and the retry works.
func TestFAACommitFailureRevertsTheAnchor(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	digest, _ := d.decision.Digest()
	before, _ := a.anchor.Load(ctx, successionTestBootstrap)
	cancelCtx, cancel := context.WithCancel(ctx)
	a.anchor.AfterSet = func(faa.State) { cancel() } // the commit will fail
	err := a.repo.anchorDecisionRetired(cancelCtx, d.request.ID, d.request.Version, digest, "revoked", time.Now().UTC())
	a.anchor.AfterSet = nil
	if err == nil {
		t.Fatal("the commit was expected to fail")
	}
	after, _ := a.anchor.Load(ctx, successionTestBootstrap)
	if after != before {
		t.Fatalf("the anchor was left ahead after a failed commit: %+v vs %+v", after, before)
	}
	restarted := a.reopen(t)
	if _, err := restarted.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("the store and anchor must agree again: %v", err)
	}
}

// A backend that reports success without persisting is caught by the read-back
// and the write is refused.
func TestFAAAnchorWriteThatDoesNotStickIsRefused(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	digest, _ := d.decision.Digest()
	a.anchor.DropWrite = true
	defer func() { a.anchor.DropWrite = false }()
	if err := a.repo.anchorDecisionRetired(ctx, d.request.ID, d.request.Version, digest, "revoked", time.Now().UTC()); !errors.Is(err, ErrGovernanceAnchorUnavailable) {
		t.Fatalf("a lying anchor must refuse the retirement: %v", err)
	}
	if facts, _ := a.store.ListGovernanceFactRecords(ctx); len(facts) != 1 { // genesis only
		t.Fatalf("no fact may be committed when the anchor did not advance: %d", len(facts))
	}
}

// Legitimate recovery. A restored older store is refused; the governed
// re-anchor makes it consumable again WITHOUT making anything retired current
// and without admitting anything except the installation root the owner
// attested. The incident is a durable, inspectable fact.
func TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	a := anchoredFixture(t)
	root := rootSuccessionFixture(t, a.repo, now)
	d := a.withDecision(t)
	if _, err := a.repo.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("control: %v", err)
	}
	// The owner's backup is taken while the decision is in force.
	snapshot := filepath.Join(t.TempDir(), "backup.db")
	if _, err := a.store.DB().Exec(`VACUUM INTO '` + snapshot + `'`); err != nil {
		t.Fatal(err)
	}
	a.revoke(t, d) // retired AFTER the backup
	if err := a.store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		_ = os.Remove(a.path + suffix)
	}
	in, _ := os.Open(snapshot)
	out, _ := os.Create(a.path)
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	in.Close()
	out.Close()

	restored := a.reopen(t)
	if _, err := restored.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("the restored store must refuse before re-anchoring: %v", err)
	}
	status, err := restored.GovernanceStatus(ctx)
	if err != nil || status.Relation != string(faa.Behind) || status.AnchorSeq <= status.StoreSeq {
		t.Fatalf("status must say the store is behind: %+v %v", status, err)
	}
	plan, err := restored.PlanReanchor(ctx)
	if err != nil || plan.Cause != "behind" || plan.LostFacts == 0 || plan.RootDigest != root.Digest {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	// A confirmation of some other plan writes nothing.
	if _, err := restored.Reanchor(ctx, ReanchorRequest{PlanDigest: "sha256:" + string(make([]byte, 0)) + "0000000000000000000000000000000000000000000000000000000000000000", OSUser: "owner", CeremonyDigest: "sha256:c"}); err == nil {
		t.Fatal("a re-anchor confirmed against a different plan must refuse")
	}
	if _, err := restored.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", ClassifiedGoals: []string{"goal-lost"}}); err != nil {
		t.Fatal(err)
	}
	after := a.reopen(t)
	// The store is consumable again ...
	if got, err := after.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err != nil || got.Digest != root.Digest {
		t.Fatalf("the attested root must be current again: %v", err)
	}
	// ... but the decision that was in force in the backup, and was revoked in
	// the lost interval, is NOT current: every admission before the re-anchor is void.
	if _, err := after.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err == nil {
		t.Fatal("a re-anchor made a pre-recovery decision current")
	}
	if err := after.ValidateAuthorityGeneration(ctx, d.decision, time.Now().UTC()); err == nil {
		t.Fatal("a re-anchor made a pre-recovery non-root generation current")
	}
	// Owner-named classification is honoured; the store now agrees with the anchor.
	if _, ok, err := after.GoalSafetyKernel(ctx, "goal-lost"); err != nil || !ok {
		t.Fatalf("an owner-named classification must hold after recovery: %v %v", ok, err)
	}
	status, err = after.GovernanceStatus(ctx)
	if err != nil || status.Relation != string(faa.Consistent) || len(status.Reanchors) != 1 || status.Reanchors[0].Reanchor.Cause != "behind" || status.Reanchors[0].Reanchor.OSUser != "owner" {
		t.Fatalf("the recovery must be durable and inspectable: %+v %v", status, err)
	}
	// New authority is established the ordinary way and is current.
	if again, err := after.PlanReanchor(ctx); err != nil || !again.Consistent {
		t.Fatalf("nothing further to re-anchor: %+v %v", again, err)
	}
}

// Recovery from a lost, unreadable or ahead anchor takes the same governed path.
func TestFAAReanchorCoversMissingUnreadableAndAheadAnchors(t *testing.T) {
	for _, mode := range []string{"missing", "unreadable", "ahead"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			a := anchoredFixture(t)
			_ = rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
			good := a.anchor.Snapshot(successionTestBootstrap)
			switch mode {
			case "missing":
				a.anchor.Restore(successionTestBootstrap, nil)
			case "unreadable":
				a.anchor.Restore(successionTestBootstrap, []byte(`{"broken"`))
			case "ahead":
				cur, _ := a.anchor.Load(ctx, successionTestBootstrap)
				f, _ := faa.Seal(faa.Fact{Seq: cur.Seq + 1, Kind: faa.KindGoalClassified, Installation: successionTestBootstrap, Prev: cur.Head, GoalID: "g", KernelVersion: "k"})
				_ = a.anchor.Set(ctx, successionTestBootstrap, &cur, faa.State{Installation: successionTestBootstrap, Seq: f.Seq, Head: f.Head})
			}
			_ = good
			r := a.reopen(t)
			if _, err := r.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); !errors.Is(err, ErrGovernanceNotFresh) {
				t.Fatalf("%s anchor must refuse: %v", mode, err)
			}
			plan, err := r.PlanReanchor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
				t.Fatalf("%s: %v", mode, err)
			}
			if _, err := a.reopen(t).LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err != nil {
				t.Fatalf("%s: the attested root must be current after re-anchoring: %v", mode, err)
			}
		})
	}
}

// A re-anchor cannot run while the anchor backend is unavailable, and it never
// writes a fact it could not anchor.
func TestFAAReanchorRefusesWhenTheAnchorCannotBeWritten(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	_ = rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	a.anchor.Restore(successionTestBootstrap, nil)
	r := a.reopen(t)
	plan, err := r.PlanReanchor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.anchor.FailSet = true
	defer func() { a.anchor.FailSet = false }()
	if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err == nil {
		t.Fatal("a re-anchor must refuse when the anchor cannot be written")
	}
	if facts, _ := a.store.ListGovernanceFactRecords(ctx); len(facts) != 1 { // genesis only
		t.Fatalf("no fact may be written when the anchor could not be: %d", len(facts))
	}
}

// Fact rows beyond the last valid link are preserved, not consulted.
func TestFAAReanchorPreservesOrphanedFactsAndBridgesFromTheLastValidLink(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	_ = rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	for _, g := range []string{"g1", "g2", "g3"} {
		if err := a.repo.anchorGoalClassified(ctx, g, "k", time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	keylessDelete(t, a.store, state.GovernanceFactNamespace, state.GovernanceFactID(2), "1") // fact 3 is now orphaned
	r := a.reopen(t)
	if _, _, err := r.GoalSafetyKernel(ctx, "g1"); !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("a gap must refuse: %v", err)
	}
	plan, err := r.PlanReanchor(ctx)
	if err != nil || plan.OrphanFacts != 1 || plan.StoreSeq != 1 {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	after := a.reopen(t)
	if status, err := after.GovernanceStatus(ctx); err != nil || status.Relation != "consistent" || status.OrphanFacts != 1 {
		t.Fatalf("orphans must be preserved: %+v %v", status, err)
	}
	if _, ok, _ := after.GoalSafetyKernel(ctx, "g1"); !ok {
		t.Fatal("a classification on the valid prefix must survive")
	}
}

// Every governance namespace is either evidence of a Goal's governed status or
// declared not to be, with a reason. A new namespace that is neither fails this
// test, so classification cannot silently ignore a new kind of evidence.
func TestGoalEvidenceRegistryCoversEveryGovernanceNamespace(t *testing.T) {
	declared := map[string]bool{}
	evidence, nonEvidence := GoalEvidenceNamespaces()
	for _, n := range evidence {
		declared[n] = true
	}
	for _, n := range nonEvidence {
		if declared[n] {
			t.Fatalf("namespace %q is both evidence and non-evidence", n)
		}
		declared[n] = true
	}
	found := map[string]string{}
	for _, dir := range []string{".", "../state"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			parsed, err := parser.ParseFile(fset, file, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, decl := range parsed.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						if !strings.HasSuffix(name.Name, "Namespace") || i >= len(vs.Values) {
							continue
						}
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok && lit.Kind == token.STRING {
							value, _ := strconv.Unquote(lit.Value)
							found[value] = file + ":" + name.Name
						}
					}
				}
			}
		}
	}
	// Namespaces owned by other subsystems of the same table are not GoalStore
	// governance and are listed here so an addition is a deliberate act.
	foreign := map[string]bool{"root_authority_succession_proposal": true, "root_authority_succession_review": true, "root_authority_succession_decision": true, "governed_authority_proposal": true, "governed_authority_review": true}
	var missing []string
	for value, where := range found {
		if declared[value] || foreign[value] {
			continue
		}
		missing = append(missing, value+" ("+where+")")
	}
	sort.Strings(missing)
	if len(found) < 12 {
		t.Fatalf("the namespace scan found only %d constants; it is not scanning", len(found))
	}
	if len(missing) != 0 {
		t.Fatalf("governance namespaces that are neither Goal evidence nor declared non-evidence: %v", missing)
	}
}

// After a governed recovery a NEW decision, admitted under the new anchor
// sequence, is current: recovery voids the past, it does not freeze the future.
func TestFAAAuthorityAdmittedAfterAReanchorIsCurrent(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	_ = rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	a.anchor.Restore(successionTestBootstrap, nil)
	r := a.reopen(t)
	plan, err := r.PlanReanchor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	after := a.reopen(t)
	d := approveUnprotectedAcceptanceOn(t, after, after.Store)
	if _, err := after.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err != nil {
		t.Fatalf("a decision admitted after the re-anchor must be current: %v", err)
	}
}

// Publication and recovery admission, ordinary re-request and the lifecycle
// repair guard all consume the same predicate: a rolled-back store or a
// retirement fact refuses them.
func TestFAAEveryConsumerHonoursFactsAndFreshness(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	root := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	d := approveUnprotectedAcceptanceOn(t, a.repo, a.store)
	now := time.Now().UTC()
	checks := map[string]func(Repository) error{
		"publication fence": func(r Repository) error {
			return r.CheckGoalsPublicationInvalidation(ctx, d.generation.Ref, d.generation.Version, d.request.ID, d.request.Version, now)
		},
		"generation validation": func(r Repository) error { return r.ValidateAuthorityGeneration(ctx, d.decision, now) },
		"current root": func(r Repository) error {
			_, err := r.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, now)
			return err
		},
		"in-transaction guard": func(r Repository) error {
			tx, err := r.Store.DB().BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			return r.CheckAuthorityInForceInTx(ctx, tx, d.request.ID, d.request.Version, d.generation.Ref, d.generation.Version)
		},
	}
	for name, check := range checks {
		if err := check(a.repo); err != nil {
			t.Fatalf("%s control: %v", name, err)
		}
	}
	// retire the generation and the decision by fact
	gd, _ := d.decision.Digest()
	if err := a.repo.anchorDecisionRetired(ctx, d.request.ID, d.request.Version, gd, "revoked", now); err != nil {
		t.Fatal(err)
	}
	if err := a.repo.anchorGenerationRetired(ctx, d.generation.Ref, d.generation.Version, d.generation.Digest, "revoked", now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"publication fence", "generation validation", "in-transaction guard"} {
		if err := checks[name](a.reopen(t)); err == nil {
			t.Fatalf("%s accepted authority a fact retires", name)
		}
	}
	// roll the fact chain back: every consumer, including the root, refuses
	if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace='governance_fact'`); err != nil {
		t.Fatal(err)
	}
	for name, check := range checks {
		if err := check(a.reopen(t)); !errors.Is(err, ErrGovernanceNotFresh) {
			t.Fatalf("%s consumed a rolled-back store: %v", name, err)
		}
	}
	_ = root
}

// A store that already holds governance state but has no anchor is refused when
// a generation is saved; an anchor is never created next to existing state.
func TestFAAAnchorIsNeverCreatedNextToExistingGovernanceState(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	root := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	a.anchor.Restore(successionTestBootstrap, nil)
	r := a.reopen(t)
	if err := r.InitializeGovernanceAnchor(ctx); !errors.Is(err, ErrGovernanceAnchorMissing) {
		t.Fatalf("an anchor must not be created next to an existing store: %v", err)
	}
	other := root
	other.Ref, other.Version = root.Ref+"-x", "9"
	if err := r.SaveAuthorityGeneration(ctx, other, time.Now().UTC(), nil); !errors.Is(err, ErrGovernanceAnchorMissing) {
		t.Fatalf("admission under a missing anchor must refuse: %v", err)
	}
}

// Concurrent writers and readers over one store and one anchor: the chain stays
// valid and gapless, no fact is lost, and a reader is never refused for more than
// the length of one write.
func TestFAAConcurrentAppendersAndReadersKeepAValidChain(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	repos := make([]Repository, 4)
	for i := range repos {
		repos[i] = a.reopen(t)
	}
	const perWriter = 12
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for w := range repos {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				goal := fmt.Sprintf("goal-%d-%d", w, i)
				if err := repos[w].anchorGoalClassified(ctx, goal, "k", time.Now().UTC()); err != nil {
					errs <- fmt.Errorf("writer %d: %w", w, err)
					return
				}
			}
		}(w)
	}
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for i := range repos {
		readers.Add(1)
		go func(i int) {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, _, err := repos[i].GoalSafetyKernel(ctx, "goal-0-0"); err != nil {
					errs <- fmt.Errorf("reader %d: %w", i, err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	readers.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	status, err := a.reopen(t).GovernanceStatus(ctx)
	if err != nil || status.Relation != "consistent" || status.StoreSeq != uint64(len(repos)*perWriter) {
		t.Fatalf("the chain lost or duplicated facts: %+v %v", status, err)
	}
}

// An anchor written by someone who cannot read it cannot be set to any earlier
// valid value: the genesis is bound to a random nonce, so the empty chain's anchor
// state is not computable from the installation digest. This is the property that
// stops "restore a snapshot from before the first fact, and set the anchor to the
// genesis" from being undetectable.
func TestFAAGenesisIsNotComputableFromPublicInformation(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	first, err := a.anchor.Load(ctx, successionTestBootstrap)
	if err != nil || first.Seq != 0 {
		t.Fatalf("%+v %v", first, err)
	}
	for _, nonce := range []string{"0", "praxis", successionTestBootstrap, "n"} {
		_, forged, err := faa.NewGenesis(successionTestBootstrap, nonce, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if forged.Head == first.Head {
			t.Fatalf("a public guess reproduced the genesis head with nonce %q", nonce)
		}
	}
	// A different installation initialised the same way gets an unrelated head.
	b := anchoredFixture(t)
	second, _ := b.anchor.Load(ctx, successionTestBootstrap)
	if second.Head == first.Head {
		t.Fatal("two initialisations produced the same genesis head")
	}
	// Setting the anchor to a guessed genesis over a store that has facts is refused.
	d := a.withDecision(t)
	a.revoke(t, d)
	_, forged, _ := faa.NewGenesis(successionTestBootstrap, "n", time.Now())
	a.anchor.Restore(successionTestBootstrap, []byte(`{"installation":"`+successionTestBootstrap+`","seq":0,"head":"`+forged.Head+`"}`))
	if _, err := a.reopen(t).LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("a forged genesis anchor must refuse: %v", err)
	}
}

// Semantic adversarial coverage for rollback of mutually consistent SUBSETS.
// A timeline of four honest states is recorded; the anchor ends at the last. For
// EVERY assignment of each of six row groups (fact chain, decision liveness,
// revocation, generation liveness, generation invalidation, classification row)
// to any of the four states (4^6 = 4096 mixed stores), the adversary's store
// must never yield permission that the final state denies. This is exhaustive over
// the enumerated groups and states; it does not claim to cover rows of other
// namespaces, which cannot change these three answers.
func TestFAAMixedSnapshotAdversaryNeverRegainsRetiredAuthority(t *testing.T) {
	if testing.Short() || raceEnabled {
		t.Skip("4096 mixed stores: single-goroutine enumeration, run in the ordinary focused pass")
	}
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	dir := t.TempDir()
	snap := func(name string) string {
		p := filepath.Join(dir, name+".db")
		if _, err := a.store.DB().Exec(`VACUUM INTO '` + p + `'`); err != nil {
			t.Fatal(err)
		}
		return p
	}
	snaps := []string{snap("s0")}
	if err := a.repo.anchorGoalClassified(ctx, "goal-1", contracts.WorkPlanSafetyKernelVersion, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	snaps = append(snaps, snap("s1"))
	a.revoke(t, d)
	snaps = append(snaps, snap("s2"))
	gd := contracts.AuthorityGenerationInvalidation{Ref: d.generation.Ref, Version: d.generation.Version, GenerationDigest: d.generation.Digest, InvalidationRef: "inv-faa-1", InvalidationVersion: "1", Kind: "revoked", InvalidatedBy: d.generation.Principal, EffectiveAt: time.Now().UTC(), Reason: "retired"}
	if err := a.repo.SaveAuthorityGenerationInvalidation(ctx, gd, gd.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	snaps = append(snaps, snap("s3"))
	final := len(snaps) - 1

	groups := [][]string{
		{state.GovernanceFactNamespace},
		{state.AuthorityDecisionLiveNamespace},
		{authorityRevocationNamespace},
		{state.AuthorityGenerationLiveNamespace},
		{authorityGenerationInvalidationNamespace},
		{goalSafetyClassificationNamespace},
	}
	total, consistentWithAnchor := 0, 0
	choice := make([]int, len(groups))
	var walk func(i int)
	walk = func(i int) {
		if i < len(groups) {
			for s := range snaps {
				choice[i] = s
				walk(i + 1)
			}
			return
		}
		total++
		mixPath := filepath.Join(dir, "mix.db")
		for _, suffix := range []string{"", "-wal", "-shm"} {
			_ = os.Remove(mixPath + suffix)
		}
		in, _ := os.Open(snaps[final])
		out, _ := os.Create(mixPath)
		if _, err := io.Copy(out, in); err != nil {
			t.Fatal(err)
		}
		in.Close()
		out.Close()
		repo, store := repoFixtureAt(t, mixPath, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
		repo.InstallationDigest = successionTestBootstrap
		repo.FAA = a.anchor
		repo.faaLagAttempts = 1 // a mixed store is never a lag between a writer and a reader
		for g, names := range groups {
			if choice[g] == final {
				continue
			}
			for _, ns := range names {
				if _, err := store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=?`, ns); err != nil {
					t.Fatal(err)
				}
				if _, err := store.DB().Exec(`ATTACH DATABASE ? AS src`, snaps[choice[g]]); err != nil {
					t.Fatal(err)
				}
				if _, err := store.DB().Exec(`INSERT INTO secure_blobs SELECT * FROM src.secure_blobs WHERE namespace=?`, ns); err != nil {
					t.Fatal(err)
				}
				if _, err := store.DB().Exec(`DETACH DATABASE src`); err != nil {
					t.Fatal(err)
				}
			}
		}
		mix := fmt.Sprint(choice)
		if _, err := repo.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err == nil {
			t.Fatalf("mix %s regained the revoked decision", mix)
		}
		if err := repo.ValidateAuthorityGeneration(ctx, d.decision, time.Now().UTC()); err == nil {
			t.Fatalf("mix %s regained the invalidated generation", mix)
		}
		if _, classified, err := repo.GoalSafetyKernel(ctx, "goal-1"); err == nil && !classified {
			t.Fatalf("mix %s made the classified Goal legacy", mix)
		}
		if choice[0] == final {
			consistentWithAnchor++
		}
		store.DB().Close()
	}
	walk(0)
	if total != 4096 || consistentWithAnchor != 1024 {
		t.Fatalf("enumeration incomplete: %d mixes, %d with the anchored fact chain", total, consistentWithAnchor)
	}
}

// A protected request whose proposal is missing fails closed: its governed
// context, and therefore its activation requirement, cannot be established. An
// unprotected request is unchanged.
func TestProtectedRequestWithoutItsProposalFailsClosed(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	protected := contracts.AuthorityRequest{ID: "r", Version: "1", RequestedAuthority: contracts.GovernedWorkPlanAccept, ProposalID: "proposal-missing", ProposalVersion: "1", CeremonyProfile: contracts.OwnerCeremonyProfile}
	if binding, err := repo.safetyBindingForRequest(ctx, protected); err == nil || binding != nil || !errors.Is(err, state.ErrSecureBlobNotFound) {
		t.Fatalf("a protected request without its proposal must be refused: %v %v", binding, err)
	}
	legacy := protected
	legacy.CeremonyProfile = ""
	if binding, err := repo.safetyBindingForRequest(ctx, legacy); err != nil || binding != nil {
		t.Fatalf("an unprotected request without its proposal is unchanged: %v %v", binding, err)
	}
}

// Recovery admission re-checks authority inside the admitting transaction; that
// check reads the same anchored chain the transaction sees.
func TestFAARecoveryAdmissionRecheckConsultsTheAnchoredChain(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := approveUnprotectedAcceptanceOn(t, a.repo, a.store)
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	auth := contracts.PackagePublishAuthorization{}
	auth.Request.ID, auth.Request.Version = d.request.ID, d.request.Version
	auth.Decision.ExpiresAt = &expires
	auth.Generation.Ref, auth.Generation.Version, auth.Generation.ExpiresAt = d.generation.Ref, d.generation.Version, &expires
	check := func(r Repository) error {
		tx, err := r.Store.DB().BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		return r.RevalidateGoalsPublicationRecoveryAuthorizationTx(ctx, tx, auth, now)
	}
	if err := check(a.repo); err != nil {
		t.Fatalf("control: %v", err)
	}
	gd, _ := d.decision.Digest()
	if err := a.repo.anchorDecisionRetired(ctx, d.request.ID, d.request.Version, gd, "revoked", now); err != nil {
		t.Fatal(err)
	}
	if err := check(a.reopen(t)); err == nil {
		t.Fatal("recovery admission accepted a decision an anchored fact retires")
	}
	if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace='governance_fact'`); err != nil {
		t.Fatal(err)
	}
	if err := check(a.reopen(t)); !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("recovery admission consumed a rolled-back store: %v", err)
	}
}

// Crash boundary 4: a re-anchor whose fact and anchor committed but whose root
// re-admission did not. The store is consistent, the root is not current, and
// running the ceremony again completes exactly the re-admission the fact names.
func TestFAAReanchorInterruptedBeforeRootReadmissionIsCompletedByTheSameCeremony(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	root := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	a.anchor.Restore(successionTestBootstrap, nil)
	r := a.reopen(t)
	plan, err := r.PlanReanchor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	// the crash: the root's re-stamped liveness is lost, the old stamp returns
	if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=?`, state.AuthorityGenerationLiveNamespace); err != nil {
		t.Fatal(err)
	}
	after := a.reopen(t)
	if _, err := after.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err == nil {
		t.Fatal("the root must not be current before its re-admission")
	}
	plan, err = after.PlanReanchor(ctx)
	if err != nil || !plan.Consistent {
		t.Fatalf("the store is consistent with the anchor: %+v %v", plan, err)
	}
	if _, err := after.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	got, err := a.reopen(t).LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC())
	if err != nil || got.Digest != root.Digest {
		t.Fatalf("the interrupted re-admission was not completed: %v", err)
	}
	if facts, _ := a.store.ListGovernanceFactRecords(ctx); len(facts) != 2 { // genesis + one re-anchor: completing appends nothing
		t.Fatalf("completing a re-admission must not append a second re-anchor: %d", len(facts))
	}
}

// A backend that hands back another installation's state is refused by the
// repository even if the backend itself did not.
type foreignStateAnchor struct{ *faatest.Memory }

func (f foreignStateAnchor) Load(ctx context.Context, installation string) (faa.State, error) {
	s, err := f.Memory.Load(ctx, installation)
	if err != nil {
		return s, err
	}
	s.Installation = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	return s, nil
}

func TestFAARepositoryRefusesAnAnchorThatNamesAnotherInstallation(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	r := a.reopen(t)
	r.FAA = foreignStateAnchor{a.anchor}
	if _, err := r.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceAnchorUnavailable) {
		t.Fatalf("an anchor state naming another installation must be refused: %v", err)
	}
}

// A reader refuses an invalid chain at once and without touching the writer lock,
// waits out a write that is genuinely in flight (the anchor one ahead of a store
// whose writer has not committed), and refuses a real disagreement immediately.
func TestFAAReaderBarrierWaitsOutAnInFlightWriteAndRefusesRealDisagreement(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	holdLock := func(hold time.Duration) chan struct{} {
		held, release := make(chan struct{}), make(chan struct{})
		go func() {
			_ = a.store.WithWriteLock(ctx, func(*sql.Tx) error {
				close(held)
				<-release
				return nil
			})
		}()
		<-held
		go func() { time.Sleep(hold); close(release) }()
		return release
	}

	// (1) an invalid chain is refused at once, even while a writer holds the lock
	if err := a.repo.anchorGoalClassified(ctx, "goal-1", "k", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := a.repo.anchorGoalClassified(ctx, "goal-2", "k", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	keylessDelete(t, a.store, state.GovernanceFactNamespace, state.GovernanceFactID(1), "1") // a gap
	release := holdLock(600 * time.Millisecond)
	start := time.Now()
	if _, err := a.reopen(t).LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceChainInvalid) {
		t.Fatalf("want an invalid chain, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("an invalid chain waited for the writer lock (%s)", elapsed)
	}
	<-release
	time.Sleep(700 * time.Millisecond)

	// (2) a real disagreement (the store behind the anchor, no write in flight) is refused at once
	if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace='governance_fact'`); err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	slow := a.reopen(t)
	slow.faaLagAttempts, slow.faaLagDelay = 1, 2*time.Second // the backoff fallback would take two seconds
	if _, err := slow.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("want behind, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("a real rollback was waited out instead of refused (%s)", elapsed)
	}
}

// While a writer is between advancing the anchor and committing, a reader waits
// for it and then succeeds.
func TestFAAReaderWaitsForAWriterBetweenAnchorAdvanceAndCommit(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	d := a.withDecision(t)
	a.anchor.AfterSet = func(faa.State) { time.Sleep(400 * time.Millisecond) } // runs inside the writer's transaction, after the anchor advanced
	writerDone := make(chan error, 1)
	go func() {
		digest, _ := d.decision.Digest()
		writerDone <- a.repo.anchorDecisionRetired(ctx, d.request.ID, d.request.Version, digest, "revoked", time.Now().UTC())
	}()
	time.Sleep(100 * time.Millisecond) // the writer is now in flight: anchor ahead, store not yet
	start := time.Now()
	reader := a.reopen(t)
	reader.faaLagAttempts, reader.faaLagDelay = 1, 3*time.Second // a reader that backed off instead of using the lock would sleep this long
	_, err := reader.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC())
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("the reader backed off instead of accepting the state observed behind the writer lock (%s)", elapsed)
	}
	if err := <-writerDone; err != nil {
		t.Fatal(err)
	}
	a.anchor.AfterSet = nil
	if errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("a reader was refused for an in-flight write: %v", err)
	}
	if !errors.Is(err, ErrAuthorityRetired) && !errors.Is(err, ErrAuthorityDecisionRevoked) {
		t.Fatalf("after the write commits the decision must be retired: %v", err)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("the reader did not wait for the in-flight writer (%s)", elapsed)
	}
}

// Initialisation never creates an anchor next to existing governance state, even
// when no fact survives, and only replaces a crashed initialisation at sequence 0.
func TestFAAInitialisationRefusesExistingStateAndNonZeroAnchors(t *testing.T) {
	ctx := context.Background()
	// generations exist, no facts, no anchor
	path := filepath.Join(t.TempDir(), "praxis.db")
	plain, store := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	plain.InstallationDigest = successionTestBootstrap
	_ = rootSuccessionFixture(t, plain, time.Now().UTC().Add(-time.Minute)) // unanchored: no facts
	anchored := plain
	anchored.FAA = faatest.NewMemory()
	if err := anchored.InitializeGovernanceAnchor(ctx); !errors.Is(err, ErrGovernanceAnchorMissing) {
		t.Fatalf("an anchor must not be created next to existing generations: %v", err)
	}
	// a non-zero anchor over an empty store is not a crashed initialisation
	empty := anchoredFixture(t)
	cur, _ := empty.anchor.Load(ctx, successionTestBootstrap)
	f, _ := faa.Seal(faa.Fact{Seq: 1, Kind: faa.KindGoalClassified, Installation: successionTestBootstrap, Prev: cur.Head, GoalID: "g", KernelVersion: "k"})
	if err := empty.anchor.Set(ctx, successionTestBootstrap, &cur, faa.State{Installation: successionTestBootstrap, Seq: 1, Head: f.Head}); err != nil {
		t.Fatal(err)
	}
	if _, err := empty.store.DB().Exec(`DELETE FROM secure_blobs`); err != nil {
		t.Fatal(err)
	}
	if err := empty.reopen(t).InitializeGovernanceAnchor(ctx); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("an anchor past genesis over an erased store must refuse, not be replaced: %v", err)
	}
	_ = store
}

// A classification that exists only as a row (or is derivable) is anchored the
// next time it is marked, so it survives loss of the evidence.
func TestFAAClassificationIsAnchoredWhenMarkedAndWhenOnlyDerived(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	binding := &contracts.WorkPlanSafetyBinding{KernelVersion: contracts.WorkPlanSafetyKernelVersion}
	if err := a.repo.markGoalSafetyBearing(ctx, "goal-a", binding, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	snap, err := a.repo.governanceSnapshot(ctx)
	if err != nil || snap.View.Classified["goal-a"] != contracts.WorkPlanSafetyKernelVersion {
		t.Fatalf("a first classification must be an anchored fact: %+v %v", snap.View.Classified, err)
	}
	// derived-only: a classification row with no fact
	payload := []byte(`{"goal_id":"goal-b","kernel_version":"` + contracts.WorkPlanSafetyKernelVersion + `"}`)
	if err := a.repo.putWorkPlanBlob(ctx, goalSafetyClassificationNamespace, "goal-b", "1", payload, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if snap, _ := a.repo.governanceSnapshot(ctx); snap.View.Classified["goal-b"] != "" {
		t.Fatal("the row alone must not be an anchored fact yet")
	}
	if err := a.repo.markGoalSafetyBearing(ctx, "goal-b", binding, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if snap, _ := a.repo.governanceSnapshot(ctx); snap.View.Classified["goal-b"] != contracts.WorkPlanSafetyKernelVersion {
		t.Fatal("marking an already-classified Goal must anchor the fact")
	}
}

// A root retired by an anchored fact is not current although every row still
// says it is; and root resolution uses the anchored chain, not liveness alone.
func TestFAARootRetiredByFactAloneIsNotCurrent(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	root := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	if _, err := a.repo.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err != nil {
		t.Fatalf("control: %v", err)
	}
	if err := a.repo.anchorGenerationRetired(ctx, root.Ref, root.Version, root.Digest, "revoked", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.reopen(t).LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err == nil {
		t.Fatal("a root retired by an anchored fact was current")
	}
}

// Root succession anchors the supersession, so deleting the successor and the
// predecessor's supersession and replaying the predecessor's copied liveness
// cannot revive it.
func TestFAARootSuccessionIsAnchoredAndItsRollbackByReplayIsRefused(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	a := anchoredFixture(t)
	predecessor := rootSuccessionFixture(t, a.repo, now)
	copied := snapshotRow(t, a.store, state.AuthorityGenerationLiveNamespace, predecessor.Ref, predecessor.Version)
	successor, _, _, _ := acceptedRootSuccession(t, a.repo, predecessor, now)
	snap, err := a.repo.governanceSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.View.RetiredGenerations[faa.GenerationKey(predecessor.Ref, predecessor.Version, predecessor.Digest)]; !ok {
		t.Fatal("the supersession must be an anchored fact")
	}
	keylessDelete(t, a.store, state.AuthorityGenerationNamespace, successor.Ref, successor.Version)
	keylessDelete(t, a.store, state.AuthorityGenerationInvalidationNamespace, predecessor.Ref, predecessor.Version)
	if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=?`, state.AuthorityGenerationLiveNamespace); err != nil {
		t.Fatal(err)
	}
	replayRow(t, a.store, copied)
	if got, err := a.reopen(t).LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err == nil {
		t.Fatalf("the superseded predecessor was revived: %s/%s", got.Ref, got.Version)
	}
}

// The in-transaction guard needs BOTH the decision and its issuing generation.
func TestFAAInTransactionGuardNeedsTheDecisionAndTheGenerationEachOnItsOwn(t *testing.T) {
	ctx := context.Background()
	for _, deleteNS := range []string{state.AuthorityDecisionLiveNamespace, state.AuthorityGenerationLiveNamespace} {
		a := anchoredFixture(t)
		d := approveUnprotectedAcceptanceOn(t, a.repo, a.store)
		check := func() error {
			tx, err := a.repo.Store.DB().BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			return a.repo.CheckAuthorityInForceInTx(ctx, tx, d.request.ID, d.request.Version, d.generation.Ref, d.generation.Version)
		}
		if err := check(); err != nil {
			t.Fatalf("control: %v", err)
		}
		if _, err := a.store.DB().Exec(`DELETE FROM secure_blobs WHERE namespace=?`, deleteNS); err != nil {
			t.Fatal(err)
		}
		if err := check(); err == nil {
			t.Fatalf("the guard accepted authority whose %s is missing", deleteNS)
		}
	}
}

// Re-anchoring: exactly one un-retired root, a root retired by fact is not a
// candidate, and completing an already-complete re-admission rewrites nothing.
func TestFAAReanchorRootSelectionAndIdempotence(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	first := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	second := first
	second.Version = "2"
	second.EffectiveAt = first.EffectiveAt.Add(time.Second)
	second.Digest, _ = second.ComputeDigest()
	if err := a.repo.SaveAuthorityGeneration(ctx, second, second.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	a.anchor.Restore(successionTestBootstrap, nil)
	r := a.reopen(t)
	if _, err := r.PlanReanchor(ctx); err == nil {
		t.Fatal("two un-retired installation roots must refuse a re-anchor")
	}
	// retire one by fact: it stops being a candidate (the anchor is missing, so
	// the chain is appended through a repository that still sees it)
	a.anchor.Restore(successionTestBootstrap, nil)
	b := anchoredFixture(t)
	root1 := rootSuccessionFixture(t, b.repo, time.Now().UTC().Add(-time.Minute))
	root2 := root1
	root2.Version = "2"
	root2.EffectiveAt = root1.EffectiveAt.Add(time.Second)
	root2.Digest, _ = root2.ComputeDigest()
	if err := b.repo.SaveAuthorityGeneration(ctx, root2, root2.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.repo.anchorGenerationRetired(ctx, root1.Ref, root1.Version, root1.Digest, "revoked", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	b.anchor.Restore(successionTestBootstrap, nil)
	rb := b.reopen(t)
	plan, err := rb.PlanReanchor(ctx)
	if err != nil || plan.RootVersion != "2" {
		t.Fatalf("a root retired by fact must not be a candidate: %+v %v", plan, err)
	}
	if _, err := rb.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	before := snapshotRow(t, b.store, state.AuthorityGenerationLiveNamespace, root2.Ref, root2.Version)
	again, _ := b.reopen(t).PlanReanchor(ctx)
	if _, err := b.reopen(t).Reanchor(ctx, ReanchorRequest{PlanDigest: again.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	after := snapshotRow(t, b.store, state.AuthorityGenerationLiveNamespace, root2.Ref, root2.Version)
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatal("completing an already-complete re-admission rewrote the liveness record")
	}
}

// A root whose liveness record predates the latest re-anchor is void even though
// the record is authentic, names the right digest and nothing retires it.
func TestFAARootAdmittedBeforeTheReanchorIsNotCurrent(t *testing.T) {
	ctx := context.Background()
	a := anchoredFixture(t)
	root := rootSuccessionFixture(t, a.repo, time.Now().UTC().Add(-time.Minute))
	before := snapshotRow(t, a.store, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version)
	a.anchor.Restore(successionTestBootstrap, nil)
	r := a.reopen(t)
	plan, err := r.PlanReanchor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.reopen(t).LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err != nil {
		t.Fatalf("control: %v", err)
	}
	keylessDelete(t, a.store, state.AuthorityGenerationLiveNamespace, root.Ref, root.Version)
	replayRow(t, a.store, before) // the pre-recovery admission, byte for byte
	if _, err := a.reopen(t).LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err == nil {
		t.Fatal("a root admitted before the re-anchor was current")
	}
}
