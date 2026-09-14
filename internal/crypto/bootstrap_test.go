package crypto

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type bootstrapTestBackend struct {
	id        string
	available error
	opened    int
}

func (b *bootstrapTestBackend) ProviderID() string { return b.id }
func (b *bootstrapTestBackend) SecurityLevel(context.Context) (SecurityLevel, error) {
	return SecurityPlatformProtected, nil
}
func (b *bootstrapTestBackend) Available(context.Context) error { return b.available }
func (b *bootstrapTestBackend) Bootstrap(context.Context, BootstrapRequest) (BootstrapRecord, KeyWrapper, error) {
	return BootstrapRecord{Version: BootstrapRecordVersion, ProviderID: b.id, KeyID: "goal-kek", KeyVersion: "1", Owner: "user", Purpose: "goalstore", Profile: contracts.CryptoClassicalCompatible, SecurityLevel: SecurityPlatformProtected, Platform: "test", Architecture: "test", CreatedAt: time.Unix(1, 0).UTC()}, &fakeWrapper{caps: Capabilities{Classical: true}}, nil
}
func (b *bootstrapTestBackend) Open(context.Context, BootstrapRecord) (KeyWrapper, error) {
	b.opened++
	return &fakeWrapper{caps: Capabilities{Classical: true}}, nil
}

func validBootstrapRequest() BootstrapRequest {
	return BootstrapRequest{ProviderID: "platform", KeyID: "goal-kek", Owner: "user", Purpose: "goalstore", Profile: contracts.CryptoClassicalCompatible}
}

func TestBootstrapRegistryRequiresExplicitProviderAndDoesNotFallback(t *testing.T) {
	r := NewBootstrapRegistry()
	backend := &bootstrapTestBackend{id: "platform", available: errors.New("locked")}
	if err := r.Register(backend); err != nil {
		t.Fatal(err)
	}
	missing := validBootstrapRequest()
	missing.ProviderID = ""
	if _, _, err := r.Bootstrap(context.Background(), missing); err == nil {
		t.Fatal("missing provider selection must fail closed")
	}
	if _, _, err := r.Bootstrap(context.Background(), validBootstrapRequest()); err == nil {
		t.Fatal("unavailable selected provider must not fall back")
	}
}

func TestBootstrapRecordContainsOnlyNonSecretBindingAndOpensExactProvider(t *testing.T) {
	r := NewBootstrapRegistry()
	backend := &bootstrapTestBackend{id: "platform"}
	if err := r.Register(backend); err != nil {
		t.Fatal(err)
	}
	record, _, err := r.Bootstrap(context.Background(), validBootstrapRequest())
	if err != nil {
		t.Fatal(err)
	}
	if record.KeyID != "goal-kek" || record.ProviderID != "platform" {
		t.Fatalf("unexpected binding: %+v", record)
	}
	if _, err := record.Digest(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Open(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	if backend.opened != 1 {
		t.Fatalf("expected one exact-provider open, got %d", backend.opened)
	}
	copyRecord := record
	copyRecord.ProviderID = "other"
	if _, err := r.Open(context.Background(), copyRecord); err == nil {
		t.Fatal("provider substitution must fail closed")
	}
}
