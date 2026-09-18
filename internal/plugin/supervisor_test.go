package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/scheduler"
)

type fakeProcessControl struct {
	starts       int
	terminates   int
	startErr     error
	terminateErr error
}

type observedFakeProcessControl struct {
	fakeProcessControl
	handler func(error)
}

func (f *observedFakeProcessControl) SetExitHandler(_ InstanceIdentity, handler func(error)) {
	f.handler = handler
}

type fakeSupervisorPersistence struct {
	mu        sync.Mutex
	snapshots []SupervisorSnapshot
}

type fakeSupervisorResourceLeaser struct {
	acquires        int
	releases        int
	attemptReleases int
	lastSliceID     string
	lastAttemptID   string
	leaseErr        error
	releaseErr      error
	leases          []scheduler.ResourceLease
}

func (f *fakeSupervisorResourceLeaser) AcquireSchedulerResourceLeases(_ context.Context, sliceID, attemptID string, requirements []scheduler.ResourceRequirement, now time.Time, expiry *time.Time) ([]scheduler.ResourceLease, error) {
	f.acquires++
	if f.leaseErr != nil {
		return nil, f.leaseErr
	}
	if len(f.leases) != 0 {
		leases := append([]scheduler.ResourceLease(nil), f.leases...)
		for i := range leases {
			if leases[i].SliceID == "" {
				leases[i].SliceID = sliceID
			}
			if leases[i].AttemptID == "" {
				leases[i].AttemptID = attemptID
			}
		}
		return leases, nil
	}
	leases := make([]scheduler.ResourceLease, 0, len(requirements))
	for _, req := range requirements {
		lease := scheduler.ResourceLease{ID: sliceID + ":" + attemptID + ":" + req.Key, SliceID: sliceID, AttemptID: attemptID, ResourceKey: req.Key, Capacity: req.Capacity, AcquiredAt: now}
		if expiry != nil {
			value := *expiry
			lease.ExpiresAt = &value
		}
		leases = append(leases, lease)
	}
	return leases, nil
}

func (f *fakeSupervisorResourceLeaser) ReleaseSchedulerResourceLeases(context.Context, []string, time.Time) error {
	f.releases++
	return f.releaseErr
}

func (f *fakeSupervisorResourceLeaser) ReleaseSchedulerResourceLeasesForAttempt(_ context.Context, sliceID, attemptID string, _ time.Time) error {
	f.releases++
	f.attemptReleases++
	f.lastSliceID = sliceID
	f.lastAttemptID = attemptID
	return f.releaseErr
}

func (f *fakeSupervisorPersistence) LoadSupervisorSnapshots(context.Context) ([]SupervisorSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SupervisorSnapshot(nil), f.snapshots...), nil
}

func (f *fakeSupervisorPersistence) SaveSupervisorSnapshot(_ context.Context, snapshot SupervisorSnapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots = []SupervisorSnapshot{snapshot}
	return nil
}

func (f *fakeSupervisorPersistence) latest() []SupervisorSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SupervisorSnapshot(nil), f.snapshots...)
}

