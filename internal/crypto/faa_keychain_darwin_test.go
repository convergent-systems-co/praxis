//go:build darwin

package crypto

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/faa"
)

// The qualification tests must never wait for a person: with interaction
// disabled, a prompt (for instance for an item another test binary created)
// surfaces at once as errSecInteractionNotAllowed (-25308).
func TestMain(m *testing.M) {
	restore := DisableUserInteraction()
	code := m.Run()
	restore()
	os.Exit(code)
}

// snapshotRekeyState records the values of the test-visible re-key state AS
// OBSERVED NOW and restores exactly those when the test ends, so a test that
// forgets (or fails before) its own reset can never leak state into the next one
// and no outcome depends on execution order. Restoring an assumed default would
// only be correct if every earlier test had left the defaults in place.
func snapshotRekeyState(t *testing.T) {
	t.Helper()
	hook, change, symbol, quiet := rotationHook, changePassword, changePasswordSymbol, rekeyWithoutInteraction
	t.Cleanup(func() {
		rotationHook, changePassword, changePasswordSymbol, rekeyWithoutInteraction = hook, change, symbol, quiet
	})
}

// uniqueService names a Keychain service no other test binary, run or mutation
// copy can share, so an item is only ever touched by the binary that made it.
func uniqueService(base string) string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s.%d.%s", base, os.Getpid(), hex.EncodeToString(raw))
}

var keychainProbe struct {
	once sync.Once
	err  error
}

// requireKeychain decides ONCE, with a fresh unique service and interaction
// disabled, whether this environment can run the real Keychain at all. Only that
// probe may justify a skip. After it passes, a later "unavailable" or -25308 is
// an unexpected interaction requirement and the test FAILS: a regression must
// never turn into a skipped test. PRAXIS_REQUIRE_KEYCHAIN=1 turns even a failed
// probe into a failure, which qualification runs set.
func requireKeychain(t *testing.T) {
	t.Helper()
	keychainProbe.once.Do(func() {
		dir, err := os.MkdirTemp("", "praxis-faa-probe-")
		if err != nil {
			keychainProbe.err = err
			return
		}
		defer os.RemoveAll(dir)
		k := &KeychainAnchor{Service: uniqueService(macOSFAAService + ".test.probe"), Dir: dir}
		installation := "sha256:00000000000000000000000000000000000000000000000000000000000000f0"
		defer k.deleteForTest(installation)
		_, g, err := faa.NewGenesis(installation, "probe", time.Now())
		if err != nil {
			keychainProbe.err = err
			return
		}
		if err := k.Set(context.Background(), installation, nil, g); err != nil {
			keychainProbe.err = err
			return
		}
		if got, err := k.Load(context.Background(), installation); err != nil || got != g {
			keychainProbe.err = fmt.Errorf("probe read-back: %+v %v", got, err)
		}
	})
	if keychainProbe.err == nil {
		return
	}
	if os.Getenv("PRAXIS_REQUIRE_KEYCHAIN") != "" {
		t.Fatalf("PRAXIS_REQUIRE_KEYCHAIN is set and the real Keychain probe failed: %v", keychainProbe.err)
	}
	t.Skipf("the real Keychain is unavailable in this environment (probe: %v); set PRAXIS_REQUIRE_KEYCHAIN=1 to make this a failure", keychainProbe.err)
}

// requireRekey is the same for the undocumented re-key entry point.
func requireRekey(t *testing.T) {
	t.Helper()
	if rekeySupported() {
		return
	}
	if os.Getenv("PRAXIS_REQUIRE_KEYCHAIN") != "" {
		t.Fatal("PRAXIS_REQUIRE_KEYCHAIN is set and this macOS does not export SecKeychainChangePassword")
	}
	t.Skip("this macOS does not export SecKeychainChangePassword; production fails closed here (see the missing-entry-point test)")
}

