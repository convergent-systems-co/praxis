package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/convergent-systems-co/praxis/internal/scheduler"
)

// ProcessControl is implemented by the host-specific launcher. The supervisor
// never relies on a plugin voluntarily shutting itself down.
type ProcessControl interface {
	Start(ctx context.Context, spec LaunchSpec) error
	Terminate(ctx context.Context, instance InstanceIdentity) error
}

// SupervisorSnapshot is durable lifecycle state. Runtime advertisement is
// deliberately absent: a restored instance must perform a fresh handshake
// before it can re-enter the registry.
type SupervisorSnapshot struct {
	Provider            ProviderSnapshot
	Launch              LaunchSnapshot
	ResourceLeases      []scheduler.ResourceLease
	ConsecutiveFailures int
	LastFailureAt       time.Time
	LastStartAt         time.Time
}

type ProviderSnapshot struct {
	Manifest  Manifest
	Identity  InstanceIdentity
	State     State
	Isolation IsolationProfile
	Priority  int
}

type LaunchSnapshot struct {
	Executable        []byte
	Entrypoint        string
	SocketPath        string
	RequiredIsolation []IsolationProperty
}

type SupervisorPersistence interface {
	LoadSupervisorSnapshots(context.Context) ([]SupervisorSnapshot, error)
	SaveSupervisorSnapshot(context.Context, SupervisorSnapshot) error
}

type SupervisorPolicy struct {
	MaxConsecutiveFailures int
	RestartBackoff         time.Duration
	RequiredIsolation      []IsolationProperty
	ResourceRequirements   []scheduler.ResourceRequirement
	ResourceLeaseTTL       time.Duration
	ResourceLeaser         ResourceLeaser
}

// ResourceLeaser is the authoritative scheduler boundary used by supervised
// process attempts. Implementations must acquire the complete requirement set
// atomically and make release idempotent.
type ResourceLeaser interface {
	AcquireSchedulerResourceLeases(context.Context, string, string, []scheduler.ResourceRequirement, time.Time, *time.Time) ([]scheduler.ResourceLease, error)
	ReleaseSchedulerResourceLeases(context.Context, []string, time.Time) error
}

type supervisedInstance struct {
	Provider            Provider
	Launch              LaunchSpec
	ConsecutiveFailures int
	LastFailureAt       time.Time
	LastStartAt         time.Time
	resourceLeases      []scheduler.ResourceLease
	// runtimeGeneration is intentionally ephemeral. It fences callbacks from
	// an older process when an instance ID is reused for a later launch; it is
	// not durable authority and must never be restored as runtime state.
	runtimeGeneration uint64
}

type Supervisor struct {
	mu             sync.Mutex
	registry       *Registry
	process        ProcessControl
	policy         SupervisorPolicy
	persist        SupervisorPersistence
	entries        map[string]supervisedInstance
	nextGeneration uint64
}

func NewSupervisor(registry *Registry, process ProcessControl, policy SupervisorPolicy) (*Supervisor, error) {
	return newSupervisor(registry, process, policy, nil, context.Background())
}

func NewPersistentSupervisor(ctx context.Context, registry *Registry, process ProcessControl, policy SupervisorPolicy, persist SupervisorPersistence) (*Supervisor, error) {
	if persist == nil {
		return nil, errors.New("supervisor persistence is required")
	}
	return newSupervisor(registry, process, policy, persist, ctx)
}

