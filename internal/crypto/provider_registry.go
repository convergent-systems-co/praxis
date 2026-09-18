package crypto

import (
	"errors"
	"fmt"
)

var ErrUnknownKeyProvider = errors.New("unknown cryptographic key provider")

// ProviderRegistry binds opaque key-provider identities to implementations.
// Key material remains inside the provider; the registry stores no secrets and
// does not make provider discovery an authority grant.
type ProviderRegistry struct {
	providers map[string]KeyWrapper
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: map[string]KeyWrapper{}}
}

func (r *ProviderRegistry) Register(providerID string, provider KeyWrapper) error {
	if r == nil {
		return errors.New("cryptographic provider registry is required")
	}
	if providerID == "" || provider == nil {
		return errors.New("cryptographic provider identity and implementation are required")
	}
	if _, exists := r.providers[providerID]; exists {
		return fmt.Errorf("cryptographic provider %q is already registered", providerID)
	}
	r.providers[providerID] = provider
	return nil
}

func (r *ProviderRegistry) Resolve(providerID string) (KeyWrapper, error) {
	if r == nil || providerID == "" {
		return nil, ErrUnknownKeyProvider
	}
	provider, ok := r.providers[providerID]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownKeyProvider, providerID)
	}
	return provider, nil
}

func (r *ProviderRegistry) Service(providerID string, policy EnvelopePolicy) (EnvelopeService, error) {
	provider, err := r.Resolve(providerID)
	if err != nil {
		return EnvelopeService{}, err
	}
	return EnvelopeService{Wrapper: provider, Policy: policy}, nil
}