func TestKeychainAnchorIsForwardOnlyCompareAndSetAndFailsClosed(t *testing.T) {
	ctx := context.Background()
	requireKeychain(t)
	k := &KeychainAnchor{Service: uniqueService(macOSFAAService + ".test"), Dir: t.TempDir()}
	installation := "sha256:" + string(make([]byte, 0)) + "0000000000000000000000000000000000000000000000000000000000000001"
	t.Cleanup(func() { _ = k.deleteForTest(installation) })
	_ = k.deleteForTest(installation)
	if _, err := k.Load(ctx, installation); !errors.Is(err, faa.ErrMissing) {
		t.Fatalf("a missing item must be ErrMissing (the probe passed, so unavailable here is a failure): %v", err)
	}
	_, g, err := faa.NewGenesis(installation, "nonce", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Set(ctx, installation, nil, g); err != nil {
		t.Fatal(err)
	}
	if err := k.Set(ctx, installation, nil, g); !errors.Is(err, faa.ErrConflict) {
		t.Fatalf("creating twice must conflict: %v", err)
	}
	head1, _ := faa.Seal(faa.Fact{Seq: 1, Kind: faa.KindGoalClassified, Installation: installation, Prev: g.Head, GoalID: "g", KernelVersion: "k"})
	s1 := faa.State{Installation: installation, Seq: 1, Head: head1.Head}
	if err := k.Set(ctx, installation, &g, s1); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != s1 {
		t.Fatalf("load after set: %+v %v", got, err)
	}
	// Not strictly forward, and a stale expectation, both refuse.
	if err := k.Set(ctx, installation, &s1, s1); !errors.Is(err, faa.ErrConflict) {
		t.Fatalf("a non-forward set must conflict: %v", err)
	}
	if err := k.Set(ctx, installation, &g, faa.State{Installation: installation, Seq: 2, Head: s1.Head}); !errors.Is(err, faa.ErrConflict) {
		t.Fatalf("a stale expectation must conflict: %v", err)
	}
	// Another installation's state is never returned.
	if _, err := k.Load(ctx, "sha256:0000000000000000000000000000000000000000000000000000000000000002"); !errors.Is(err, faa.ErrMissing) {
		t.Fatalf("a different installation must read as missing: %v", err)
	}
}

func TestKeychainAnchorNeverReturnsAnotherInstallationsState(t *testing.T) {
	ctx := context.Background()
	requireKeychain(t)
	k := &KeychainAnchor{Service: uniqueService(macOSFAAService + ".test"), Dir: t.TempDir()}
	a := "sha256:0000000000000000000000000000000000000000000000000000000000000011"
	b := "sha256:0000000000000000000000000000000000000000000000000000000000000012"
	t.Cleanup(func() { _ = k.deleteForTest(a) })
	_ = k.deleteForTest(a)
	_, other, err := faa.NewGenesis(b, "n", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// State for installation B written under installation A's account.
	if err := k.write(a, other, true); err != nil {
		t.Fatalf("writing the fixture failed after the Keychain probe passed: %v", err)
	}
	if _, err := k.Load(ctx, a); !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("another installation's state must not be returned: %v", err)
	}
}

func keychainFixture(t *testing.T, suffix string) (*KeychainAnchor, string, faa.State) {
	t.Helper()
	requireKeychain(t)
	snapshotRekeyState(t)
	k := &KeychainAnchor{Service: uniqueService(macOSFAAService + ".test." + suffix), Dir: t.TempDir()}
	installation := "sha256:00000000000000000000000000000000000000000000000000000000000000a1"
	t.Cleanup(func() { _ = k.deleteForTest(installation) })
	_ = k.deleteForTest(installation)
	_, g, err := faa.NewGenesis(installation, "nonce", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Set(context.Background(), installation, nil, g); err != nil {
		// The probe passed, so this is not a missing capability: an unexpected
		// interaction requirement (-25308) or any other failure is a defect.
		t.Fatalf("creating the fixture failed after the Keychain probe passed: %v", err)
	}
	return k, installation, g
}

const (
	opRead = iota
	opOverwrite
	opAdd
	opDelete
)

// The dedicated keychain is locked between operations, so another party that
// does not hold its password can neither overwrite the anchor, nor plant an
// item, nor read it. (Measured against the item access list alone, the same
// overwrite succeeded silently.) Deletion cannot be prevented by the platform;
// it is reported as missing and fails closed.
func TestKeychainAnchorRefusesOverwriteAddAndReadWithoutThePassword(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "locked")
	for name, op := range map[string]int{"overwrite": opOverwrite, "add": opAdd, "read": opRead} {
		account := k.account(installation)
		if op == opAdd {
			account = "faa:planted"
		}
		status, err := k.foreignAttempt(installation, op, account)
		if err != nil {
			t.Fatal(err)
		}
		if status == 0 {
			t.Fatalf("%s without the keychain password must be refused, got success", name)
		}
		t.Logf("%s without the password: OSStatus %d (refused)", name, status)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != g {
		t.Fatalf("the anchor must be untouched by the refused attempts: %+v %v", got, err)
	}
}

func TestKeychainAnchorSilentDeletionFailsClosedAndOnlyTheCeremonyRecovers(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "deleted")
	status, err := k.foreignAttempt(installation, opDelete, k.account(installation))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("deletion without the password: OSStatus %d (the platform does not protect deletion)", status)
	if status == 0 {
		if _, err := k.Load(ctx, installation); !errors.Is(err, faa.ErrMissing) {
			t.Fatalf("a deleted anchor must read as missing: %v", err)
		}
		head, _ := faa.Seal(faa.Fact{Seq: 1, Kind: faa.KindGoalClassified, Installation: installation, Prev: g.Head, GoalID: "g", KernelVersion: "k"})
		next := faa.State{Installation: installation, Seq: 1, Head: head.Head}
		if err := k.Set(ctx, installation, &g, next); !errors.Is(err, faa.ErrMissing) {
			t.Fatalf("a forward step must not succeed against a deleted anchor: %v", err)
		}
	}
	if err := k.Reset(ctx, installation, g); err != nil {
		t.Fatalf("the governed reset must recreate the anchor: %v", err)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != g {
		t.Fatalf("after reset: %+v %v", got, err)
	}
}

func TestKeychainAnchorPasswordItemDamageFailsClosedAsCorruptAndResetRecovers(t *testing.T) {
	ctx := context.Background()
	for name, damage := range map[string]func(k *KeychainAnchor, installation string){
		"password item removed": func(k *KeychainAnchor, installation string) {
			_, _, service, account, _ := k.layout(installation)
			removePassword(service, account)
		},
		"password item replaced": func(k *KeychainAnchor, installation string) {
			_, _, service, account, _ := k.layout(installation)
			_ = storePassword(service, account, "0000000000000000000000000000000000000000000000000000000000000000")
		},
		"password item malformed": func(k *KeychainAnchor, installation string) {
			_, _, service, account, _ := k.layout(installation)
			_ = storePassword(service, account, "short")
		},
	} {
		t.Run(name, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "damage")
			damage(k, installation)
			if _, err := k.Load(ctx, installation); !errors.Is(err, faa.ErrCorrupt) {
				t.Fatalf("damage to the anchor's protection must read as corrupt: %v", err)
			}
			head, _ := faa.Seal(faa.Fact{Seq: 1, Kind: faa.KindGoalClassified, Installation: installation, Prev: g.Head, GoalID: "g", KernelVersion: "k"})
			next := faa.State{Installation: installation, Seq: 1, Head: head.Head}
			if err := k.Set(ctx, installation, &g, next); err == nil {
				t.Fatal("a forward step must not succeed against a damaged anchor")
			}
			if err := k.Reset(ctx, installation, g); err != nil {
				t.Fatalf("the governed reset must recover from damage: %v", err)
			}
			if got, err := k.Load(ctx, installation); err != nil || got != g {
				t.Fatalf("after reset: %+v %v", got, err)
			}
		})
	}
}