func newSupervisor(registry *Registry, process ProcessControl, policy SupervisorPolicy, persist SupervisorPersistence, ctx context.Context) (*Supervisor, error) {
	if registry == nil || process == nil {
		return nil, errors.New("plugin registry and process control are required")
	}
	if policy.MaxConsecutiveFailures < 0 || policy.RestartBackoff < 0 {
		return nil, errors.New("invalid supervisor policy")
	}
	if policy.ResourceLeaseTTL < 0 {
		return nil, errors.New("invalid resource lease TTL")
	}
	if len(policy.ResourceRequirements) != 0 && policy.ResourceLeaser == nil {
		return nil, errors.New("resource leaser is required when resource requirements are configured")
	}
	s := &Supervisor{registry: registry, process: process, policy: policy, persist: persist, entries: map[string]supervisedInstance{}}
	if persist == nil {
		return s, nil
	}
	snapshots, err := persist.LoadSupervisorSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("load supervisor snapshots: %w", err)
	}
	for _, snapshot := range snapshots {
		provider := Provider{Manifest: snapshot.Provider.Manifest, Identity: snapshot.Provider.Identity, State: snapshot.Provider.State, Isolation: snapshot.Provider.Isolation, Priority: snapshot.Provider.Priority}
		// A process cannot be presumed alive across runtime restart. Ready,
		// degraded, or starting snapshots become stopped and require handshake.
		normalizedRuntimeState := provider.State == StateReady || provider.State == StateDegraded || provider.State == StateStarting
		if normalizedRuntimeState {
			provider.State = StateStopped
		}
		launch := LaunchSpec{Provider: provider, Executable: append([]byte(nil), snapshot.Launch.Executable...), Entrypoint: snapshot.Launch.Entrypoint, SocketPath: snapshot.Launch.SocketPath, RequiredIsolation: append([]IsolationProperty(nil), snapshot.Launch.RequiredIsolation...)}
		if err := provider.Identity.ValidateAgainst(provider.Manifest); err != nil {
			return nil, fmt.Errorf("validate persisted plugin %q: %w", provider.Identity.InstanceID, err)
		}
		if len(launch.Executable) != 0 {
			if err := launch.Validate(); err != nil {
				return nil, fmt.Errorf("validate persisted launch %q: %w", provider.Identity.InstanceID, err)
			}
		}
		entry := supervisedInstance{Provider: provider, Launch: launch, resourceLeases: append([]scheduler.ResourceLease(nil), snapshot.ResourceLeases...), ConsecutiveFailures: snapshot.ConsecutiveFailures, LastFailureAt: snapshot.LastFailureAt, LastStartAt: snapshot.LastStartAt}
		s.entries[provider.Identity.InstanceID] = entry
		// A restored process is never live. Fence any leases belonging to the
		// abandoned attempt, including leases without a TTL, before publishing
		// the normalized durable state. The scheduler remains the authority for
		// the release operation; this snapshot only preserves the exact IDs.
		if len(entry.resourceLeases) != 0 && s.policy.ResourceLeaser == nil {
			return nil, fmt.Errorf("persisted plugin %q has scheduler leases but no resource leaser is configured", provider.Identity.InstanceID)
		}
		if len(entry.resourceLeases) != 0 {
			if err := s.releaseResources(ctx, entry, time.Now().UTC()); err != nil {
				return nil, fmt.Errorf("release restored plugin resources %q: %w", provider.Identity.InstanceID, err)
			}
			entry.resourceLeases = nil
			s.entries[provider.Identity.InstanceID] = entry
		}
		if normalizedRuntimeState || len(snapshot.ResourceLeases) != 0 {
			if err := s.persistEntry(context.Background(), entry); err != nil {
				return nil, fmt.Errorf("persist normalized plugin %q: %w", provider.Identity.InstanceID, err)
			}
		}
	}
	return s, nil
}

func snapshotOf(entry supervisedInstance) SupervisorSnapshot {
	return SupervisorSnapshot{Provider: ProviderSnapshot{Manifest: entry.Provider.Manifest, Identity: entry.Provider.Identity, State: entry.Provider.State, Isolation: entry.Provider.Isolation, Priority: entry.Provider.Priority}, Launch: LaunchSnapshot{Executable: append([]byte(nil), entry.Launch.Executable...), Entrypoint: entry.Launch.Entrypoint, SocketPath: entry.Launch.SocketPath, RequiredIsolation: append([]IsolationProperty(nil), entry.Launch.RequiredIsolation...)}, ResourceLeases: append([]scheduler.ResourceLease(nil), entry.resourceLeases...), ConsecutiveFailures: entry.ConsecutiveFailures, LastFailureAt: entry.LastFailureAt, LastStartAt: entry.LastStartAt}
}