func TestPersistentSupervisorRestoresLaunchButRequiresFreshHandshake(t *testing.T) {
	provider := fixtureProvider("persistent", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	bytes := []byte("verified")
	digest := sha256.Sum256(bytes)
	provider.Manifest.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	provider.Identity.ArtifactDigest = provider.Manifest.ArtifactDigest
	persist := &fakeSupervisorPersistence{snapshots: []SupervisorSnapshot{{Provider: ProviderSnapshot{Manifest: provider.Manifest, Identity: provider.Identity, State: StateReady, Isolation: provider.Isolation, Priority: provider.Priority}, Launch: LaunchSnapshot{Executable: bytes, Entrypoint: provider.Manifest.Entrypoint, SocketPath: "/tmp/plugin.sock"}}}}
	s, err := NewPersistentSupervisor(context.Background(), NewRegistry(), &fakeProcessControl{}, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := s.State(provider.Identity.InstanceID)
	if !ok || state != StateStopped {
		t.Fatalf("restored runtime must require a fresh start/handshake, got %q %v", state, ok)
	}
	if len(persist.snapshots) != 1 || persist.snapshots[0].Provider.State != StateStopped {
		t.Fatalf("restart normalization must be durably persisted, got %+v", persist.snapshots)
	}
}

func TestPersistentSupervisorFencesRestoredSchedulerLeases(t *testing.T) {
	provider := fixtureProvider("restored-lease", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	lease := scheduler.ResourceLease{ID: "abandoned-lease", SliceID: provider.Identity.InstanceID, AttemptID: "old-attempt", ResourceKey: "cpu", Capacity: 1}
	persist := &fakeSupervisorPersistence{snapshots: []SupervisorSnapshot{{
		Provider:       ProviderSnapshot{Manifest: provider.Manifest, Identity: provider.Identity, State: StateReady, Isolation: provider.Isolation, Priority: provider.Priority},
		ResourceLeases: []scheduler.ResourceLease{lease},
	}}}
	leaser := &fakeSupervisorResourceLeaser{}
	if _, err := NewPersistentSupervisor(context.Background(), NewRegistry(), &fakeProcessControl{}, SupervisorPolicy{ResourceLeaser: leaser}, persist); err != nil {
		t.Fatal(err)
	}
	if leaser.releases != 1 {
		t.Fatalf("restart must fence abandoned scheduler leases, got %d releases", leaser.releases)
	}
	snapshots := persist.latest()
	if len(snapshots) != 1 || len(snapshots[0].ResourceLeases) != 0 || snapshots[0].Provider.State != StateStopped {
		t.Fatalf("restart must persist stopped state without old leases: %+v", snapshots)
	}
}

func (f *fakeProcessControl) Start(_ context.Context, _ LaunchSpec) error {
	f.starts++
	return f.startErr
}

func (f *fakeProcessControl) Terminate(_ context.Context, _ InstanceIdentity) error {
	f.terminates++
	return f.terminateErr
}

func installedProvider(id string, isolation map[IsolationProperty]EnforcementState) Provider {
	p := fixtureProvider(id, StateReady, isolation, 1)
	p.State = StateInstalled
	return p
}

func TestSupervisorRequiresIsolationBeforeStart(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{RequiredIsolation: []IsolationProperty{IsolationFilesystem, IsolationNetwork}})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("p", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced, IsolationNetwork: Unknown})
	if err := s.Register(p); err == nil {
		t.Fatal("unknown required isolation must fail closed")
	}
	if process.starts != 0 {
		t.Fatal("process must not start without required isolation")
	}
}

func TestSupervisorQuarantinesAfterFailureThreshold(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{startErr: errors.New("boom")}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{MaxConsecutiveFailures: 2})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("p", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.Start(context.Background(), p.Identity.InstanceID, now); err == nil {
		t.Fatal("failed process start must surface")
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateFailed {
		t.Fatalf("expected failed, got %s", state)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, now.Add(time.Second)); err == nil {
		t.Fatal("second failed start must surface")
	}
	state, _ := s.State(p.Identity.InstanceID)
	if state != StateQuarantined {
		t.Fatalf("expected quarantined, got %s", state)
	}
	if process.terminates != 0 {
		t.Fatalf("failed starts must not terminate absent processes, got %d terminations", process.terminates)
	}
}

func TestSupervisorStartFailurePreservesLaunchErrorWithoutTermination(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{startErr: errors.New("verified executable failed to start")}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("start-error", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	err = s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "verified executable failed to start") {
		t.Fatalf("launch error was not preserved: %v", err)
	}
	if process.terminates != 0 {
		t.Fatalf("absent failed launch must not be terminated, got %d", process.terminates)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateFailed {
		t.Fatalf("failed launch must persist failed state, got %q", state)
	}
}

func TestSupervisorClearsExitObserverWhenStartFails(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{fakeProcessControl: fakeProcessControl{startErr: errors.New("boom")}}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("start-failure", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err == nil {
		t.Fatal("failed process start must surface")
	}
	if process.handler != nil {
		t.Fatal("failed start must not retain an exit observer")
	}
}

func TestSupervisorRevokeRemovesProviderAndTerminates(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	s, _ := NewSupervisor(registry, process, SupervisorPolicy{})
	p := installedProvider("p", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	handshake := HandshakeResult{Provider: p}
	handshake.Provider.State = StateStarting
	if err := s.MarkReady(p.Identity.InstanceID, handshake); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("workspace.search.text", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(context.Background(), p.Identity.InstanceID); err != nil {
		t.Fatal(err)
	}
	if process.terminates != 1 {
		t.Fatalf("expected forced termination, got %d", process.terminates)
	}
	if _, err := registry.Resolve("workspace.search.text", nil); !errors.Is(err, ErrNoEligibleProvider) {
		t.Fatalf("revoked provider still eligible: %v", err)
	}
}

func TestSupervisorRevokeStoppedProviderDoesNotTerminateAbsentProcess(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("stopped-revoke", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	p.State = StateStopped
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	// A stopped instance has no child to terminate, but revocation must remain
	// authoritative and must not manufacture a cleanup failure.
	if err := s.Revoke(context.Background(), p.Identity.InstanceID); err != nil {
		t.Fatal(err)
	}
	if process.terminates != 0 {
		t.Fatalf("revoking a non-running instance must not terminate, got %d", process.terminates)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateRevoked {
		t.Fatalf("expected revoked state, got %q", state)
	}
	if snapshots := persist.latest(); len(snapshots) != 1 || snapshots[0].Provider.State != StateRevoked {
		t.Fatalf("revocation was not durably persisted: %+v", snapshots)
	}
}

func TestSupervisorRevokeTerminationFailureStaysRevoked(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{terminateErr: errors.New("kill denied")}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("revoke-failure", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	handshake := HandshakeResult{Provider: p}
	handshake.Provider.State = StateStarting
	handshake.Provider.advertisementValidated = true
	if err := s.MarkReady(p.Identity.InstanceID, handshake); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(context.Background(), p.Identity.InstanceID); err == nil || !strings.Contains(err.Error(), "kill denied") {
		t.Fatalf("termination failure was not surfaced: %v", err)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateRevoked {
		t.Fatalf("failed revocation must remain revoked, got %q", state)
	}
}

func TestSupervisorRejectsCallerConstructedReadyState(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("p", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	forged := HandshakeResult{Provider: p}
	forged.Provider.State = StateReady
	forged.Provider.advertisementValidated = false
	if err := s.MarkReady(p.Identity.InstanceID, forged); err == nil {
		t.Fatal("caller-selected ready state must not publish a provider")
	}
	if _, err := registry.Resolve("workspace.search.text", nil); !errors.Is(err, ErrNoEligibleProvider) {
		t.Fatalf("forged readiness must not make capability routable: %v", err)
	}
}

func TestSupervisorRestartBackoff(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{startErr: errors.New("boom")}
	s, _ := NewSupervisor(registry, process, SupervisorPolicy{MaxConsecutiveFailures: 3, RestartBackoff: time.Minute})
	p := installedProvider("p", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	_ = s.Start(context.Background(), p.Identity.InstanceID, now)
	if err := s.Start(context.Background(), p.Identity.InstanceID, now.Add(10*time.Second)); err == nil {
		t.Fatal("restart within backoff must fail")
	}
	if process.starts != 1 {
		t.Fatalf("backoff must prevent second process start; got %d", process.starts)
	}
}

func TestSupervisorResourceAdmissionGatesProcessAndReleasesOnStop(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	leaser := &fakeSupervisorResourceLeaser{leases: []scheduler.ResourceLease{{ID: "lease-1", ResourceKey: "cpu", Capacity: 1}}}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{
		ResourceRequirements: []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}},
		ResourceLeaser:       leaser,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("resource-gated", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if leaser.acquires != 1 || process.starts != 1 {
		t.Fatalf("resource admission did not precede launch: acquires=%d starts=%d", leaser.acquires, process.starts)
	}
	if err := s.Stop(context.Background(), p.Identity.InstanceID); err != nil {
		t.Fatal(err)
	}
	if leaser.releases != 1 {
		t.Fatalf("stop must release the process attempt lease, got %d releases", leaser.releases)
	}
	if leaser.attemptReleases != 1 || leaser.lastSliceID != p.Identity.InstanceID || leaser.lastAttemptID == "" {
		t.Fatalf("stop must release the exact scheduler attempt, got slice=%q attempt=%q count=%d", leaser.lastSliceID, leaser.lastAttemptID, leaser.attemptReleases)
	}
}

func TestSupervisorRejectsPartialOrForeignResourceAdmission(t *testing.T) {
	for name, leases := range map[string][]scheduler.ResourceLease{
		"partial": {{ID: "lease-cpu", SliceID: "resource-partial", AttemptID: "resource-partial:session", ResourceKey: "cpu", Capacity: 1}},
		"foreign": {{ID: "lease-foreign", SliceID: "other", AttemptID: "other", ResourceKey: "cpu", Capacity: 1}, {ID: "lease-memory", SliceID: "resource-foreign", AttemptID: "resource-foreign:session", ResourceKey: "memory", Capacity: 1}},
	} {
		t.Run(name, func(t *testing.T) {
			registry := NewRegistry()
			process := &fakeProcessControl{}
			leaser := &fakeSupervisorResourceLeaser{leases: leases}
			s, err := NewSupervisor(registry, process, SupervisorPolicy{
				ResourceRequirements: []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}, {Key: "memory", Capacity: 1}},
				ResourceLeaser:       leaser,
			})
			if err != nil {
				t.Fatal(err)
			}
			p := installedProvider("resource-"+name, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
			if err := s.Register(p); err != nil {
				t.Fatal(err)
			}
			if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err == nil {
				t.Fatal("invalid admission must fail closed")
			}
			if process.starts != 0 {
				t.Fatalf("invalid admission must not launch process, got %d starts", process.starts)
			}
			if state, _ := s.State(p.Identity.InstanceID); state != StateInstalled {
				t.Fatalf("invalid admission must restore prior state, got %q", state)
			}
		})
	}
}

func TestSupervisorFailurePersistsReleasedResourceLeases(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	leaser := &fakeSupervisorResourceLeaser{leases: []scheduler.ResourceLease{{ID: "lease-failure", ResourceKey: "cpu", Capacity: 1}}}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{
		ResourceRequirements: []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}},
		ResourceLeaser:       leaser,
	}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("resource-failure", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFailure(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	snapshots := persist.latest()
	if len(snapshots) != 1 || len(snapshots[0].ResourceLeases) != 0 {
		t.Fatalf("failed lifecycle must persist released leases, got %+v", snapshots)
	}
}

func TestSupervisorUnexpectedExitPersistsReleasedResourceLeases(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{}
	leaser := &fakeSupervisorResourceLeaser{leases: []scheduler.ResourceLease{{ID: "lease-crash", ResourceKey: "cpu", Capacity: 1}}}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{
		ResourceRequirements: []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}},
		ResourceLeaser:       leaser,
	}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("resource-crash", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	process.handler(errors.New("crash"))
	snapshots := persist.latest()
	if len(snapshots) != 1 || len(snapshots[0].ResourceLeases) != 0 {
		t.Fatalf("unexpected exit must persist released leases, got %+v", snapshots)
	}
}

func TestSupervisorResourceDenialDoesNotStartProcess(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	leaser := &fakeSupervisorResourceLeaser{leaseErr: errors.New("capacity unavailable")}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{
		ResourceRequirements: []scheduler.ResourceRequirement{{Key: "gpu", Capacity: 1, Exclusive: true}},
		ResourceLeaser:       leaser,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("resource-denied", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err == nil || !strings.Contains(err.Error(), "capacity unavailable") {
		t.Fatalf("resource denial was not surfaced: %v", err)
	}
	if process.starts != 0 {
		t.Fatalf("resource denial must prevent process launch, got %d starts", process.starts)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateInstalled {
		t.Fatalf("resource denial must restore pre-admission state, got %q", state)
	}
}

func TestSupervisorReplacementCannotDiscardActiveProcess(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("active-replacement", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	p.State = StateStopped
	if err := s.Register(p); err == nil || !strings.Contains(err.Error(), "stop it first") {
		t.Fatalf("active replacement must fail closed, got %v", err)
	}
	if process.starts != 1 {
		t.Fatalf("failed replacement must preserve active process, got %d starts", process.starts)
	}
}

func TestSupervisorRecordsUnexpectedProcessExit(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("crash", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if process.handler == nil {
		t.Fatal("supervisor must observe process exits")
	}
	process.handler(errors.New("exit status 23"))
	if state, _ := s.State(p.Identity.InstanceID); state != StateFailed {
		t.Fatalf("unexpected process exit must fail provider, got %q", state)
	}
	if len(persist.snapshots) != 1 || persist.snapshots[0].Provider.State != StateFailed {
		t.Fatal("unexpected process exit was not durably persisted")
	}
}

func TestSupervisorStopDetachesObserverAndPersistsStopped(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("stop", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	handshake := HandshakeResult{Provider: p}
	handshake.Provider.State = StateStarting
	handshake.Provider.advertisementValidated = true
	handshake.Provider.advertisedCapabilities = append([]string(nil), p.Manifest.Capabilities...)
	if err := s.MarkReady(p.Identity.InstanceID, handshake); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(context.Background(), p.Identity.InstanceID); err != nil {
		t.Fatal(err)
	}
	if process.handler != nil {
		t.Fatal("explicit stop must detach the exit observer")
	}
	if process.terminates != 1 {
		t.Fatalf("expected one termination, got %d", process.terminates)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateStopped {
		t.Fatalf("expected stopped state, got %q", state)
	}
	if snapshots := persist.latest(); len(snapshots) != 1 || snapshots[0].Provider.State != StateStopped {
		t.Fatalf("stopped state was not durably persisted: %+v", snapshots)
	}
}

func TestSupervisorStopFailurePersistsFailed(t *testing.T) {
	registry := NewRegistry()
	process := &fakeProcessControl{terminateErr: errors.New("kill denied")}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("stop-failure", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	handshake := HandshakeResult{Provider: p}
	handshake.Provider.State = StateStarting
	handshake.Provider.advertisementValidated = true
	handshake.Provider.advertisedCapabilities = append([]string(nil), p.Manifest.Capabilities...)
	if err := s.MarkReady(p.Identity.InstanceID, handshake); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(context.Background(), p.Identity.InstanceID); err == nil || !strings.Contains(err.Error(), "kill denied") {
		t.Fatalf("stop failure was not surfaced: %v", err)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateFailed {
		t.Fatalf("failed stop must preserve failed lifecycle state, got %q", state)
	}
}

func TestSupervisorPersistsUnexpectedExitFromLocalProcess(t *testing.T) {
	registry := NewRegistry()
	persist := &fakeSupervisorPersistence{}
	process := NewLocalProcessControl()
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	provider := installedProvider("local-crash", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	executable := []byte("#!/bin/sh\nexit 23\n")
	digest := sha256.Sum256(executable)
	provider.Manifest.RequiredIsolation = nil
	provider.Manifest.ArtifactDigest = "sha256:" + hex.EncodeToString(digest[:])
	provider.Identity.ArtifactDigest = provider.Manifest.ArtifactDigest
	spec := LaunchSpec{
		Provider:   provider,
		Executable: executable,
		Entrypoint: "plugins/local-crash",
		SocketPath: filepath.Join(t.TempDir(), "plugin.sock"),
	}
	if err := s.RegisterLaunch(spec); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), provider.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if state, _ := s.State(provider.Identity.InstanceID); state == StateFailed {
			snapshots := persist.latest()
			if len(snapshots) != 1 || snapshots[0].Provider.State != StateFailed {
				t.Fatalf("unexpected local exit was not durably persisted: %+v", snapshots)
			}
			if _, err := registry.Resolve("workspace.search.text", nil); err == nil {
				t.Fatal("crashed plugin remained routable")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("local process exit was not reflected in supervisor state: %q", func() State { state, _ := s.State(provider.Identity.InstanceID); return state }())
}

func TestSupervisorUnexpectedExitReleaseFailureRetainsLeasesForRecovery(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{}
	persist := &fakeSupervisorPersistence{}
	leaser := &fakeSupervisorResourceLeaser{
		leases: []scheduler.ResourceLease{{ID: "lease-crash-release-failed", ResourceKey: "cpu", Capacity: 1}},
	}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{
		ResourceRequirements: []scheduler.ResourceRequirement{{Key: "cpu", Capacity: 1}},
		ResourceLeaser:       leaser,
	}, persist)
	if err != nil {
		t.Fatal(err)
	}
	provider := installedProvider("crash-release-failed", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(provider); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), provider.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if process.handler == nil {
		t.Fatal("supervisor must observe the child")
	}
	// The scheduler rejects cleanup. The supervisor must preserve the exact
	// lease authority in durable failed state so restart can retry the release.
	leaser.releaseErr = errors.New("scheduler unavailable")
	process.handler(errors.New("child crashed"))
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshots := persist.latest()
		if len(snapshots) == 1 && snapshots[0].Provider.State == StateFailed {
			if len(snapshots[0].ResourceLeases) != 1 || snapshots[0].ResourceLeases[0].ID != "lease-crash-release-failed" {
				t.Fatalf("release failure must retain exact lease IDs for recovery: %+v", snapshots[0].ResourceLeases)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("unexpected exit was not durably recorded: %+v", persist.latest())
}

func TestSupervisorIgnoresLateExitFromDifferentRuntimeSession(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{}
	persist := &fakeSupervisorPersistence{}
	s, err := NewPersistentSupervisor(context.Background(), registry, process, SupervisorPolicy{}, persist)
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("late-exit", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	late := p.Identity
	late.RuntimeSession = "new-session"
	s.recordUnexpectedExit(p.Identity.InstanceID, late, 0, errors.New("old process exited"))
	if state, _ := s.State(p.Identity.InstanceID); state != StateStarting {
		t.Fatalf("late exit from another runtime session changed state to %q", state)
	}
	if len(persist.snapshots) != 1 || persist.snapshots[0].Provider.State != StateStarting {
		t.Fatal("late exit must not persist a lifecycle mutation")
	}
}

func TestSupervisorIgnoresLateExitFromReusedRuntimeIdentity(t *testing.T) {
	registry := NewRegistry()
	process := &observedFakeProcessControl{}
	s, err := NewSupervisor(registry, process, SupervisorPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	p := installedProvider("reused-runtime", map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced})
	if err := s.Register(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	oldHandler := process.handler
	if oldHandler == nil {
		t.Fatal("supervisor must observe the first process")
	}
	if err := s.recordFailure(context.Background(), p.Identity.InstanceID, time.Now().UTC(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background(), p.Identity.InstanceID, time.Now().UTC().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.State(p.Identity.InstanceID); state != StateStarting {
		t.Fatalf("replacement process should be starting, got %q", state)
	}
	// A buggy process implementation may deliver the old callback after a
	// replacement has started. Instance identity alone is insufficient here.
	oldHandler(errors.New("old process exited"))
	if state, _ := s.State(p.Identity.InstanceID); state != StateStarting {
		t.Fatalf("late callback from reused identity changed state to %q", state)
	}
}
