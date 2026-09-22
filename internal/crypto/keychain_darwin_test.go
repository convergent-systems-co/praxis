//go:build darwin

package crypto

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestMacOSKeychainBootstrapUnlockRestartAndMissingKeyFailClosed(t *testing.T) {
	requireKeychain(t)
	backend := NewMacOSKeychainBackend()
	request := BootstrapRequest{ProviderID: backend.ProviderID(), KeyID: uniqueService("test-goal-kek-" + strings.ReplaceAll(t.Name(), "/", "-")), Owner: "test-user", Purpose: "goalstore", Profile: contracts.CryptoClassicalCompatible}
	record, wrapper, err := backend.Bootstrap(context.Background(), request)
	if errors.Is(err, ErrKeychainUnavailable) {
		// The probe passed, so this is not a missing capability.
		t.Fatalf("native Keychain became unavailable after the probe passed (an unexpected interaction requirement?): %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.deleteForTest(record.KeyID, record.KeyVersion) })
	if record.Platform != runtime.GOOS || record.Architecture != runtime.GOARCH {
		t.Fatalf("unexpected platform binding: %+v", record)
	}
	plaintext := []byte("01234567890123456789012345678901")
	wrapped, err := wrapper.Wrap(context.Background(), request.KeyID, request.Profile, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := wrapper.Unwrap(context.Background(), wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if string(opened) != string(plaintext) {
		t.Fatal("Keychain round trip changed plaintext")
	}

	restarted, err := NewMacOSKeychainBackend().Open(context.Background(), record)
	if err != nil {
		t.Fatal(err)
	}
	if recovered, err := restarted.Unwrap(context.Background(), wrapped); err != nil || string(recovered) != string(plaintext) {
		t.Fatalf("restart recovery failed: %v", err)
	}
	if err := backend.deleteForTest(record.KeyID, record.KeyVersion); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMacOSKeychainBackend().Open(context.Background(), record); !errors.Is(err, ErrKeychainItemMissing) {
		t.Fatalf("missing Keychain item must fail as key failure: %v", err)
	}
}

func TestMacOSKeychainRejectsSubstitutionAndImplicitFallback(t *testing.T) {
	requireKeychain(t)
	backend := NewMacOSKeychainBackend()
	request := BootstrapRequest{ProviderID: backend.ProviderID(), KeyID: uniqueService("test-substitution-" + strings.ReplaceAll(t.Name(), "/", "-")), Owner: "test-user", Purpose: "goalstore", Profile: contracts.CryptoClassicalCompatible}
	record, wrapper, err := backend.Bootstrap(context.Background(), request)
	if errors.Is(err, ErrKeychainUnavailable) {
		// The probe passed, so this is not a missing capability.
		t.Fatalf("native Keychain became unavailable after the probe passed (an unexpected interaction requirement?): %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.deleteForTest(record.KeyID, record.KeyVersion) })
	wrong := record
	wrong.ProviderID = "other-provider"
	if _, err := backend.Open(context.Background(), wrong); err == nil {
		t.Fatal("provider substitution must fail closed")
	}
	if _, _, err := backend.Bootstrap(context.Background(), request); !errors.Is(err, ErrKeychainDuplicate) {
		t.Fatalf("existing item must not be replaced: %v", err)
	}
	if _, err := wrapper.Wrap(context.Background(), "other-key", contracts.CryptoClassicalCompatible, make([]byte, 32)); err == nil {
		t.Fatal("wrong key identity must fail closed")
	}
	if _, err := wrapper.Wrap(context.Background(), request.KeyID, contracts.CryptoPQRequired, make([]byte, 32)); err == nil {
		t.Fatal("unsupported stronger profile must not downgrade")
	}
}