func (s *Supervisor) persistEntry(ctx context.Context, entry supervisedInstance) error {
	if s.persist == nil {
		return nil
	}
	return s.persist.SaveSupervisorSnapshot(ctx, snapshotOf(entry))
}

func (s *Supervisor) Register(provider Provider) error {
	return s.RegisterLaunch(LaunchSpec{Provider: provider})
}

// RegisterLaunch binds a verified executable payload to the supervised
// provider. The payload is retained only as launch input; runtime advertisement
// is still absent until a fresh handshake is validated.
func (s *Supervisor) RegisterLaunch(spec LaunchSpec) error {
	provider := spec.Provider
	if err := provider.Identity.ValidateAgainst(provider.Manifest); err != nil {
		return err
	}
	if len(spec.Executable) != 0 {
		if err := spec.Validate(); err != nil {
			return err
		}
	}
	required := append([]IsolationProperty(nil), provider.Manifest.RequiredIsolation...)
	required = append(required, s.policy.RequiredIsolation...)
	if err := provider.Isolation.Satisfies(uniqueIsolation(required)); err != nil {
		return fmt.Errorf("supervisor isolation requirement: %w", err)
	}
	if provider.State != StateInstalled && provider.State != StateStopped && provider.State != StateFailed {
		return fmt.Errorf("provider state %q cannot be supervised for start", provider.State)
	}
	s.mu.Lock()
	previous, hadPrevious := s.entries[provider.Identity.InstanceID]
	if hadPrevious && (previous.Provider.State == StateStarting || previous.Provider.State == StateReady || previous.Provider.State == StateDegraded || previous.Provider.State == StateDraining) {
		s.mu.Unlock()
		return fmt.Errorf("cannot replace active supervised plugin in state %q; stop it first", previous.Provider.State)
	}
	if hadPrevious && len(previous.resourceLeases) != 0 && s.policy.ResourceLeaser == nil {
		s.mu.Unlock()
		return errors.New("cannot replace supervised plugin with unreleasable scheduler leases")
	}
	s.entries[provider.Identity.InstanceID] = supervisedInstance{Provider: provider, Launch: spec}
	entry := s.entries[provider.Identity.InstanceID]
	s.mu.Unlock()
	if hadPrevious && len(previous.resourceLeases) != 0 {
		if err := s.releaseResources(context.Background(), previous, time.Now().UTC()); err != nil {
			// Restore the prior entry so a failed release cannot silently lose
			// scheduler authority or replace a still-live process binding.
			s.mu.Lock()
			s.entries[provider.Identity.InstanceID] = previous
			s.mu.Unlock()
			return fmt.Errorf("release replaced plugin resources: %w", err)
		}
	}
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist supervised launch: %w", err)
	}
	return nil
}

