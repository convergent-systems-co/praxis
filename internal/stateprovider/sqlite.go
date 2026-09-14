package stateprovider

import (
	"context"
	"database/sql"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
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

func (r sqlitePackageRegistry) ActivatePackage(ctx context.Context, request packagecatalog.ActivationRequest, now time.Time) error {
	return r.store.ActivatePackage(ctx, request, now)
}
func (r sqlitePackageRegistry) TransitionPackage(ctx context.Context, request packagecatalog.TransitionRequest, now time.Time) error {
	return r.store.TransitionPackage(ctx, request, now)
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
func (r sqlitePackageRegistry) ActiveContents(ctx context.Context, kind packagecatalog.ContentKind) ([]RegisteredContent, error) {
	items, err := r.store.ActiveContents(ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]RegisteredContent, 0, len(items))
	for _, item := range items {
		out = append(out, RegisteredContent{PackageID: item.PackageID, PackageVersion: item.PackageVersion, PackageDigest: item.PackageDigest, Content: item.Content, ArtifactBytes: append([]byte(nil), item.ArtifactBytes...)})
	}
	return out, nil
}
func (r sqlitePackageRegistry) ResolveContent(ctx context.Context, kind packagecatalog.ContentKind, id, version string) (RegisteredContent, error) {
	item, err := r.store.ResolveContent(ctx, kind, id, version)
	if err != nil {
		return RegisteredContent{}, err
	}
	return RegisteredContent{PackageID: item.PackageID, PackageVersion: item.PackageVersion, PackageDigest: item.PackageDigest, Content: item.Content, ArtifactBytes: append([]byte(nil), item.ArtifactBytes...)}, nil
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
func (r sqlitePackageRegistry) SelectedPackage(ctx context.Context, packageID string) (InstalledPackage, error) {
	item, err := r.store.SelectedPackage(ctx, packageID)
	if err != nil {
		return InstalledPackage{}, err
	}
	return InstalledPackage{Manifest: item.Manifest, State: item.State, SourceKind: item.SourceKind, SourceRef: item.SourceRef}, nil
}
func (r sqlitePackageRegistry) PrepareAgentInstantiation(ctx context.Context, packageID, definitionID, definitionVersion, agentID, generationID, ownerScope, governanceRef, approvalID string, actor contracts.PrincipalRef) (contracts.PackageAgentInstantiationRequest, error) {
	return r.store.PreparePackageAgentInstantiation(ctx, packageID, definitionID, definitionVersion, agentID, generationID, ownerScope, governanceRef, approvalID, actor)
}
func (r sqlitePackageRegistry) InstantiateAgent(ctx context.Context, request contracts.PackageAgentInstantiationRequest, now time.Time) (contracts.PackageAgentInstance, error) {
	return r.store.InstantiatePackageAgent(ctx, request, now)
}

var _ Provider = (*SQLiteProvider)(nil)
var _ PackageRegistry = sqlitePackageRegistry{}