func TestKeychainAnchorSessionsAreSerialisedAndLeaveNoTraceAfterCleanup(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "concurrent")
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := k.Load(ctx, installation); err != nil || got != g {
				errs <- errors.New("concurrent load disagreed")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	file, _, _, _, _ := k.layout(installation)
	if err := k.deleteForTest(installation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the test keychain file must be removed: %v", err)
	}
	if _, err := k.Load(ctx, installation); !errors.Is(err, faa.ErrMissing) {
		t.Fatalf("after cleanup the anchor is missing: %v", err)
	}
}

// Removing the keychain file (which the platform cannot prevent) reads as a
// missing anchor, never as a valid one, and a forward step is refused.
func TestKeychainAnchorRemovedFileReadsAsMissing(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "removed")
	file, _, pwService0, pwAccount0, _ := k.layout(installation)
	destroyKeychainFile(file)
	removePassword(pwService0, pwAccount0)
	if _, err := k.Load(ctx, installation); !errors.Is(err, faa.ErrMissing) {
		t.Fatalf("a removed keychain file must read as missing: %v", err)
	}
	// Reading never creates anything: no keychain file, no password item.
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a read must not recreate the keychain file: %v", err)
	}
	_, _, pwService, pwAccount, _ := k.layout(installation)
	if _, err := readPassword(pwService, pwAccount); err == nil || !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("the password item must remain removed after a read (the read made none): %v", err)
	}
	head, _ := faa.Seal(faa.Fact{Seq: 1, Kind: faa.KindGoalClassified, Installation: installation, Prev: g.Head, GoalID: "g", KernelVersion: "k"})
	if err := k.Set(ctx, installation, &g, faa.State{Installation: installation, Seq: 1, Head: head.Head}); !errors.Is(err, faa.ErrMissing) {
		t.Fatalf("a forward step must not succeed against a removed keychain: %v", err)
	}
}

// A keychain that a call created and could not populate is not left behind:
// neither the file nor the password item.
func TestKeychainAnchorCreatedButUnpopulatedKeychainIsRemoved(t *testing.T) {
	requireKeychain(t)
	k := &KeychainAnchor{Service: uniqueService(macOSFAAService + ".test.unpopulated"), Dir: t.TempDir()}
	installation := "sha256:00000000000000000000000000000000000000000000000000000000000000a2"
	t.Cleanup(func() { _ = k.deleteForTest(installation) })
	_ = k.deleteForTest(installation)
	err := k.session(context.Background(), installation, sessionCreate, func(keychainRef, func() error) error { return errors.New("population failed") })
	if err == nil {
		t.Fatal("the failure must be returned")
	}
	if errors.Is(err, faa.ErrUnavailable) {
		t.Fatalf("the Keychain probe passed, yet the session was unavailable: %v", err)
	}
	file, _, pwService, pwAccount, _ := k.layout(installation)
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an unpopulated keychain file must be removed: %v", err)
	}
	if _, err := readPassword(pwService, pwAccount); !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("an unpopulated keychain's password item must be removed: %v", err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	body, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func nextState(t *testing.T, installation string, prev faa.State, goal string) faa.State {
	t.Helper()
	fact, err := faa.Seal(faa.Fact{Seq: prev.Seq + 1, Kind: faa.KindGoalClassified, Installation: installation, Prev: prev.Head, GoalID: goal, KernelVersion: "k"})
	if err != nil {
		t.Fatal(err)
	}
	return faa.State{Installation: installation, Seq: prev.Seq + 1, Head: fact.Head}
}

// replayer models a same-user process that copied the dedicated keychain file as
// opaque bytes earlier and restores those bytes now: a second anchor over the
// same service (so the same, current login Keychain password item) whose file is
// the old copy. Reading through a second path bypasses the OS daemon's cached
// reference to the live file, exactly as Review 6's probe does.
func replayer(t *testing.T, k *KeychainAnchor, installation, snapshot string) *KeychainAnchor {
	t.Helper()
	other := &KeychainAnchor{Service: k.Service, Dir: t.TempDir()}
	file, _, _, _, _ := other.layout(installation)
	copyFile(t, snapshot, file)
	return other
}

// N16 (Review 6): a copy of the dedicated keychain file taken at an earlier
// anchor sequence must not open with the current password item once the anchor
// has advanced, or a retired decision could be restored together with its
// matching database.
func TestKeychainAnchorOpaqueFileReplayIsRefusedOnceTheAnchorHasAdvanced(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "replay")
	live, _, _, _, _ := k.layout(installation)
	snapshot := filepath.Join(t.TempDir(), "seq0.keychain-db")
	copyFile(t, live, snapshot)

	// Control: the copy IS the current file, so it opens with the current password.
	if got, err := replayer(t, k, installation, snapshot).Load(ctx, installation); err != nil || got != g {
		t.Fatalf("control: an unadvanced snapshot must still be the current state: %+v %v", got, err)
	}

	s1 := nextState(t, installation, g, "g1")
	if err := k.Set(ctx, installation, &g, s1); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != s1 {
		t.Fatalf("the advance must be readable: %+v %v", got, err)
	}
	got, err := replayer(t, k, installation, snapshot).Load(ctx, installation)
	if err == nil {
		t.Fatalf("N16: an earlier copy of the keychain file opened with the current password and returned %+v", got)
	}
	if !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("a replayed file must fail closed as corrupt: %v", err)
	}
	// Advancing again through the replayed file must be refused too.
	other := replayer(t, k, installation, snapshot)
	if err := other.Set(ctx, installation, &g, s1); err == nil {
		t.Fatal("a Set through a replayed file must be refused")
	}
	if got, err := k.Load(ctx, installation); err != nil || got != s1 {
		t.Fatalf("the live anchor must be untouched: %+v %v", got, err)
	}
}

