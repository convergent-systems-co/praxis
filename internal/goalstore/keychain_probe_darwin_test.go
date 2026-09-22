//go:build darwin

package goalstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/faa"
)

// Qualification must never wait for a person: with interaction disabled a
// Keychain prompt surfaces at once as errSecInteractionNotAllowed (-25308).
func TestMain(m *testing.M) {
	restore := praxiscrypto.DisableUserInteraction()
	code := m.Run()
	restore()
	os.Exit(code)
}

// uniqueKeychainService names a Keychain service no other test binary, run or
// mutation copy can share.
func uniqueKeychainService(base string) string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s.%d.%s", base, os.Getpid(), hex.EncodeToString(raw))
}

var keychainProbe struct {
	once sync.Once
	err  error
}

// requireKeychain decides once, with a fresh unique service and interaction
// disabled, whether this environment can run the real Keychain. Only that probe
// may justify a skip; after it passes any later unavailability is a failure, so a
// regression that starts needing a person cannot become a skipped test.
// PRAXIS_REQUIRE_KEYCHAIN=1 makes even a failed probe a failure.
func requireKeychain(t *testing.T) {
	t.Helper()
	keychainProbe.once.Do(func() {
		anchor, remove := praxiscrypto.NewTemporaryKeychainAnchor(uniqueKeychainService("com.convergent-systems-co.praxis.faa.qualification.probe"))
		installation := "sha256:00000000000000000000000000000000000000000000000000000000000000f1"
		defer remove(installation)
		_, g, err := faa.NewGenesis(installation, "probe", time.Now())
		if err != nil {
			keychainProbe.err = err
			return
		}
		if err := anchor.Set(context.Background(), installation, nil, g); err != nil {
			keychainProbe.err = err
			return
		}
		if got, err := anchor.Load(context.Background(), installation); err != nil || got != g {
			keychainProbe.err = fmt.Errorf("probe read-back: %+v %v", got, err)
		}
	})
	if keychainProbe.err == nil {
		return
	}
	if os.Getenv("PRAXIS_REQUIRE_KEYCHAIN") != "" {
		t.Fatalf("PRAXIS_REQUIRE_KEYCHAIN is set and the real Keychain probe failed: %v", keychainProbe.err)
	}
	t.Skipf("the real Keychain is unavailable in this environment (probe: %v); set PRAXIS_REQUIRE_KEYCHAIN=1 to make this a failure", keychainProbe.err)
}