func (s *Supervisor) Start(ctx context.Context, instanceID string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	entry, ok := s.entries[instanceID]
	if !ok {
		s.mu.Unlock()
		return errors.New("plugin instance is not supervised")
	}
	if entry.Provider.State == StateQuarantined || entry.Provider.State == StateRevoked {
		s.mu.Unlock()
		return errors.New("quarantined/revoked plugin cannot start")
	}
	if s.policy.MaxConsecutiveFailures > 0 && entry.ConsecutiveFailures >= s.policy.MaxConsecutiveFailures {
		entry.Provider.State = StateQuarantined
		s.entries[instanceID] = entry
		s.registry.Remove(instanceID)
		s.mu.Unlock()
		return errors.New("plugin failure threshold reached; quarantined")
	}
	if !entry.LastFailureAt.IsZero() && s.policy.RestartBackoff > 0 && now.Sub(entry.LastFailureAt) < s.policy.RestartBackoff {
		s.mu.Unlock()
		return errors.New("plugin restart backoff active")
	}
	if err := ValidateTransition(entry.Provider.State, StateStarting); err != nil {
		s.mu.Unlock()
		return err
	}
	previousState := entry.Provider.State
	entry.Provider.State = StateStarting
	entry.LastStartAt = now
	s.entries[instanceID] = entry
	s.mu.Unlock()
	// Resource admission is part of the launch boundary. A process is never
	// started while holding only a caller-side estimate of capacity.
	if s.policy.ResourceLeaser != nil && len(s.policy.ResourceRequirements) != 0 {
		attemptID := instanceID + ":" + entry.Provider.Identity.RuntimeSession
		var expiry *time.Time
		if s.policy.ResourceLeaseTTL > 0 {
			expires := now.Add(s.policy.ResourceLeaseTTL)
			expiry = &expires
		}
		leases, err := s.policy.ResourceLeaser.AcquireSchedulerResourceLeases(ctx, instanceID, attemptID, s.policy.ResourceRequirements, now, expiry)
		if err != nil {
			s.mu.Lock()
			entry = s.entries[instanceID]
			entry.Provider.State = previousState
			s.entries[instanceID] = entry
			s.mu.Unlock()
			_ = s.persistEntry(context.Background(), entry)
			return fmt.Errorf("admit plugin resources: %w", err)
		}
		if err := validateResourceLeases(leases, instanceID, attemptID, s.policy.ResourceRequirements); err != nil {
			// A leaser that reports success without the complete, identity-bound
			// set has not established admission authority. Release any usable
			// leases before restoring the pre-attempt lifecycle state; never
			// launch against a caller-side or partial capacity assertion.
			entry.resourceLeases = leases
			s.mu.Lock()
			s.entries[instanceID] = entry
			s.mu.Unlock()
			releaseErr := s.releaseResources(context.Background(), entry, now)
			s.mu.Lock()
			entry = s.entries[instanceID]
			entry.Provider.State = previousState
			entry.resourceLeases = nil
			s.entries[instanceID] = entry
			s.mu.Unlock()
			_ = s.persistEntry(context.Background(), entry)
			if releaseErr != nil {
				return fmt.Errorf("invalid plugin resource admission: %w (release resources: %v)", err, releaseErr)
			}
			return fmt.Errorf("invalid plugin resource admission: %w", err)
		}
		entry.resourceLeases = leases
		s.mu.Lock()
		s.entries[instanceID] = entry
		s.mu.Unlock()
	}
	if err := s.persistEntry(context.Background(), entry); err != nil {
		if releaseErr := s.releaseResources(context.Background(), entry, now); releaseErr != nil {
			return fmt.Errorf("persist plugin start: %w (release resources: %v)", err, releaseErr)
		}
		if failureErr := s.recordFailure(context.Background(), instanceID, now, false); failureErr != nil {
			return fmt.Errorf("persist plugin start: %w (record failure: %v)", err, failureErr)
		}
		return fmt.Errorf("persist plugin start: %w", err)
	}
	if observer, ok := s.process.(ProcessExitObserver); ok {
		expectedIdentity := entry.Provider.Identity
		s.mu.Lock()
		s.nextGeneration++
		generation := s.nextGeneration
		entry.runtimeGeneration = generation
		s.entries[instanceID] = entry
		s.mu.Unlock()
		observer.SetExitHandler(entry.Provider.Identity, func(exitErr error) {
			s.recordUnexpectedExit(instanceID, expectedIdentity, generation, exitErr)
		})
	} else {
		s.mu.Lock()
		s.nextGeneration++
		entry.runtimeGeneration = s.nextGeneration
		s.entries[instanceID] = entry
		s.mu.Unlock()
	}

	if err := s.process.Start(ctx, entry.Launch); err != nil {
		if observer, ok := s.process.(ProcessExitObserver); ok {
			// No child was successfully started, so no exit callback may retain
			// authority over a later runtime generation.
			observer.SetExitHandler(entry.Provider.Identity, nil)
		}
		// Start returned before establishing a running child. Record the durable
		// failure, but do not ask the process controller to terminate an absent
		// process or let that cleanup error mask the launch error.
		if failureErr := s.recordFailure(context.Background(), instanceID, now, false); failureErr != nil {
			return fmt.Errorf("start plugin: %w (record failure: %v)", err, failureErr)
		}
		return fmt.Errorf("start plugin: %w", err)
	}
	return nil
}

