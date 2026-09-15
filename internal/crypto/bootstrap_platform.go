package crypto

// NewFirstPartyBootstrapRegistry registers only backends available in the
// current build. It never chooses a backend on behalf of the caller.
func NewFirstPartyBootstrapRegistry() (*BootstrapRegistry, error) {
	registry := NewBootstrapRegistry()
	if err := registerPlatformBootstrapBackends(registry); err != nil {
		return nil, err
	}
	return registry, nil
}
