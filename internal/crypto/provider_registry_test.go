package crypto

import (
	"context"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestProviderRegistryRequiresExplicitUniqueProvider(t *testing.T) {
	registry := NewProviderRegistry()
	provider := &fakeWrapper{caps: Capabilities{PQ: true}}
	if err := registry.Register("os-keychain", provider); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("os-keychain", provider); err == nil {
		t.Fatal("duplicate provider registration must fail")
	}
	if _, err := registry.Resolve("missing"); !errors.Is(err, ErrUnknownKeyProvider) {
		t.Fatalf("unknown provider must fail closed: %v", err)
	}
}

func TestProviderRegistryReturnsEnvelopeServiceWithoutKeyMaterial(t *testing.T) {
	registry := NewProviderRegistry()
	if err := registry.Register("os-keychain", &fakeWrapper{caps: Capabilities{PQ: true}}); err != nil {
		t.Fatal(err)
	}
	service, err := registry.Service("os-keychain", EnvelopePolicy{})
	if err != nil || service.Wrapper == nil {
		t.Fatalf("provider service mismatch: %+v err=%v", service, err)
	}
	if _, err := service.Seal(context.Background(), "key:goal", contracts.CryptoPQRequired, []byte("goal"), []byte("aad")); err != nil {
		t.Fatal(err)
	}
}
