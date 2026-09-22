//go:build darwin

package goalstore

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/faa"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// The real macOS Keychain backend, end to end: revocation is anchored, a
// byte-consistent earlier copy of the database is restored, and the store is
// refused although it is internally authentic. Skipped only when the one-time
// probe (requireKeychain) shows the Keychain is unavailable in this
// environment; never reported as passed.
func TestFAAKeychainBackendDetectsWholeDatabaseRollbackEndToEnd(t *testing.T) {
	ctx := context.Background()
	requireKeychain(t)
	anchor, remove := praxiscrypto.NewTemporaryKeychainAnchor(uniqueKeychainService("com.convergent-systems-co.praxis.faa.qualification"))
	t.Cleanup(func() { _ = remove(successionTestBootstrap) })
	_ = remove(successionTestBootstrap)
	if _, err := anchor.Load(ctx, successionTestBootstrap); errors.Is(err, faa.ErrUnavailable) {
		t.Fatalf("the Keychain probe passed, yet the anchor is unavailable: %v", err)
	}
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, store := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	repo.FAA = anchor
	if err := repo.InitializeGovernanceAnchor(ctx); err != nil {
		t.Fatalf("initialising the anchor failed after the Keychain probe passed: %v", err)
	}
	a := anchored{repo: repo, store: store, path: path}
	d := approveUnprotectedAcceptanceOn(t, repo, store)
	snapshot := filepath.Join(t.TempDir(), "earlier.db")
	if _, err := store.DB().Exec(`VACUUM INTO '` + snapshot + `'`); err != nil {
		t.Fatal(err)
	}
	digest, _ := d.decision.Digest()
	revocation := contracts.AuthorityRevocation{RequestID: d.request.ID, RequestVersion: d.request.Version, DecisionRef: d.decision.DecisionRef, DecisionVersion: d.decision.DecisionVersion, DecisionDigest: digest, RevocationRef: "kc-revoke", RevocationVersion: "1", RevokedBy: d.decision.DecidedBy, AuthorityDigest: "sha256:r", EffectiveAt: time.Now().UTC(), Reason: "withdrawn"}
	if err := repo.SaveAuthorityRevocation(ctx, d.request.ID, d.request.Version, revocation, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if st, err := anchor.Load(ctx, successionTestBootstrap); err != nil || st.Seq != 1 {
		t.Fatalf("the Keychain item must hold the anchored sequence: %+v %v", st, err)
	}
	if err := store.DB().Close(); err != nil {
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
	restored, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	restored.InstallationDigest = successionTestBootstrap
	restored.FAA = anchor
	if _, err := restored.LoadAuthorityDecision(ctx, d.request.ID, d.request.Version, time.Now().UTC()); !errors.Is(err, ErrGovernanceRolledBack) {
		t.Fatalf("the real Keychain anchor did not refuse an earlier authentic store: %v", err)
	}
}

// Loss of the password item that protects the anchor's keychain (which the
// platform lets another process of the user do silently) fails closed as an
// unreadable anchor, and the only way back is the governed re-anchor, which
// creates a fresh protected keychain and re-admits only the attested root.
func TestFAAKeychainPasswordLossFailsClosedAndTheGovernedReanchorRecovers(t *testing.T) {
	ctx := context.Background()
	requireKeychain(t)
	service := uniqueKeychainService("com.convergent-systems-co.praxis.faa.qualification-passwordloss")
	anchor, remove := praxiscrypto.NewTemporaryKeychainAnchor(service)
	t.Cleanup(func() { _ = remove(successionTestBootstrap) })
	_ = remove(successionTestBootstrap)
	if _, err := anchor.Load(ctx, successionTestBootstrap); errors.Is(err, faa.ErrUnavailable) {
		t.Fatalf("the Keychain probe passed, yet the anchor is unavailable: %v", err)
	}
	path := filepath.Join(t.TempDir(), "praxis.db")
	repo, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	repo.InstallationDigest = successionTestBootstrap
	repo.FAA = anchor
	if err := repo.InitializeGovernanceAnchor(ctx); err != nil {
		t.Fatal(err)
	}
	_ = rootSuccessionFixture(t, repo, time.Now().UTC().Add(-time.Minute))
	if _, err := repo.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err != nil {
		t.Fatalf("the root must be current before the loss: %v", err)
	}
	// What another process of the same user can do silently: remove the password item.
	// Done in process: an external tool could raise a prompt of its own.
	if err := anchor.(*praxiscrypto.KeychainAnchor).RemovePasswordItem(successionTestBootstrap); err != nil {
		t.Fatalf("could not remove the password item: %v", err)
	}
	reopened, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	reopened.InstallationDigest = successionTestBootstrap
	reopened.FAA = anchor
	if _, err := reopened.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); !errors.Is(err, ErrGovernanceNotFresh) {
		t.Fatalf("an anchor whose protection is gone must refuse: %v", err)
	}
	plan, err := reopened.PlanReanchor(ctx)
	if err != nil || plan.Cause != "unreadable" {
		t.Fatalf("the ceremony must classify it as unreadable: %+v %v", plan, err)
	}
	if _, err := reopened.Reanchor(ctx, ReanchorRequest{PlanDigest: plan.Digest(), OSUser: "owner", CeremonyDigest: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}); err != nil {
		t.Fatalf("the governed re-anchor must recreate the protected anchor: %v", err)
	}
	after, _ := repoFixtureAt(t, path, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	after.InstallationDigest = successionTestBootstrap
	after.FAA = anchor
	if _, err := after.LoadCurrentInstallationRoot(ctx, successionTestBootstrap, time.Now().UTC()); err != nil {
		t.Fatalf("the attested root must be current after the ceremony: %v", err)
	}
}