func validateResourceLeases(leases []scheduler.ResourceLease, sliceID, attemptID string, requirements []scheduler.ResourceRequirement) error {
	ordered, err := (scheduler.Slice{ID: sliceID, Resources: append([]scheduler.ResourceRequirement(nil), requirements...)}).OrderedRequirements()
	if err != nil {
		return err
	}
	if len(leases) != len(ordered) {
		return fmt.Errorf("leaser returned %d leases for %d requirements", len(leases), len(ordered))
	}
	byKey := make(map[string]scheduler.ResourceLease, len(leases))
	for _, lease := range leases {
		if lease.ID == "" || lease.SliceID != sliceID || lease.AttemptID != attemptID || lease.ResourceKey == "" || lease.Capacity <= 0 {
			return errors.New("leaser returned an unbound resource lease")
		}
		if _, exists := byKey[lease.ResourceKey]; exists {
			return fmt.Errorf("leaser returned duplicate resource lease for %s", lease.ResourceKey)
		}
		byKey[lease.ResourceKey] = lease
	}
	for _, req := range ordered {
		lease, ok := byKey[req.Key]
		if !ok || lease.Capacity != req.Capacity {
			return fmt.Errorf("leaser returned incomplete resource lease for %s", req.Key)
		}
	}
	return nil
}

// recordUnexpectedExit handles a child that exited without a supervisor
// command (crash, external kill, or launch-context cancellation). The child
// may already have been reaped, so this path deliberately does not call
// ProcessControl.Terminate.
func (s *Supervisor) recordUnexpectedExit(instanceID string, identity InstanceIdentity, generation uint64, _ error) {
	now := time.Now().UTC()
	s.mu.Lock()
	entry, ok := s.entries[instanceID]
	if !ok {
		s.mu.Unlock()
		return
	}
	if entry.Provider.Identity != identity || entry.runtimeGeneration != generation {
		// A late exit from an older runtime session cannot mutate the current
		// supervised generation, even when a caller reused the map key or
		// runtime identity.
		s.mu.Unlock()
		return
	}
	if entry.Provider.State != StateStarting && entry.Provider.State != StateReady && entry.Provider.State != StateDegraded {
		s.mu.Unlock()
		return
	}
	entry.ConsecutiveFailures++
	entry.LastFailureAt = now
	entry.Provider.State = StateFailed
	if s.policy.MaxConsecutiveFailures > 0 && entry.ConsecutiveFailures >= s.policy.MaxConsecutiveFailures {
		entry.Provider.State = StateQuarantined
	}
	s.entries[instanceID] = entry
	s.registry.Remove(instanceID)
	s.mu.Unlock()
	// Persist the authoritative post-release entry. The local copy still has
	// the pre-release lease IDs, while releaseResources clears the current
	// entry only after the scheduler accepts the idempotent release.
	_ = s.releaseResources(context.Background(), entry, now)
	_ = s.persistCurrent(context.Background(), instanceID)
}

// MarkReady publishes only the result of a just-completed validated handshake.
// A launch definition, a package declaration, or a caller-selected state cannot
// publish a routable provider by itself.
func (s *Supervisor) MarkReady(instanceID string, handshake HandshakeResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[instanceID]
	if !ok {
		return errors.New("plugin instance is not supervised")
	}
	if handshake.Provider.Identity != entry.Provider.Identity {
		return errors.New("handshake provider identity does not match supervised launch")
	}
	if handshake.Provider.Manifest.ID != entry.Provider.Manifest.ID || handshake.Provider.Manifest.Version != entry.Provider.Manifest.Version || handshake.Provider.Manifest.ArtifactDigest != entry.Provider.Manifest.ArtifactDigest {
		return errors.New("handshake provider manifest does not match supervised launch")
	}
	if !handshake.Provider.advertisementValidated {
		return errors.New("supervisor requires handshake-validated runtime advertisement")
	}
	if err := ValidateTransition(entry.Provider.State, StateReady); err != nil {
		return err
	}
	priority := entry.Provider.Priority
	entry.Provider = handshake.Provider
	entry.Provider.State = StateReady
	entry.Provider.Priority = priority
	entry.ConsecutiveFailures = 0
	s.entries[instanceID] = entry
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin readiness: %w", err)
	}
	return s.registry.Register(entry.Provider)
}