// Every advance re-keys, so a snapshot from ANY earlier step is refused and the
// current file keeps working across a long chain.
func TestKeychainAnchorEveryEarlierSnapshotIsRefusedAcrossAChain(t *testing.T) {
	ctx := context.Background()
	k, installation, cur := keychainFixture(t, "chain")
	live, _, _, _, _ := k.layout(installation)
	var snapshots []string
	for i := 0; i < 5; i++ {
		snap := filepath.Join(t.TempDir(), "snap.keychain-db")
		copyFile(t, live, snap)
		snapshots = append(snapshots, snap)
		next := nextState(t, installation, cur, "g")
		if err := k.Set(ctx, installation, &cur, next); err != nil {
			t.Fatal(err)
		}
		cur = next
	}
	if got, err := k.Load(ctx, installation); err != nil || got != cur {
		t.Fatalf("the live file must keep working: %+v %v", got, err)
	}
	for i, snap := range snapshots {
		if got, err := replayer(t, k, installation, snap).Load(ctx, installation); err == nil {
			t.Fatalf("snapshot %d replayed as %+v", i, got)
		}
	}
}

// Undoing a step also re-keys, and the anchor still reads as the restored state.
func TestKeychainAnchorRevertRekeysAndKeepsTheRestoredState(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "revert")
	s1 := nextState(t, installation, g, "g")
	if err := k.Set(ctx, installation, &g, s1); err != nil {
		t.Fatal(err)
	}
	live, _, _, _, _ := k.layout(installation)
	snap := filepath.Join(t.TempDir(), "s1.keychain-db")
	copyFile(t, live, snap)
	if err := k.Revert(ctx, installation, s1, g); err != nil {
		t.Fatal(err)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != g {
		t.Fatalf("revert must restore the earlier state: %+v %v", got, err)
	}
	if got, err := replayer(t, k, installation, snap).Load(ctx, installation); err == nil {
		t.Fatalf("a copy taken before the revert replayed as %+v", got)
	}
}

// A crash between any two steps of a re-key never strands the file, never
// leaves an earlier copy usable, and the next open finishes the job.
func TestKeychainAnchorInterruptedRekeyRecoversAtEveryStep(t *testing.T) {
	ctx := context.Background()
	for _, step := range []string{"start", "pending-stored", "file-rekeyed", "current-updated"} {
		t.Run(step, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "crash")
			live, _, pwService, pwAccount, _ := k.layout(installation)
			before := filepath.Join(t.TempDir(), "before.keychain-db")
			copyFile(t, live, before)
			s1 := nextState(t, installation, g, "g")

			rotationHook = func(at string) error {
				if at == step {
					return errors.New("simulated crash")
				}
				return nil
			}
			// A crash leaves the write in place: nothing runs after it.
			err := k.session(ctx, installation, sessionExisting, func(kc keychainRef, rotate func() error) error {
				if err := k.writeIn(kc, installation, s1, false); err != nil {
					return err
				}
				return rotate()
			})
			rotationHook = nil
			if err == nil {
				t.Fatal("the simulated crash must surface")
			}
			// The next open recovers the anchor, at the advanced state.
			if got, err := k.Load(ctx, installation); err != nil || got != s1 {
				t.Fatalf("after a crash at %q the anchor must be readable at the advanced state: %+v %v", step, got, err)
			}
			if _, present, _ := readOptionalPassword(pwService, pendingAccount(pwAccount)); present {
				t.Fatalf("a completed recovery must leave no pending password behind (crash at %q)", step)
			}
			if got, err := replayer(t, k, installation, before).Load(ctx, installation); err == nil && step != "start" && step != "pending-stored" {
				t.Fatalf("a copy taken before the interrupted step replayed as %+v", got)
			}
			// The chain continues normally.
			s2 := nextState(t, installation, s1, "g2")
			if err := k.Set(ctx, installation, &s1, s2); err != nil {
				t.Fatalf("a Set after recovery: %v", err)
			}
		})
	}
}

// A failed re-key is refused cleanly: the anchor stays where it was.
func TestKeychainAnchorRefusedRekeyLeavesTheAnchorUnchanged(t *testing.T) {
	ctx := context.Background()
	for _, step := range []string{"pending-stored", "file-rekeyed"} {
		t.Run(step, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "refused")
			s1 := nextState(t, installation, g, "g")
			rotationHook = func(at string) error {
				if at == step {
					return errors.New("simulated failure")
				}
				return nil
			}
			err := k.Set(ctx, installation, &g, s1)
			rotationHook = nil
			if err == nil {
				t.Fatal("a Set whose re-key failed must be refused")
			}
			if got, err := k.Load(ctx, installation); err != nil || got != g {
				t.Fatalf("a refused Set must leave the anchor unchanged: %+v %v", got, err)
			}
			if err := k.Set(ctx, installation, &g, s1); err != nil {
				t.Fatalf("the same Set must succeed afterwards: %v", err)
			}
		})
	}
}

// ---- Repair 6: the re-key lifecycle ----

func passwords(t *testing.T, k *KeychainAnchor, installation string) (current string, pending string, havePending bool) {
	t.Helper()
	_, _, service, account, _ := k.layout(installation)
	current, err := readPassword(service, account)
	if err != nil {
		t.Fatalf("the current password item must be readable: %v", err)
	}
	pending, havePending, err = readOptionalPassword(service, pendingAccount(account))
	if err != nil {
		t.Fatalf("the pending password item must be absent or well formed: %v", err)
	}
	return current, pending, havePending
}

func restoreRekeyHooks() {
	rotationHook = nil
	changePassword = realChangePassword
	changePasswordSymbol = "SecKeychainChangePassword"
	rekeyWithoutInteraction = false
}

var realChangePassword = changePassword

