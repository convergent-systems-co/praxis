//go:build darwin

package crypto

import (
	"os"
	"path/filepath"

	"github.com/convergent-systems-co/praxis/internal/faa"
)

// NewTemporaryKeychainAnchor returns a Keychain anchor under a caller-chosen
// service name, keeping its dedicated keychain files in a temporary directory,
// together with the function that removes an installation's keychain and
// password item. It exists so qualification can exercise the REAL Keychain
// backend end to end without leaving items behind or touching the production
// service. Production code has no delete path and uses NewPlatformAnchor.
func NewTemporaryKeychainAnchor(service string) (faa.Anchor, func(installation string) error) {
	k := &KeychainAnchor{Service: service, Dir: filepath.Join(os.TempDir(), "praxis-faa-qualification-keychains")}
	return k, k.deleteForTest
}

// RemovePasswordItem deletes only the login-Keychain item that holds an
// installation's dedicated-keychain password, leaving the file: the silent
// deletion another process of the same user can do (the platform does not
// protect deletion). It exists for qualification, in process, so a test does not
// have to run an external tool that could raise a prompt of its own.
func (k *KeychainAnchor) RemovePasswordItem(installation string) error {
	_, _, service, account, err := k.layout(installation)
	if err != nil {
		return err
	}
	removePassword(service, account)
	return nil
}
