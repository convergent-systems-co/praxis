//go:build darwin

package crypto

func registerPlatformBootstrapBackends(registry *BootstrapRegistry) error {
	return registry.Register(NewMacOSKeychainBackend())
}
