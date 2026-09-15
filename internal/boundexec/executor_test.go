package boundexec

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/plugin"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type fakeRepository struct {
	invocation state.RegisteredInvocation
	resolved   state.ResolvedPlugin
	err        error
}

func (r fakeRepository) ResolveInvocationAlias(context.Context, string) (state.RegisteredInvocation, error) {
	return r.invocation, r.err
}
func (r fakeRepository) ResolvePlugin(context.Context, string, string) (state.ResolvedPlugin, error) {
	return r.resolved, r.err
}

type fakeAuthority struct {
	called bool
	err    error
}

func (a *fakeAuthority) Authorize(context.Context, contracts.InvocationContract, plugin.InstanceIdentity, capability.Request, time.Time) (plugin.LeaseBinding, error) {
	a.called = true
	if a.err != nil {
		return plugin.LeaseBinding{}, a.err
	}
	return plugin.LeaseBinding{Lease: contracts.CapabilityLease{ID: "lease-1", Principal: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, Capability: "goal.execute", Operations: []string{"run"}, Scope: "goal:weather", IssuedAt: time.Now().UTC()}}, nil
}

type fakeRuntime struct{ called bool }

func (r *fakeRuntime) Execute(context.Context, state.ResolvedPlugin, contracts.InvocationContract, plugin.LeaseBinding, Request, plugin.InstanceIdentity) ([]byte, error) {
	r.called = true
	return []byte("plugin-result"), nil
}

func fixture() (state.RegisteredInvocation, state.ResolvedPlugin) {
	contract := contracts.InvocationContract{Version: contracts.InvocationContractCurrentVersion(), PackageID: "first-party/goals", PackageVersion: "1", GraphID: "goals", GraphVersion: "1", EntryPointID: "goals.run", Aliases: []string{"goals"}, RequiredCapabilities: []string{"goal.execute"}}
	binding := contracts.ExecutableBinding{EntryPointID: "goals.run", PluginID: "goals-runtime", PluginVersion: "1", PluginDefinitionDigest: testDigest, ExecutableContentID: "goals-runtime.bin", ExecutableContentVersion: "1", ExecutableDigest: testDigest, RuntimeID: "praxis.plugin.grpc", RuntimeVersion: "1", RuntimeDigest: testDigest, ProtocolMin: "1", ProtocolMax: "1"}
	inv := state.RegisteredInvocation{Contract: contract, ContentDigest: testDigest, ContractDigest: testDigest, RuntimeBinding: contracts.InvocationRuntimeBinding{PackageID: contract.PackageID, PackageVersion: contract.PackageVersion, PackageDigest: testDigest, EntryPointID: contract.EntryPointID, ContractDigest: testDigest, RuntimeID: binding.RuntimeID, RuntimeVersion: binding.RuntimeVersion, RuntimeDigest: binding.RuntimeDigest, Executable: &binding}}
	manifest := plugin.Manifest{ContractVersion: "v1", ID: binding.PluginID, Version: binding.PluginVersion, ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "bin/goals", ExecutableContentID: binding.ExecutableContentID, ExecutableContentVersion: binding.ExecutableContentVersion, ArtifactDigest: binding.ExecutableDigest}
	resolved := state.ResolvedPlugin{Manifest: manifest, Definition: state.RegisteredContent{PackageID: contract.PackageID, PackageVersion: contract.PackageVersion, PackageDigest: testDigest, Content: packagecatalog.ContentRef{Kind: packagecatalog.ContentPlugin, ID: binding.PluginID, Version: binding.PluginVersion, Digest: binding.PluginDefinitionDigest}}, Executable: state.RegisteredContent{PackageID: contract.PackageID, PackageVersion: contract.PackageVersion, PackageDigest: testDigest, Content: packagecatalog.ContentRef{Kind: packagecatalog.ContentPluginExecutable, ID: binding.ExecutableContentID, Version: binding.ExecutableContentVersion, Digest: binding.ExecutableDigest, Artifact: manifest.Entrypoint}, ArtifactBytes: []byte("verified")}}
	return inv, resolved
}

func TestBoundExecutorResolvesAndUsesBoundRuntime(t *testing.T) {
	inv, resolved := fixture()
	authority := &fakeAuthority{}
	runtime := &fakeRuntime{}
	executor := Executor{Store: fakeRepository{invocation: inv, resolved: resolved}, Authority: authority, Runtime: runtime}
	result, err := executor.Execute(context.Background(), Request{Alias: "goals", Actor: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, Operation: "run", Scope: "goal:weather", Now: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if !authority.called || !runtime.called {
		t.Fatal("bound execution must authorize and use the package runtime")
	}
	if string(result.Payload) != "plugin-result" || result.ExecutableDigest != testDigest {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestBoundExecutorFailsClosedWithoutBinding(t *testing.T) {
	inv, resolved := fixture()
	inv.RuntimeBinding.Executable = nil
	authority := &fakeAuthority{}
	runtime := &fakeRuntime{}
	_, err := (Executor{Store: fakeRepository{invocation: inv, resolved: resolved}, Authority: authority, Runtime: runtime}).Execute(context.Background(), Request{Alias: "goals", Actor: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, Operation: "run", Scope: "goal:weather"})
	if !errors.Is(err, ErrMissingExecutableBinding) {
		t.Fatalf("expected missing binding, got %v", err)
	}
	if authority.called || runtime.called {
		t.Fatal("missing binding must fail before authority/runtime")
	}
}

func TestBoundExecutorRejectsSubstitutedExecutable(t *testing.T) {
	inv, resolved := fixture()
	resolved.Manifest.ArtifactDigest = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"
	authority := &fakeAuthority{}
	runtime := &fakeRuntime{}
	_, err := (Executor{Store: fakeRepository{invocation: inv, resolved: resolved}, Authority: authority, Runtime: runtime}).Execute(context.Background(), Request{Alias: "goals", Actor: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, Operation: "run", Scope: "goal:weather"})
	if err == nil {
		t.Fatal("substituted executable must fail closed")
	}
	if authority.called || runtime.called {
		t.Fatal("substitution must fail before authority/runtime")
	}
}

func TestBoundExecutorDoesNotFallbackWhenAuthorityFails(t *testing.T) {
	inv, resolved := fixture()
	authority := &fakeAuthority{err: errors.New("denied")}
	runtime := &fakeRuntime{}
	_, err := (Executor{Store: fakeRepository{invocation: inv, resolved: resolved}, Authority: authority, Runtime: runtime}).Execute(context.Background(), Request{Alias: "goals", Actor: contracts.PrincipalRef{ID: "human-1", Kind: "human"}, Operation: "run", Scope: "goal:weather"})
	if err == nil || runtime.called {
		t.Fatal("authority failure must not launch or fall back")
	}
}
