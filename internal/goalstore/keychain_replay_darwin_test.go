//go:build darwin

package goalstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestRepair6OpaqueKeychainFileReplayIsRefused is Review 6 finding N16 turned
// into a regression: the attack that restored a revoked decision must now be
// refused end to end. The setup is the reviewer's, unchanged, except that every
// step that used to reproduce the attack now asserts the refusal.
func TestRepair6OpaqueKeychainFileReplayIsRefused(t *testing.T) {
	ctx := context.Background()
	requireKeychain(t)
	service := uniqueKeychainService("com.convergent-systems-co.praxis.faa.repair6")
	anchor, remove := praxiscrypto.NewTemporaryKeychainAnchor(service)
	k := anchor.(*praxiscrypto.KeychainAnchor)
	k.Dir = t.TempDir()
	t.Cleanup(func() {
		if err := remove(successionTestBootstrap); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, store := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	repo.FAA = anchor
	if err := repo.InitializeGovernanceAnchor(ctx); err != nil {
		t.Fatalf("initialising the anchor failed after the Keychain probe passed: %v", err)
	}
	d := approveUnprotectedAcceptanceOn(t, repo, store)
	snapshot := filepath.Join(t.TempDir(), "earlier.db")
	if _, err := store.DB().Exec(`VACUUM INTO '` + snapshot + `'`); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(service + "\x00" + successionTestBootstrap))
	kcPath := filepath.Join(k.Dir, "praxis-faa-"+hex.EncodeToString(sum[:8])+".keychain-db")
	kcSnapshot := filepath.Join(t.TempDir(), "opaque-keychain-snapshot")
	cp := func(src, dst string) {
		t.Helper()
		if out, err := exec.Command("/bin/cp", src, dst).CombinedOutput(); err != nil {
			t.Fatalf("opaque copy: %v %s", err, out)
		}
	}
	cp(kcPath, kcSnapshot)
	old, err := anchor.Load(ctx, successionTestBootstrap)
	if err != nil {
		t.Fatal(err)
	}
	digest, _ := d.decision.Digest()
	revocation := contracts.AuthorityRevocation{
		RequestID: d.request.ID, RequestVersion: d.request.Version,
		DecisionRef: d.decision.DecisionRef, DecisionVersion: d.decision.DecisionVersion,
		DecisionDigest: digest, RevocationRef: "review6-revoke", RevocationVersion: "1",
		RevokedBy: d.decision.DecidedBy, AuthorityDigest: "sha256:r",
		EffectiveAt: time.Now().UTC(), Reason: "withdrawn",
	}
	if err := repo.SaveAuthorityRevocation(ctx, d.request.ID, d.request.Version, revocation, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	advanced, err := anchor.Load(ctx, successionTestBootstrap)
	if err != nil || advanced.Seq != old.Seq+1 {
		t.Fatalf("advance: %+v %v", advanced, err)
	}
	if _, err := repo.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); err == nil {
		t.Fatal("revocation did not refuse")
	}
	if err := store.DB().Close(); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	cp(snapshot, path)
	restored, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	restored.InstallationDigest = successionTestBootstrap
	restored.FAA = anchor
	if _, err := restored.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("database-only rollback control: %v", err)
	}

	// Reopen the opaque snapshot from another path to avoid Security.framework's
	// cached handle for the current file. The encrypted bytes and current login-
	// Keychain password item are otherwise unchanged.
	replayDir := t.TempDir()
	replayPath := filepath.Join(replayDir, filepath.Base(kcPath))
	cp(kcSnapshot, replayPath)
	k.Dir = replayDir
	got, err := anchor.Load(ctx, successionTestBootstrap)
	t.Logf("opaque file restore: old seq=%d advanced seq=%d restored seq=%d error=%v", old.Seq, advanced.Seq, got.Seq, err)
	if err == nil {
		t.Fatalf("N16: the earlier dedicated-keychain file opened with the current password item and read %+v", got)
	}
	if !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("the replayed file must read as a corrupt anchor: %v", err)
	}
	decision, err := restored.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC())
	if err == nil {
		t.Fatalf("N16: a revoked decision was accepted after an opaque database + dedicated-keychain-file restore: %s", decision.DecisionRef)
	}
	t.Logf("the revoked decision stays refused: %v", err)
	// The store is refused wholesale, not just this decision: no governed read passes.
	if _, err := restored.LoadAuthorityDecision(ctx, "any", "1", time.Now().UTC()); err == nil {
		t.Fatal("a store compared against a corrupt anchor must yield no governance")
	}
}
