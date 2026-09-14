package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeProcessControl struct {
	starts     int
	terminates int
	startErr   error
}

func (f *fakeProcessControl) Start(_ context.Context, _ LaunchSpec) error {
	f.starts++
	return f.startErr
}

func (f *fakeProcessControl) Terminate(_ context.Context, _ InstanceIdentity) error {
	f.terminates++
	return nil
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
	if process.terminates != 2 {
		t.Fatalf("expected termination on each failure, got %d", process.terminates)
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