// Every successful advance, and every undo, gives the dedicated file a new
// password; no password is ever reused; no pending item outlives a step.
func TestKeychainAnchorEveryAdvanceAndUndoRotatesThePassword(t *testing.T) {
	ctx := context.Background()
	k, installation, cur := keychainFixture(t, "rotates")
	seen := map[string]string{}
	record := func(step string) {
		t.Helper()
		pw, _, havePending := passwords(t, k, installation)
		if len(pw) != 64 {
			t.Fatalf("%s: the password must stay 32 random bytes hex encoded: %q", step, pw)
		}
		if prior, dup := seen[pw]; dup {
			t.Fatalf("%s reused the password of %s", step, prior)
		}
		if havePending {
			t.Fatalf("%s left a pending password behind", step)
		}
		seen[pw] = step
	}
	record("create")
	for i := 1; i <= 4; i++ {
		next := nextState(t, installation, cur, "g")
		if err := k.Set(ctx, installation, &cur, next); err != nil {
			t.Fatal(err)
		}
		record("advance " + string(rune('0'+i)))
		cur = next
	}
	back := faa.State{}
	// Undo the last advance: the state is restored and the password still changes.
	prev := cur
	cur = nextState(t, installation, prev, "g-undone")
	if err := k.Set(ctx, installation, &prev, cur); err != nil {
		t.Fatal(err)
	}
	record("advance to be undone")
	back = prev
	if err := k.Revert(ctx, installation, cur, back); err != nil {
		t.Fatal(err)
	}
	record("undo")
	if got, err := k.Load(ctx, installation); err != nil || got != back {
		t.Fatalf("undo must restore the earlier state: %+v %v", got, err)
	}
	// A refused (conflicting) Set changes nothing, including the password.
	before, _, _ := passwords(t, k, installation)
	if err := k.Set(ctx, installation, &cur, nextState(t, installation, cur, "x")); !errors.Is(err, faa.ErrConflict) {
		t.Fatalf("a stale expectation must conflict: %v", err)
	}
	if after, _, _ := passwords(t, k, installation); after != before {
		t.Fatal("a refused Set must not re-key")
	}
	// A read never re-keys.
	if _, err := k.Load(ctx, installation); err != nil {
		t.Fatal(err)
	}
	if after, _, _ := passwords(t, k, installation); after != before {
		t.Fatal("a Load must not re-key")
	}
}

// Undoing a step must not let a copy taken before it, or during it, be replayed,
// and an interrupted undo recovers at every boundary.
func TestKeychainAnchorInterruptedUndoRecoversAtEveryStep(t *testing.T) {
	ctx := context.Background()
	for _, step := range []string{"start", "pending-stored", "file-rekeyed", "current-updated"} {
		t.Run(step, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "undo-crash")
			s1 := nextState(t, installation, g, "g")
			if err := k.Set(ctx, installation, &g, s1); err != nil {
				t.Fatal(err)
			}
			live, _, _, _, _ := k.layout(installation)
			snap := filepath.Join(t.TempDir(), "s1.keychain-db")
			copyFile(t, live, snap)
			rotationHook = func(at string) error {
				if at == step {
					return errors.New("simulated crash")
				}
				return nil
			}
			err := k.Revert(ctx, installation, s1, g)
			rotationHook = nil
			if err == nil || !errors.Is(err, faa.ErrUnavailable) {
				t.Fatalf("an undo whose re-key stops must report unavailable: %v", err)
			}
			if got, err := k.Load(ctx, installation); err != nil || got != g {
				t.Fatalf("after a crash at %q the undone state must be readable: %+v %v", step, got, err)
			}
			if _, _, havePending := passwords(t, k, installation); havePending {
				t.Fatalf("recovery must leave no pending password (crash at %q)", step)
			}
			if got, err := replayer(t, k, installation, snap).Load(ctx, installation); err == nil && step != "start" && step != "pending-stored" {
				t.Fatalf("a copy taken before the interrupted undo replayed as %+v", got)
			}
		})
	}
}

// A re-key that fails, at any step, never advances, regresses or strands the
// anchor: the Set is refused, the anchor is exactly where it was, and the same
// Set then succeeds.
func TestKeychainAnchorFailedRekeyNeverAdvancesOrStrandsTheAnchor(t *testing.T) {
	ctx := context.Background()
	for _, step := range []string{"start", "pending-stored", "file-rekeyed", "current-updated"} {
		t.Run(step, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "failed-rekey")
			s1 := nextState(t, installation, g, "g")
			before, _, _ := passwords(t, k, installation)
			rotationHook = func(at string) error {
				if at == step {
					return errors.New("simulated failure")
				}
				return nil
			}
			err := k.Set(ctx, installation, &g, s1)
			rotationHook = nil
			if !errors.Is(err, faa.ErrUnavailable) {
				t.Fatalf("a failed re-key must be reported as unavailable: %v", err)
			}
			if got, err := k.Load(ctx, installation); err != nil || got != g {
				t.Fatalf("a refused Set must leave the anchor exactly where it was: %+v %v", got, err)
			}
			if _, _, havePending := passwords(t, k, installation); havePending {
				t.Fatal("recovery must leave no pending password")
			}
			// The first refusal was a re-key failure; the snapshot from before it must
			// not open once the same step succeeds.
			live, _, _, _, _ := k.layout(installation)
			old := filepath.Join(t.TempDir(), "g.keychain-db")
			copyFile(t, live, old)
			if err := k.Set(ctx, installation, &g, s1); err != nil {
				t.Fatalf("the same Set must succeed afterwards: %v", err)
			}
			if after, _, _ := passwords(t, k, installation); after == before {
				t.Fatal("the successful Set must have re-keyed")
			}
			if got, err := replayer(t, k, installation, old).Load(ctx, installation); err == nil {
				t.Fatalf("a copy of the refused step's file replayed as %+v", got)
			}
		})
	}
}

