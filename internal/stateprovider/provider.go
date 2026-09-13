package stateprovider

import (
	"context"
	"errors"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type EnforcementState string

const (
	Enforced    EnforcementState = "enforced"
	Unsupported EnforcementState = "unsupported"
	Unknown     EnforcementState = "unknown"
)

type Capability string

const (
	EventsAppendOptimistic        Capability = "events.append_optimistic"
	EventsReplay                  Capability = "events.replay"
	EventsGlobalSequence          Capability = "events.global_sequence"
	ProjectionsCheckpointed       Capability = "projections.checkpointed"
	ApprovalAtomicConsume         Capability = "authority.approval_atomic_consume"
	LeaseAtomicConsume            Capability = "authority.lease_atomic_consume"
	EffectsOutboxReconciliation   Capability = "effects.outbox_reconciliation"
	SecureBlobsImmutableEncrypted Capability = "secure_blobs.immutable_encrypted"
	PackagesAtomicActivation      Capability = "packages.atomic_activation"
	RunsDurableReplay             Capability = "runs.durable_replay"
	MigrationsVersioned           Capability = "migrations.versioned"
	TransactionsMultiRepository   Capability = "transactions.multi_repository_atomic"
)

type Profile map[Capability]EnforcementState

func (p Profile) Require(required ...Capability) error {
	for _, capability := range required {
		if p[capability] != Enforced {
			return errors.New("authoritative state provider does not enforce required capability: " + string(capability))
		}
	}
	return nil
}

type RegisteredInvocation struct {
	Contract       contracts.InvocationContract
	ContentDigest  string
	ContractDigest string
}

type RegisteredContent struct {
	PackageID      string
	PackageVersion string
	PackageDigest  string
	Content        packagecatalog.ContentRef
}

type InstalledPackage struct {
	Manifest   packagecatalog.Manifest
	State      string
	SourceKind string
	SourceRef  string
}

type PackageRegistry interface {
	ActivatePackage(ctx context.Context, manifest packagecatalog.Manifest, sourceKind, sourceRef string, now time.Time) error
	DeactivatePackage(ctx context.Context, packageID string) error
	RemovePackage(ctx context.Context, packageID string) error
	ActiveInvocations(ctx context.Context) ([]RegisteredInvocation, error)
	ActiveContents(ctx context.Context, kind packagecatalog.ContentKind) ([]RegisteredContent, error)
	ResolveContent(ctx context.Context, kind packagecatalog.ContentKind, id, version string) (RegisteredContent, error)
	InstalledPackages(ctx context.Context) ([]InstalledPackage, error)
	ActivePackage(ctx context.Context, packageID string) (InstalledPackage, error)
}

type Provider interface {
	Profile() Profile
	Events() eventstore.Store
	Packages() PackageRegistry
}
