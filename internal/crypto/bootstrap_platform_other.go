//go:build !darwin

package crypto

import "errors"

func registerPlatformBootstrapBackends(*BootstrapRegistry) error {
	return errors.New("no first-party bootstrap backend is available for this platform")
}
