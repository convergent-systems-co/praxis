package stateprovider

import (
	"context"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

// MemoryProvider is a bounded deterministic provider for semantic conformance
// tests. It advertises only event capabilities; unsupported package operations
// fail rather than imitating stronger transactional guarantees.
type MemoryProvider struct {
	events  *eventstore.MemoryStore
	profile Profile
}

func NewMemory() *MemoryProvider {
	return &MemoryProvider{events: eventstore.NewMemoryStore(), profile: Profile{
		EventsAppendOptimistic: Enforced,
		EventsReplay:           Enforced,
		EventsGlobalSequence:   Enforced,
	}}
}

func (p *MemoryProvider) Profile() Profile {
	out := Profile{}
	for capability, state := range p.profile {
		out[capability] = state
	}
	return out
}

func (p *MemoryProvider) Events() eventstore.Store  { return p.events }
func (p *MemoryProvider) Packages() PackageRegistry { return unsupportedPackageRegistry{} }

type unsupportedPackageRegistry struct{}

func (unsupportedPackageRegistry) ActivatePackage(context.Context, packagecatalog.Manifest, string, string, time.Time) error {
	return errors.New("provider does not support atomic package activation")
}
func (unsupportedPackageRegistry) DeactivatePackage(context.Context, string) error {
	return errors.New("provider does not support atomic package activation")
}
func (unsupportedPackageRegistry) RemovePackage(context.Context, string) error {
	return errors.New("provider does not support atomic package activation")
}
func (unsupportedPackageRegistry) ActiveInvocations(context.Context) ([]RegisteredInvocation, error) {
	return nil, errors.New("provider does not support package registry")
}
func (unsupportedPackageRegistry) ActiveContents(context.Context, packagecatalog.ContentKind) ([]RegisteredContent, error) {
	return nil, errors.New("provider does not support package registry")
}
func (unsupportedPackageRegistry) ResolveContent(context.Context, packagecatalog.ContentKind, string, string) (RegisteredContent, error) {
	return RegisteredContent{}, errors.New("provider does not support package registry")
}
func (unsupportedPackageRegistry) InstalledPackages(context.Context) ([]InstalledPackage, error) {
	return nil, errors.New("provider does not support package registry")
}
func (unsupportedPackageRegistry) ActivePackage(context.Context, string) (InstalledPackage, error) {
	return InstalledPackage{}, errors.New("provider does not support package registry")
}

func RequireEvents(provider Provider, required ...Capability) (eventstore.Store, error) {
	if provider == nil {
		return nil, errors.New("authoritative state provider is required")
	}
	if err := provider.Profile().Require(required...); err != nil {
		return nil, err
	}
	store := provider.Events()
	if store == nil {
		return nil, errors.New("provider advertised event semantics without an event store")
	}
	return store, nil
}

var _ Provider = (*MemoryProvider)(nil)
var _ PackageRegistry = unsupportedPackageRegistry{}
