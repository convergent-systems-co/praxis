package plugin

import (
	"errors"
	"testing"
)

func TestRegistrySelectsOnlyReadyIsolatedProviders(t *testing.T) {
	registry := NewRegistry()
	good := fixtureProvider("good", StateReady, map[IsolationProperty]EnforcementState{
		IsolationFilesystem: Enforced,
		IsolationNetwork:    Enforced,
	}, 10)
	badIsolation := fixtureProvider("bad-isolation", StateReady, map[IsolationProperty]EnforcementState{
		IsolationFilesystem: Enforced,
		IsolationNetwork:    Unknown,
	}, 100)
	degraded := fixtureProvider("degraded", StateDegraded, map[IsolationProperty]EnforcementState{
		IsolationFilesystem: Enforced,
		IsolationNetwork:    Enforced,
	}, 1000)

	if err := registry.Register(good); err != nil {
		t.Fatal(err)
	}
	// Registration itself verifies the manifest-required filesystem property.
	// The unknown network property is only required by this particular request.
	badIsolation.Manifest.RequiredIsolation = []IsolationProperty{IsolationFilesystem}
	if err := registry.Register(badIsolation); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(degraded); err != nil {
		t.Fatal(err)
	}

	providers, err := registry.Resolve("workspace.search.text", []IsolationProperty{IsolationNetwork})
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Identity.InstanceID != "good-instance" {
		t.Fatalf("unexpected providers: %+v", providers)
	}
}

func TestRegistryResolutionDoesNotTreatAdvertisementAsAuthority(t *testing.T) {
	registry := NewRegistry()
	provider := fixtureProvider("provider", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	providers, err := registry.Resolve("workspace.search.text", nil)
	if err != nil || len(providers) != 1 {
		t.Fatalf("expected advertised provider: providers=%v err=%v", providers, err)
	}
	// Registry exposes no dispatch/authorization method. A caller receives only
	// provider metadata and must separately pass the lease/capability boundary.
	if providers[0].Manifest.Capabilities[0] != "workspace.search.text" {
		t.Fatal("unexpected advertised capability")
	}
}

func TestRegistryFailsClosedWhenRequiredIsolationIsUnknown(t *testing.T) {
	registry := NewRegistry()
	provider := fixtureProvider("provider", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 1)
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Resolve("workspace.search.text", []IsolationProperty{IsolationNetwork})
	if !errors.Is(err, ErrNoEligibleProvider) {
		t.Fatalf("expected no eligible provider, got %v", err)
	}
}

func TestRegistryOrderingIsDeterministic(t *testing.T) {
	registry := NewRegistry()
	for _, provider := range []Provider{
		fixtureProvider("zeta", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 5),
		fixtureProvider("alpha", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 5),
		fixtureProvider("preferred", StateReady, map[IsolationProperty]EnforcementState{IsolationFilesystem: Enforced}, 10),
	} {
		if err := registry.Register(provider); err != nil {
			t.Fatal(err)
		}
	}
	providers, err := registry.Resolve("workspace.search.text", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 3 || providers[0].Manifest.ID != "preferred" || providers[1].Manifest.ID != "alpha" || providers[2].Manifest.ID != "zeta" {
		t.Fatalf("unexpected provider order: %+v", providers)
	}
}

func fixtureProvider(id string, state State, isolation map[IsolationProperty]EnforcementState, priority int) Provider {
	manifest := Manifest{
		ID: id, Version: "1.0.0", ProtocolMin: "1", ProtocolMax: "1", Entrypoint: "/plugin",
		Capabilities: []string{"workspace.search.text"}, RequiredIsolation: []IsolationProperty{IsolationFilesystem},
		Publisher: "test", ArtifactDigest: "sha256:" + id,
	}
	return Provider{
		Manifest: manifest,
		Identity: InstanceIdentity{
			InstanceID: id + "-instance", PluginID: id, PluginVersion: "1.0.0",
			ArtifactDigest: "sha256:" + id, RuntimeSession: "session-1",
		},
		State: state, Isolation: IsolationProfile{Properties: isolation}, Priority: priority,
	}
}