func (s *Supervisor) RecordFailure(ctx context.Context, instanceID string, now time.Time) error {
	return s.recordFailure(ctx, instanceID, now, true)
}

// Stop drains a running plugin and durably settles it as stopped. An explicit
// stop is distinct from an unexpected exit: the latter is failure evidence,
// while this path is the lifecycle boundary used before replacement, update,
// or clean shutdown. The observer is detached before termination so the
// expected child exit cannot race this state transition and become a failure.
func (s *Supervisor) Stop(ctx context.Context, instanceID string) error {
	s.mu.Lock()
	entry, ok := s.entries[instanceID]
	if !ok {
		s.mu.Unlock()
		return errors.New("plugin instance is not supervised")
	}
	if err := ValidateTransition(entry.Provider.State, StateDraining); err != nil {
		s.mu.Unlock()
		return err
	}
	entry.Provider.State = StateDraining
	s.entries[instanceID] = entry
	s.registry.Remove(instanceID)
	s.mu.Unlock()
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin draining: %w", err)
	}
	if observer, ok := s.process.(ProcessExitObserver); ok {
		observer.SetExitHandler(entry.Provider.Identity, nil)
	}
	if err := s.process.Terminate(ctx, entry.Provider.Identity); err != nil {
		// Keep the process lifecycle authoritative: a failed termination is not
		// success, and the child must remain represented as failed for recovery.
		failureErr := s.recordFailure(context.Background(), instanceID, time.Now().UTC(), false)
		if failureErr != nil {
			return fmt.Errorf("terminate draining plugin: %w (record failure: %v)", err, failureErr)
		}
		return fmt.Errorf("terminate draining plugin: %w", err)
	}
	if err := s.releaseResources(ctx, entry, time.Now().UTC()); err != nil {
		failureErr := s.recordFailure(context.Background(), instanceID, time.Now().UTC(), false)
		if failureErr != nil {
			return fmt.Errorf("release stopped plugin resources: %w (record failure: %v)", err, failureErr)
		}
		return fmt.Errorf("release stopped plugin resources: %w", err)
	}
	s.mu.Lock()
	entry, ok = s.entries[instanceID]
	if !ok {
		s.mu.Unlock()
		return errors.New("plugin instance disappeared while stopping")
	}
	if entry.Provider.State != StateDraining {
		s.mu.Unlock()
		return fmt.Errorf("plugin stop was superseded by state %q", entry.Provider.State)
	}
	entry.Provider.State = StateStopped
	s.entries[instanceID] = entry
	s.mu.Unlock()
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin stopped: %w", err)
	}
	return nil
}

func (s *Supervisor) recordFailure(ctx context.Context, instanceID string, now time.Time, terminate bool) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	entry, ok := s.entries[instanceID]
	if !ok {
		s.mu.Unlock()
		return errors.New("plugin instance is not supervised")
	}
	entry.ConsecutiveFailures++
	entry.LastFailureAt = now
	if entry.Provider.State != StateFailed {
		if CanTransition(entry.Provider.State, StateFailed) {
			entry.Provider.State = StateFailed
		}
	}
	quarantine := s.policy.MaxConsecutiveFailures > 0 && entry.ConsecutiveFailures >= s.policy.MaxConsecutiveFailures
	if quarantine {
		entry.Provider.State = StateQuarantined
	}
	s.entries[instanceID] = entry
	s.registry.Remove(instanceID)
	s.mu.Unlock()
	releaseErr := s.releaseResources(ctx, entry, now)
	// Persist even when release reports an error: retaining the lease IDs in
	// durable failed state gives restart a second authoritative recovery chance.
	persistErr := s.persistCurrent(context.Background(), instanceID)
	if releaseErr != nil {
		if persistErr != nil {
			return fmt.Errorf("release plugin resources: %w (persist failure: %v)", releaseErr, persistErr)
		}
		return fmt.Errorf("release plugin resources: %w", releaseErr)
	}
	if persistErr != nil {
		return fmt.Errorf("persist plugin failure: %w", persistErr)
	}

	if terminate {
		if err := s.process.Terminate(ctx, entry.Provider.Identity); err != nil {
			return fmt.Errorf("terminate failed plugin: %w", err)
		}
	}
	return nil
}