// When the advance cannot be taken back after a failed re-key, the anchor is
// one step ahead of the store: the stranded fail-closed state, readable and
// valid, never a wrong or unreadable one. The error says so.
func TestKeychainAnchorFailedRekeyThatCannotBeUndoneIsTheStrandedAheadState(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "stranded")
	s1 := nextState(t, installation, g, "g")
	rotationHook = func(at string) error {
		if at == "file-rekeyed" || at == "undo" {
			return errors.New("simulated failure")
		}
		return nil
	}
	err := k.Set(ctx, installation, &g, s1)
	rotationHook = nil
	if !errors.Is(err, faa.ErrUnavailable) {
		t.Fatalf("must be unavailable: %v", err)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != s1 {
		t.Fatalf("the stranded anchor must read as exactly the advanced state (ahead of the store, which refuses it): %+v %v", got, err)
	}
	if _, _, havePending := passwords(t, k, installation); havePending {
		t.Fatal("recovery must leave no pending password")
	}
}

// SecKeychainChangePassword is an exported but undocumented Security.framework
// entry point. These tests establish what the evidence does and does not show.
func TestKeychainAnchorRekeyDependsOnAnUndocumentedEntryPointAndFailsClosedWithoutIt(t *testing.T) {
	ctx := context.Background()
	requireRekey(t)
	t.Logf("SecKeychainChangePassword resolves via dlsym on this OS")
	for name, status := range map[string]int{
		"symbol missing (errSecUnimplemented)":         -4,
		"prompt needed but not allowed (-25308)":       -25308,
		"user cancelled the prompt (-128)":             -128,
		"authorization failed (-25293)":                -25293,
		"framework error (errSecInternalError -26276)": -26276,
	} {
		t.Run(name, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "platform")
			s1 := nextState(t, installation, g, "g")
			changePassword = func(keychainRef, string, string) int { return status }
			err := k.Set(ctx, installation, &g, s1)
			changePassword = realChangePassword
			if !errors.Is(err, faa.ErrUnavailable) {
				t.Fatalf("a platform refusal of the re-key must be unavailable, not success and not corruption: %v", err)
			}
			if errors.Is(err, faa.ErrCorrupt) {
				t.Fatal("an unavailable platform must not be classified as a corrupt anchor")
			}
			if _, _, havePending := passwords(t, k, installation); havePending {
				t.Fatal("a refused re-key must not leave a pending password")
			}
			if got, err := k.Load(ctx, installation); err != nil || got != g {
				t.Fatalf("the anchor must stay where it was: %+v %v", got, err)
			}
			// Availability failures are never masked: with the platform back, the same
			// Set succeeds.
			if err := k.Set(ctx, installation, &g, s1); err != nil {
				t.Fatalf("after the platform recovers the Set must succeed: %v", err)
			}
		})
	}
}

// Measured, not assumed: the real re-key needs no user interaction on this
// machine (it succeeds with interaction disabled), so a non-interactive session
// is not blocked by the re-key itself. Other Keychain operations, such as the
// per-build access prompt for the password item, are unchanged.
func TestKeychainAnchorRealRekeyNeedsNoUserInteraction(t *testing.T) {
	ctx := context.Background()
	requireRekey(t)
	k, installation, g := keychainFixture(t, "noninteractive")
	rekeyWithoutInteraction = true
	defer restoreRekeyHooks()
	s1 := nextState(t, installation, g, "g")
	if err := k.Set(ctx, installation, &g, s1); err != nil {
		// The probe passed and interaction is disabled for the whole process: a
		// refusal here means the re-key itself needs a person. That is a limitation
		// to record, never a skip.
		t.Fatalf("with interaction disabled the real re-key was refused: %v", err)
	}
	if got, err := k.Load(ctx, installation); err != nil || got != s1 {
		t.Fatalf("%+v %v", got, err)
	}
}

// A governed re-anchor is the only recovery from a replayed file; it re-creates
// the pair with a fresh password, installs exactly the state the ceremony gives
// it, removes any pending password, and does not resurrect the replayed state.
func TestKeychainAnchorOnlyTheGovernedResetRecoversAReplayedFileAndItManufacturesNothing(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "reset-after-replay")
	live, _, _, _, _ := k.layout(installation)
	snap := filepath.Join(t.TempDir(), "g.keychain-db")
	copyFile(t, live, snap)
	s1 := nextState(t, installation, g, "g")
	if err := k.Set(ctx, installation, &g, s1); err != nil {
		t.Fatal(err)
	}
	liveAnchor := k
	// The earlier file is restored next to the current password item. It is
	// reopened through a second path, as in Review 6, to bypass the OS daemon's
	// cached reference to the live file.
	k = replayer(t, k, installation, snap)
	// Every ordinary operation refuses the replayed pair as corrupt.
	if _, err := k.Load(ctx, installation); !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("a replayed file must read as corrupt: %v", err)
	}
	if err := k.Set(ctx, installation, &s1, nextState(t, installation, s1, "x")); !errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("no forward step may pass through a replayed file: %v", err)
	}
	if err := k.Revert(ctx, installation, s1, g); err == nil {
		t.Fatal("an undo must not pass through a replayed file")
	}
	// A pending item planted next to the damaged pair is discarded with it.
	_, _, pwService, pwAccount, _ := k.layout(installation)
	if err := storePassword(pwService, pendingAccount(pwAccount), "1111111111111111111111111111111111111111111111111111111111111111"); err != nil {
		t.Fatal(err)
	}
	// The ceremony recovers, and installs exactly the state it was given.
	fresh := nextState(t, installation, s1, "reanchor")
	if err := k.Reset(ctx, installation, fresh); err != nil {
		t.Fatal(err)
	}
	// Checked BEFORE any read: an open would discard a stray pending item anyway.
	if _, _, havePending := passwords(t, k, installation); havePending {
		t.Fatal("a reset must leave no pending password")
	}
	if got, err := k.Load(ctx, installation); err != nil || got != fresh {
		t.Fatalf("the ceremony's state, exactly: %+v %v", got, err)
	}
	// The replayed file stays refused after the reset (a fresh password): the
	// ceremony did not make the replay valid.
	if got, err := replayer(t, liveAnchor, installation, snap).Load(ctx, installation); err == nil {
		t.Fatalf("the replayed file must stay refused after the ceremony, read as %+v", got)
	}
}

