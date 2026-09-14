package crypto

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestBootstrapRecordStoreIsDigestBoundAndDoesNotReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.json")
	record := BootstrapRecord{Version: BootstrapRecordVersion, ProviderID: "provider", KeyID: "goal", KeyVersion: "1", KeyMaterialHash: "sha256:" + "0000000000000000000000000000000000000000000000000000000000000000", Owner: "user", Purpose: "goalstore", Profile: contracts.CryptoClassicalCompatible, SecurityLevel: SecurityPlatformProtected, Platform: "darwin", Architecture: "arm64", CreatedAt: time.Unix(1, 0).UTC()}
	if err := SaveBootstrapRecord(path, record); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadBootstrapRecord(path)
	if err != nil || loaded.ProviderID != record.ProviderID {
		t.Fatalf("load mismatch: %+v %v", loaded, err)
	}
	if err := SaveBootstrapRecord(path, record); !errors.Is(err, ErrBootstrapRecordExists) {
		t.Fatalf("replacement must fail closed: %v", err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBootstrapRecord(path); !errors.Is(err, ErrBootstrapRecordCorrupt) {
		t.Fatalf("corruption must fail closed: %v", err)
	}
}