func (s *Supervisor) Revoke(ctx context.Context, instanceID string) error {
	s.mu.Lock()
	entry, ok := s.entries[instanceID]
	if !ok {
		s.mu.Unlock()
		return errors.New("plugin instance is not supervised")
	}
	if err := ValidateTransition(entry.Provider.State, StateRevoked); err != nil {
		s.mu.Unlock()
		return err
	}
	terminate := entry.Provider.State == StateStarting || entry.Provider.State == StateReady || entry.Provider.State == StateDegraded || entry.Provider.State == StateDraining
	if observer, ok := s.process.(ProcessExitObserver); ok {
		observer.SetExitHandler(entry.Provider.Identity, nil)
	}
	entry.Provider.State = StateRevoked
	s.entries[instanceID] = entry
	s.registry.Remove(instanceID)
	s.mu.Unlock()
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin revocation: %w", err)
	}
	if !terminate {
		if err := s.releaseResources(ctx, entry, time.Now().UTC()); err != nil {
			return fmt.Errorf("release revoked plugin resources: %w", err)
		}
		if err := s.persistCurrent(context.Background(), instanceID); err != nil {
			return fmt.Errorf("persist revoked plugin cleanup: %w", err)
		}
		return nil
	}
	if err := s.process.Terminate(ctx, entry.Provider.Identity); err != nil {
		// Revocation is an authority boundary. A failed best-effort process
		// termination must not roll the durable state back to a routable state.
		if releaseErr := s.releaseResources(ctx, entry, time.Now().UTC()); releaseErr != nil {
			return fmt.Errorf("terminate revoked plugin: %w (release resources: %v)", err, releaseErr)
		}
		return fmt.Errorf("terminate revoked plugin: %w", err)
	}
	if err := s.releaseResources(ctx, entry, time.Now().UTC()); err != nil {
		return fmt.Errorf("release revoked plugin resources: %w", err)
	}
	if err := s.persistCurrent(context.Background(), instanceID); err != nil {
		return fmt.Errorf("persist revoked plugin cleanup: %w", err)
	}
	return nil
}

func (s *Supervisor) persistCurrent(ctx context.Context, instanceID string) error {
	s.mu.Lock()
	entry, ok := s.entries[instanceID]
	s.mu.Unlock()
	if !ok {
		return errors.New("plugin instance is not supervised")
	}
	return s.persistEntry(ctx, entry)
}

func (s *Supervisor) releaseResources(ctx context.Context, entry supervisedInstance, now time.Time) error {
	if s.policy.ResourceLeaser == nil || len(entry.resourceLeases) == 0 {
		return nil
	}
	ids := make([]string, 0, len(entry.resourceLeases))
	for _, lease := range entry.resourceLeases {
		ids = append(ids, lease.ID)
	}
	if err := s.policy.ResourceLeaser.ReleaseSchedulerResourceLeases(ctx, ids, now); err != nil {
		return err
	}
	// Do not retain already-released authority in the in-memory or durable
	// snapshot. The identity check prevents a late cleanup from clearing a
	// replacement attempt's leases.
	s.mu.Lock()
	current, ok := s.entries[entry.Provider.Identity.InstanceID]
	if ok && sameLeaseIDs(current.resourceLeases, ids) {
		current.resourceLeases = nil
		s.entries[entry.Provider.Identity.InstanceID] = current
	}
	s.mu.Unlock()
	return nil
}

func sameLeaseIDs(leases []scheduler.ResourceLease, ids []string) bool {
	if len(leases) != len(ids) {
		return false
	}
	seen := make(map[string]struct{}, len(leases))
	for _, lease := range leases {
		seen[lease.ID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

func (s *Supervisor) State(instanceID string) (State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[instanceID]
	if !ok {
		return "", false
	}
	return entry.Provider.State, true
}