type passwordKind int

const (
	pwRight     passwordKind = iota // the password the live file is keyed to
	pwAbsent                        // no item
	pwStale                         // the password an earlier copy of the file is keyed to
	pwJunk                          // well formed, keys nothing
	pwForeign                       // another installation's real password
	pwMalformed                     // not 32 random bytes hex encoded
)

func (p passwordKind) String() string {
	return [...]string{"right", "absent", "stale", "junk", "foreign", "malformed"}[p]
}

// The password items are inputs an attacker (or a crash) can shape. Enumerating
// every combination of current and pending shows what opens the live file and
// what opens a replayed file: nothing but the right password may open the live
// file, and the pending item is only ever a way to finish an interrupted re-key
// of THIS file, never a way to open anything else or to reach an earlier state.
func TestKeychainAnchorCurrentAndPendingPasswordStateIsFailClosedForEveryCombination(t *testing.T) {
	ctx := context.Background()
	kinds := []passwordKind{pwRight, pwAbsent, pwStale, pwJunk, pwForeign, pwMalformed}
	const junk = "1111111111111111111111111111111111111111111111111111111111111111"
	for _, replay := range []bool{false, true} {
		for _, cur := range kinds {
			for _, pend := range kinds {
				if pend == pwRight && cur == pwRight {
					continue
				}
				name := "live"
				if replay {
					name = "replayed"
				}
				t.Run(name+"/current="+cur.String()+"/pending="+pend.String(), func(t *testing.T) {
					k, installation, g := keychainFixture(t, "matrix")
					_, _, service, account, _ := k.layout(installation)
					stale, _, _ := passwords(t, k, installation)
					live, _, _, _, _ := k.layout(installation)
					snap := filepath.Join(t.TempDir(), "g.keychain-db")
					copyFile(t, live, snap)
					s1 := nextState(t, installation, g, "g")
					if err := k.Set(ctx, installation, &g, s1); err != nil {
						t.Fatal(err)
					}
					right, _, _ := passwords(t, k, installation)
					// A foreign installation's real password.
					other := "sha256:00000000000000000000000000000000000000000000000000000000000000b2"
					_, og, _ := faa.NewGenesis(other, "n", time.Now())
					if err := k.Set(ctx, other, nil, og); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = k.deleteForTest(other) })
					foreign, _, _ := passwords(t, k, other)

					value := func(kind passwordKind) (string, bool) {
						switch kind {
						case pwRight:
							return right, true
						case pwStale:
							return stale, true
						case pwJunk:
							return junk, true
						case pwForeign:
							return foreign, true
						case pwMalformed:
							return "short", true
						}
						return "", false
					}
					removePassword(service, account) // both items
					if v, ok := value(cur); ok {
						if err := storePassword(service, account, v); err != nil {
							t.Fatal(err)
						}
					}
					if v, ok := value(pend); ok {
						if err := storePassword(service, pendingAccount(account), v); err != nil {
							t.Fatal(err)
						}
					}

					target := k
					wantState := s1
					if replay {
						target = replayer(t, k, installation, snap)
						wantState = g
					}
					// What opens the file: the password it is keyed to. For the live file that
					// is `right`; for the replayed file it is `stale`. The current item is
					// consulted first, the pending item only when the current one cannot be
					// read or does not unlock; a malformed current item is refused outright.
					keyed := right
					if replay {
						keyed = stale
					}
					curV, curOK := value(cur)
					pendV, _ := value(pend)
					opens := false
					switch {
					case cur == pwMalformed:
						opens = false
					case curOK && curV == keyed:
						opens = true
					case pend != pwAbsent && pend != pwMalformed && pendV == keyed:
						opens = true
					}
					got, err := target.Load(ctx, installation)
					if opens {
						if err != nil || got != wantState {
							t.Fatalf("expected the file to open at %+v, got %+v %v", wantState, got, err)
						}
						if _, _, havePending := passwords(t, target, installation); havePending {
							t.Fatal("a successful open must leave no pending password (finished or discarded)")
						}
						return
					}
					if err == nil {
						t.Fatalf("this password state must not open the file, yet it read %+v", got)
					}
					// The file exists, so an unusable password state is a CORRUPT anchor;
					// "missing" would mean the anchor did not exist.
					if !errors.Is(err, faa.ErrCorrupt) {
						t.Fatalf("an unusable password state must fail closed as corrupt: %v", err)
					}
					// A refusal changes nothing: neither item is promoted, replaced or removed.
					for _, item := range []struct {
						account string
						kind    passwordKind
					}{{account, cur}, {pendingAccount(account), pend}} {
						got, present, rerr := readOptionalPassword(service, item.account)
						want, wantOK := value(item.kind)
						switch {
						case item.kind == pwAbsent:
							if present || rerr != nil {
								t.Fatalf("a refusal must not create the %s item", item.account)
							}
						case item.kind == pwMalformed:
							if rerr == nil {
								t.Fatalf("a refusal must not repair the malformed %s item", item.account)
							}
						case !wantOK || rerr != nil || got != want:
							t.Fatalf("a refusal must leave the %s item exactly as it was", item.account)
						}
					}
					if err := target.Set(ctx, installation, &wantState, nextState(t, installation, wantState, "x")); err == nil {
						t.Fatal("no forward step may pass through an unusable password state")
					}
				})
			}
		}
	}
}

