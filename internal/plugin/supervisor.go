package plugin

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
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
}

type supervisedInstance struct {
	Provider            Provider
	Launch              LaunchSpec
	ConsecutiveFailures int
	LastFailureAt       time.Time
	LastStartAt         time.Time
}

type Supervisor struct {
	mu       sync.Mutex
	registry *Registry
	process  ProcessControl
	policy   SupervisorPolicy
	persist  SupervisorPersistence
	entries  map[string]supervisedInstance
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
		if provider.State == StateReady || provider.State == StateDegraded || provider.State == StateStarting {
			provider.State = StateStopped
		}
		launch := LaunchSpec{Provider: provider, Executable: append([]byte(nil), snapshot.Launch.Executable...), Entrypoint: snapshot.Launch.Entrypoint, SocketPath: snapshot.Launch.SocketPath, RequiredIsolation: append([]IsolationProperty(nil), snapshot.Launch.RequiredIsolation...)}
		s.entries[provider.Identity.InstanceID] = supervisedInstance{Provider: provider, Launch: launch, ConsecutiveFailures: snapshot.ConsecutiveFailures, LastFailureAt: snapshot.LastFailureAt, LastStartAt: snapshot.LastStartAt}
	}
	return s, nil
}

func snapshotOf(entry supervisedInstance) SupervisorSnapshot {
	return SupervisorSnapshot{Provider: ProviderSnapshot{Manifest: entry.Provider.Manifest, Identity: entry.Provider.Identity, State: entry.Provider.State, Isolation: entry.Provider.Isolation, Priority: entry.Provider.Priority}, Launch: LaunchSnapshot{Executable: append([]byte(nil), entry.Launch.Executable...), Entrypoint: entry.Launch.Entrypoint, SocketPath: entry.Launch.SocketPath, RequiredIsolation: append([]IsolationProperty(nil), entry.Launch.RequiredIsolation...)}, ConsecutiveFailures: entry.ConsecutiveFailures, LastFailureAt: entry.LastFailureAt, LastStartAt: entry.LastStartAt}
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
	s.entries[provider.Identity.InstanceID] = supervisedInstance{Provider: provider, Launch: spec}
	entry := s.entries[provider.Identity.InstanceID]
	s.mu.Unlock()
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
	entry.Provider.State = StateStarting
	entry.LastStartAt = now
	s.entries[instanceID] = entry
	s.mu.Unlock()
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin start: %w", err)
	}

	if err := s.process.Start(ctx, entry.Launch); err != nil {
		s.RecordFailure(context.Background(), instanceID, now)
		return fmt.Errorf("start plugin: %w", err)
	}
	return nil
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
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin failure: %w", err)
	}

	if err := s.process.Terminate(ctx, entry.Provider.Identity); err != nil {
		return fmt.Errorf("terminate failed plugin: %w", err)
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
	entry.Provider.State = StateRevoked
	s.entries[instanceID] = entry
	s.registry.Remove(instanceID)
	s.mu.Unlock()
	if err := s.persistEntry(context.Background(), entry); err != nil {
		return fmt.Errorf("persist plugin revocation: %w", err)
	}
	if err := s.process.Terminate(ctx, entry.Provider.Identity); err != nil {
		return fmt.Errorf("terminate revoked plugin: %w", err)
	}
	return nil
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
