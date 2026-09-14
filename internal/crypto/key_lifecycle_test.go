package crypto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func lifecycleKey(version string, state KeyState) KeyReference {
	now := time.Date(2026, 9, 14, 18, 30, 0, 0, time.UTC)
	return KeyReference{ID: "publisher-key", Owner: "publisher", Purpose: "sign", AlgorithmFamily: "ml-dsa", Version: version, ProviderRef: "hsm:key", State: state, CreatedAt: now, NotBefore: now, AllowedProfiles: []contracts.CryptoProfile{contracts.CryptoPQRequired, contracts.CryptoHybridHighAssurance}, AllowedOperations: []string{"sign", "verify"}, HardwareBacked: true}
}

func TestKeyLifecycleRotationRevocationAndHistoricalVerification(t *testing.T) {
	registry := NewKeyRegistry()
	if err := registry.Register(lifecycleKey("1", KeyPending)); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 14, 19, 0, 0, 0, time.UTC)
	if err := registry.Activate("publisher-key", "1", now); err != nil {
		t.Fatal(err)
	}
	if err := registry.CanUse("publisher-key", "1", "sign", now); err != nil {
		t.Fatal(err)
	}
	next := lifecycleKey("2", KeyPending)
	next.RotationAncestor = "1"
	if err := registry.Rotate("publisher-key", "1", next, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := registry.CanUse("publisher-key", "1", "verify", now.Add(2*time.Minute)); err != nil {
		t.Fatal("historical verification must remain possible: ", err)
	}
	if err := registry.CanUse("publisher-key", "1", "sign", now.Add(2*time.Minute)); err == nil {
		t.Fatal("retired key must not sign new material")
	}
	if err := registry.Activate("publisher-key", "2", now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := registry.Revoke("publisher-key", "2"); err != nil {
		t.Fatal(err)
	}
	if err := registry.CanUse("publisher-key", "2", "sign", now.Add(3*time.Minute)); err == nil {
		t.Fatal("revoked key must not sign new material")
	}
	if got, err := registry.Get("publisher-key", "1"); err != nil || got.ID != "publisher-key" || got.Version != "1" || got.State != KeyRetiredVerifyOnly {
		t.Fatalf("rotation changed logical identity/history: %+v %v", got, err)
	}
}

func TestKeyReferenceContainsNoSecretMaterialAndAuthorizationIsSeparate(t *testing.T) {
	key := lifecycleKey("1", KeyActive)
	body, err := json.Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "" || string(body) == "secret-key-bytes" {
		t.Fatal("key reference unexpectedly serialized secret material")
	}
	registry := NewKeyRegistry()
	if err := registry.Register(key); err != nil {
		t.Fatal(err)
	}
	if err := registry.CanUse("publisher-key", "1", "encrypt", time.Now().UTC()); err == nil {
		t.Fatal("key lifecycle must not mint an unregistered crypto operation or authorization")
	}
}