// Characterisation of the accepted trust boundary (R-K3 / R-K2), not a
// protection: a holder of an EARLIER password who can also plant password items
// can reopen the earlier file whether the password is planted as the current
// item or as the pending one. The pending item therefore grants nothing that the
// ability to write the current item does not already grant, and without the
// earlier password neither item does anything (the matrix above).
func TestKeychainAnchorPendingPasswordGrantsNothingBeyondTheCurrentItem(t *testing.T) {
	ctx := context.Background()
	for _, where := range []string{"current", "pending"} {
		t.Run(where, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "boundary")
			_, _, service, account, _ := k.layout(installation)
			stale, _, _ := passwords(t, k, installation)
			live, _, _, _, _ := k.layout(installation)
			snap := filepath.Join(t.TempDir(), "g.keychain-db")
			copyFile(t, live, snap)
			if err := k.Set(ctx, installation, &g, nextState(t, installation, g, "g")); err != nil {
				t.Fatal(err)
			}
			target := account
			if where == "pending" {
				target = pendingAccount(account)
			}
			if err := storePassword(service, target, stale); err != nil {
				t.Fatal(err)
			}
			got, err := replayer(t, k, installation, snap).Load(ctx, installation)
			t.Logf("holder of the earlier password, planted as the %s item: %+v %v (accepted boundary: knowledge of the earlier password plus write access to the login Keychain)", where, got, err)
			if err != nil {
				t.Fatalf("the characterised boundary changed: %v", err)
			}
		})
	}
}

// On a macOS that no longer exports the entry point, the core still runs (the
// symbol is resolved at run time, not linked), every re-key is refused as
// unavailable, and no anchor advance completes: governance stops advancing
// rather than continuing without freshness. Modelled by naming a symbol that
// does not exist, since the real one cannot be removed from a running system.
func TestKeychainAnchorMissingRekeyEntryPointRefusesEveryAdvanceAndStillReads(t *testing.T) {
	ctx := context.Background()
	k, installation, g := keychainFixture(t, "nosymbol")
	changePasswordSymbol = "SecKeychainChangePasswordDoesNotExist"
	defer restoreRekeyHooks()
	if rekeySupported() {
		t.Fatal("the modelled symbol must not resolve")
	}
	s1 := nextState(t, installation, g, "g")
	err := k.Set(ctx, installation, &g, s1)
	if !errors.Is(err, faa.ErrUnavailable) || errors.Is(err, faa.ErrCorrupt) {
		t.Fatalf("a missing entry point must refuse the advance as unavailable: %v", err)
	}
	if _, _, havePending := passwords(t, k, installation); havePending {
		t.Fatal("a refused re-key must not leave a pending password")
	}
	if got, err := k.Load(ctx, installation); err != nil || got != g {
		t.Fatalf("reading needs no re-key and must keep working: %+v %v", got, err)
	}
	if err := k.Revert(ctx, installation, g, g); err == nil {
		t.Fatal("an undo needs a re-key too and must be refused")
	}
	changePasswordSymbol = "SecKeychainChangePassword"
	if err := k.Set(ctx, installation, &g, s1); err != nil {
		t.Fatalf("with the entry point back the same advance must succeed: %v", err)
	}
}

// Independently built test and mutation binaries share nothing: every service
// identity is unique to its process and call.
func TestUniqueServiceIdentitiesNeverCollide(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		s := uniqueService(macOSFAAService + ".test")
		if seen[s] {
			t.Fatalf("service %q was issued twice", s)
		}
		seen[s] = true
	}
	if a, b := uniqueService("x"), uniqueService("x"); a == b {
		t.Fatal("two calls must differ")
	}
}

// After a crash between the file's re-key and the current password item, the
// FIRST operation may be a write: it must promote the pending password AND
// re-key from that password (not from the stale current one), so the advance
// succeeds and the file is re-keyed again. Same for an undo.
func TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword(t *testing.T) {
	ctx := context.Background()
	for _, op := range []string{"advance", "undo"} {
		t.Run(op, func(t *testing.T) {
			k, installation, g := keychainFixture(t, "promote-write")
			s1 := nextState(t, installation, g, "g")
			if err := k.Set(ctx, installation, &g, s1); err != nil {
				t.Fatal(err)
			}
			s2 := nextState(t, installation, s1, "g2")
			rotationHook = func(at string) error {
				if at == "file-rekeyed" {
					return errors.New("simulated crash")
				}
				return nil
			}
			// A crash inside the advance: the file is re-keyed, the current item is not.
			err := k.session(ctx, installation, sessionExisting, func(kc keychainRef, rotate func() error) error {
				if err := k.writeIn(kc, installation, s2, false); err != nil {
					return err
				}
				return rotate()
			})
			rotationHook = nil
			if err == nil {
				t.Fatal("the simulated crash must surface")
			}
			_, pendingBefore, havePending := passwords(t, k, installation)
			if !havePending {
				t.Fatal("the crash must leave a pending password for the next open to promote")
			}
			// The very next operation is a write, with no read in between.
			var want faa.State
			switch op {
			case "advance":
				s3 := nextState(t, installation, s2, "g3")
				if err := k.Set(ctx, installation, &s2, s3); err != nil {
					t.Fatalf("an advance right after an interrupted re-key must succeed: %v", err)
				}
				want = s3
			case "undo":
				if err := k.Revert(ctx, installation, s2, s1); err != nil {
					t.Fatalf("an undo right after an interrupted re-key must succeed: %v", err)
				}
				want = s1
			}
			if got, err := k.Load(ctx, installation); err != nil || got != want {
				t.Fatalf("%+v %v", got, err)
			}
			after, _, stillPending := passwords(t, k, installation)
			if stillPending {
				t.Fatal("no pending password may remain")
			}
			if after == pendingBefore {
				t.Fatal("the write must have re-keyed again, away from the promoted password")
			}
		})
	}
}
