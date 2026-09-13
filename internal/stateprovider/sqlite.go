package stateprovider

import (
	"context"
	"database/sql"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
)

type SQLiteProvider struct {
	db    *sql.DB
	store *state.Store
}

func OpenSQLite(ctx context.Context, path string) (*SQLiteProvider, error) {
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		return nil, err
	}
	return NewSQLite(db), nil
}

func NewSQLite(db *sql.DB) *SQLiteProvider {
	return &SQLiteProvider{db: db, store: state.New(db)}
}

func (p *SQLiteProvider) Close() error {
	if p == nil || p.db == nil {
		return nil
	}
	return p.db.Close()
}

func (p *SQLiteProvider) Profile() Profile {
	return Profile{
		EventsAppendOptimistic:        Enforced,
		EventsReplay:                  Enforced,
		EventsGlobalSequence:          Enforced,
		ProjectionsCheckpointed:       Enforced,
		ApprovalAtomicConsume:         Enforced,
		LeaseAtomicConsume:            Enforced,
		EffectsOutboxReconciliation:   Enforced,
		SecureBlobsImmutableEncrypted: Enforced,
		PackagesAtomicActivation:      Enforced,
		RunsDurableReplay:             Enforced,
		MigrationsVersioned:           Enforced,
		TransactionsMultiRepository:   Enforced,
	}
}

func (p *SQLiteProvider) Events() eventstore.Store {
	return state.NewSQLiteEventStore(p.db)
}

func (p *SQLiteProvider) Packages() PackageRegistry { return sqlitePackageRegistry{store: p.store} }

type sqlitePackageRegistry struct{ store *state.Store }

func (r sqlitePackageRegistry) ActivatePackage(ctx context.Context, manifest packagecatalog.Manifest, sourceKind, sourceRef string, now time.Time) error {
	return r.store.ActivatePackage(ctx, manifest, sourceKind, sourceRef, now)
}
func (r sqlitePackageRegistry) DeactivatePackage(ctx context.Context, packageID string) error {
	return r.store.DeactivatePackage(ctx, packageID)
}
func (r sqlitePackageRegistry) RemovePackage(ctx context.Context, packageID string) error {
	return r.store.RemovePackage(ctx, packageID)
}
func (r sqlitePackageRegistry) ActiveInvocations(ctx context.Context) ([]RegisteredInvocation, error) {
	items, err := r.store.ActiveInvocations(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RegisteredInvocation, 0, len(items))
	for _, item := range items {
		out = append(out, RegisteredInvocation{Contract: item.Contract, ContentDigest: item.ContentDigest, ContractDigest: item.ContractDigest})
	}
	return out, nil
}
func (r sqlitePackageRegistry) InstalledPackages(ctx context.Context) ([]InstalledPackage, error) {
	items, err := r.store.InstalledPackages(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]InstalledPackage, 0, len(items))
	for _, item := range items {
		out = append(out, InstalledPackage{Manifest: item.Manifest, State: item.State, SourceKind: item.SourceKind, SourceRef: item.SourceRef})
	}
	return out, nil
}
func (r sqlitePackageRegistry) ActivePackage(ctx context.Context, packageID string) (InstalledPackage, error) {
	item, err := r.store.ActivePackage(ctx, packageID)
	if err != nil {
		return InstalledPackage{}, err
	}
	return InstalledPackage{Manifest: item.Manifest, State: item.State, SourceKind: item.SourceKind, SourceRef: item.SourceRef}, nil
}

var _ Provider = (*SQLiteProvider)(nil)
var _ PackageRegistry = sqlitePackageRegistry{}
