package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// revertInvocationRuntimeBindingsToHistorical rebuilds a freshly-migrated
// (canonical-shape) invocation_runtime_bindings table backward into the
// exact historical shape (commit 0084352), for test setup only — this is
// the mirror image of the driver's own forward rebuild, used to construct
// a realistic fixture (a fully-migrated database whose invocation_registry/
// installed_packages tables are the real current schema, with only
// invocation_runtime_bindings deliberately diverged) rather than a
// hand-rolled minimal schema that might not match production.
func revertInvocationRuntimeBindingsToHistorical(ctx context.Context, db *sql.DB) error {
	// The CREATE TABLE/INDEX/schema_meta body below is byte-for-byte the
	// 0084352 migration body; the final statement is the exact 0013 body.
	const historical0012 = `CREATE TABLE invocation_runtime_bindings (
    entry_point_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    package_id TEXT NOT NULL,
    contract_digest TEXT NOT NULL,
    handler_id TEXT NOT NULL,
    handler_version TEXT NOT NULL,
    handler_digest TEXT NOT NULL,
    registered_at TEXT NOT NULL,
    PRIMARY KEY (entry_point_id, package_version, content_digest),
    FOREIGN KEY (entry_point_id, package_version, content_digest)
      REFERENCES invocation_registry(entry_point_id, package_version, content_digest)
);

CREATE INDEX idx_invocation_runtime_bindings_package
ON invocation_runtime_bindings(package_id, content_digest);

UPDATE schema_meta SET value='12' WHERE key='schema_version';`
	const executable0013 = `ALTER TABLE invocation_runtime_bindings
    ADD COLUMN executable_binding_json BLOB;

UPDATE schema_meta SET value='13' WHERE key='schema_version';`
	stmts := []string{`DROP TABLE invocation_runtime_bindings`, historical0012, executable0013}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

type bindingRowFixture struct {
	entryPointID, packageID, packageVersion, contentDigest string
	active                                                 bool
	executableBinding                                      []byte
}

// seedInvocationRuntimeBindingsFixture inserts a realistic installed_packages
// + invocation_registry + invocation_runtime_bindings row set (historical
// handler_* shape) directly, covering both an active and an inactive
// (historical, retained) generation — exactly the population WU5 must
// preserve unconditionally.
func seedInvocationRuntimeBindingsFixture(t *testing.T, db *sql.DB, rows []bindingRowFixture) {
	t.Helper()
	ctx := context.Background()
	for i, r := range rows {
		stamp := time.Now().UTC().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		active := 0
		installedState := "superseded"
		if r.active {
			active = 1
			installedState = "active"
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO installed_packages(package_id,package_version,content_digest,state,source_kind,source_ref,manifest_json,installed_at) VALUES(?,?,?,?,?,?,?,?)`,
			r.packageID, r.packageVersion, r.contentDigest, installedState, "test", "test", []byte(`{}`), stamp); err != nil {
			t.Fatalf("seed installed_packages: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO invocation_registry(entry_point_id,package_id,package_version,content_digest,graph_id,graph_version,contract_json,contract_digest,active,registered_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			r.entryPointID, r.packageID, r.packageVersion, r.contentDigest, "graph:test", "1", []byte(`{}`), "sha256:"+r.contentDigest[7:], active, stamp); err != nil {
			t.Fatalf("seed invocation_registry: %v", err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO invocation_runtime_bindings(entry_point_id,package_version,content_digest,package_id,contract_digest,handler_id,handler_version,handler_digest,registered_at,executable_binding_json) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			r.entryPointID, r.packageVersion, r.contentDigest, r.packageID, "sha256:"+r.contentDigest[7:], "client-adapter:"+r.packageID+":"+r.entryPointID, "1", "sha256:handler-"+r.entryPointID, stamp, r.executableBinding); err != nil {
			t.Fatalf("seed invocation_runtime_bindings: %v", err)
		}
	}
}

func newHistoricalFixtureDB(t *testing.T, rows []bindingRowFixture) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := revertInvocationRuntimeBindingsToHistorical(ctx, db); err != nil {
		t.Fatalf("revert fixture to historical shape: %v", err)
	}
	seedInvocationRuntimeBindingsFixture(t, db, rows)
	return db
}

func sqliteFixturePath(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`PRAGMA database_list`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int
		var name, path string
		if err := rows.Scan(&seq, &name, &path); err != nil {
			t.Fatal(err)
		}
		if name == "main" {
			return path
		}
	}
	t.Fatal("SQLite main database path not found")
	return ""
}

func storageSchemaStep(t *testing.T, targetDigest string) contracts.LifecycleTransitionStep {
	t.Helper()
	historicalDigest, err := HistoricalInvocationRuntimeBindingsShape().Digest()
	if err != nil {
		t.Fatal(err)
	}
	decision := storageSchemaAuthorityDecision(t)
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.LifecycleTransitionStep{
		ID:       "storage-schema",
		Sequence: 1, Class: contracts.LifecycleSchema,
		Current:       contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: InvocationRuntimeBindingsTable, Version: "1", Digest: historicalDigest},
		Target:        contracts.LifecycleComponentRef{Class: contracts.LifecycleSchema, ID: InvocationRuntimeBindingsTable, Version: "2", Digest: targetDigest},
		Preconditions: []contracts.LifecycleEvidenceRef{{ID: "pre", Kind: "test", Source: "fixture", Digest: migrationDigest("c")}},
		Authority: contracts.LifecycleAuthorityRequirement{
			Required: true, Operation: contracts.GovernedInstallationRepairStorageSchema, Scope: "installation-repair:installation",
			RequestRef: decision.RequestID, RequestVersion: decision.RequestVersion, RequestDigest: decision.RequestDigest,
			DecisionRef: decision.DecisionRef, DecisionVersion: decision.DecisionVersion, DecisionDigest: decisionDigest,
			AuthorityRef: decision.AuthorityRef, AuthorityVersion: decision.AuthorityVersion, AuthorityGenerationDigest: decision.AuthorityGenerationDigest,
		},
		Effect:           contracts.LifecycleAuthorityBound,
		SnapshotRequired: true,
		Reversible:       true,
		RecoveryStrategy: "rebuild-or-noop",
		ReadinessImpact:  "schema",
	}
}

func storageSchemaAuthorityDecision(t *testing.T) contracts.AuthorityDecision {
	t.Helper()
	return contracts.AuthorityDecision{
		RequestID: "storage-schema-request", RequestVersion: "1", RequestDigest: migrationDigest("1"),
		DecisionRef: "storage-schema-decision", DecisionVersion: "1", AuthorityRef: "root-owner-generation", AuthorityVersion: "1",
		AuthorityGenerationDigest: migrationDigest("2"), DecidedBy: contracts.PrincipalRef{ID: "installation-owner", Kind: "human"},
		GrantedScope: "installation-repair:installation", Outcome: contracts.AuthorityApprove, AuthorityDigest: migrationDigest("3"),
		IssuedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC),
	}
}

type allowingGuard struct{ calls int }

func (g *allowingGuard) RevalidateInTx(context.Context, *sql.Tx) error {
	g.calls++
	return nil
}

type resultCapturingStorageSchemaDriver struct {
	inner  *StorageSchemaDriver
	result ApplyResult
}

type nonIdempotentStorageSchemaDriver struct{ inner *StorageSchemaDriver }

func (d *nonIdempotentStorageSchemaDriver) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	return d.inner.BindRunRequest(ctx, req, step, authority)
}
func (d *nonIdempotentStorageSchemaDriver) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	return d.inner.Preflight(ctx, step)
}
func (*nonIdempotentStorageSchemaDriver) Idempotent(contracts.LifecycleTransitionStep) bool {
	return false
}
func (d *nonIdempotentStorageSchemaDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	return d.inner.Apply(ctx, step)
}

func (d *resultCapturingStorageSchemaDriver) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	return d.inner.BindRunRequest(ctx, req, step, authority)
}
func (d *resultCapturingStorageSchemaDriver) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	return d.inner.Preflight(ctx, step)
}
func (d *resultCapturingStorageSchemaDriver) Idempotent(step contracts.LifecycleTransitionStep) bool {
	return d.inner.Idempotent(step)
}
func (d *resultCapturingStorageSchemaDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	result, err := d.inner.Apply(ctx, step)
	d.result = result
	return result, err
}

func preparedStorageDriver(t *testing.T, db *sql.DB, driver *StorageSchemaDriver) *StorageSchemaDriver {
	t.Helper()
	prepared, err := PrepareStorageSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	driver.Prepared = prepared
	return driver
}

func storageSchemaRequest(t *testing.T, plan contracts.LifecyclePlan, step contracts.LifecycleTransitionStep, driver *StorageSchemaDriver) RunRequest {
	t.Helper()
	digest, err := driver.Prepared.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return RunRequest{Plan: plan, StepID: step.ID, PreconditionDigest: digest, SnapshotDigest: migrationDigest("f"), Authority: migrationAuthority{decision: storageSchemaAuthorityDecision(t)}, Now: time.Now().UTC()}
}

type bindingRowSnapshot struct {
	EntryPointID, PackageVersion, ContentDigest, PackageID, ContractDigest []byte
	RuntimeID, RuntimeVersion, RuntimeDigest, RegisteredAt                 []byte
	ExecutableBinding                                                      []byte
	ExecutableType                                                         string
}

func snapshotBindingRows(t *testing.T, db *sql.DB, historical bool) []bindingRowSnapshot {
	t.Helper()
	runtimeColumns := "runtime_id,runtime_version,runtime_digest"
	if historical {
		runtimeColumns = "handler_id,handler_version,handler_digest"
	}
	rows, err := db.Query(`SELECT entry_point_id,package_version,content_digest,package_id,contract_digest,` + runtimeColumns + `,registered_at,executable_binding_json,typeof(executable_binding_json) FROM invocation_runtime_bindings ORDER BY entry_point_id,package_version,content_digest`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []bindingRowSnapshot
	for rows.Next() {
		var r bindingRowSnapshot
		if err := rows.Scan(&r.EntryPointID, &r.PackageVersion, &r.ContentDigest, &r.PackageID, &r.ContractDigest, &r.RuntimeID, &r.RuntimeVersion, &r.RuntimeDigest, &r.RegisteredAt, &r.ExecutableBinding, &r.ExecutableType); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertForeignKeyCheckClean(t *testing.T, db *sql.DB) {
	t.Helper()
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("PRAGMA foreign_key_check reported a violation")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func storageSchemaPlan(t *testing.T, step contracts.LifecycleTransitionStep) contracts.LifecyclePlan {
	t.Helper()
	p := contracts.LifecyclePlan{
		PlanID: "plan-storage-schema", PlanVersion: "1", InstallationID: "installation",
		CurrentManifestDigest: step.Current.Digest, TargetManifestDigest: step.Target.Digest,
		Steps: []contracts.LifecycleTransitionStep{step}, SnapshotRequired: true,
		ExpectedReadiness: migrationReadiness(),
	}
	digest, err := p.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	p.Digest = digest
	return p
}

func canonicalDigest(t *testing.T) string {
	t.Helper()
	d, err := CanonicalInvocationRuntimeBindingsShape().Digest()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func tableRootpage(t *testing.T, db *sql.DB, table string) int64 {
	t.Helper()
	var rootpage int64
	if err := db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&rootpage); err != nil {
		t.Fatal(err)
	}
	return rootpage
}

func TestStorageSchemaDriverRebuildsHistoricalShapeAndPreservesAllRows(t *testing.T) {
	rows := []bindingRowFixture{
		{entryPointID: "ep-a", packageID: "pkg-a", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true, executableBinding: []byte(`{"kind":"active"}`)},
		{entryPointID: "ep-a", packageID: "pkg-a", packageVersion: "0", contentDigest: "sha256:" + repeatHex("0"), active: false, executableBinding: nil},
		{entryPointID: "ep-b", packageID: "pkg-b", packageVersion: "1", contentDigest: "sha256:" + repeatHex("b"), active: true, executableBinding: []byte{0x00, 0x01, 0xfe, 0xff}},
	}
	db := newHistoricalFixtureDB(t, rows)
	ctx := context.Background()
	beforeRows := snapshotBindingRows(t, db, true)

	beforeRootpage := tableRootpage(t, db, InvocationRuntimeBindingsTable)
	var beforeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM invocation_runtime_bindings`).Scan(&beforeCount); err != nil {
		t.Fatal(err)
	}
	if beforeCount != len(rows) {
		t.Fatalf("fixture did not seed expected row count: %d", beforeCount)
	}

	target := canonicalDigest(t)
	step := storageSchemaStep(t, target)
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	guard := &allowingGuard{}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: guard})
	req := storageSchemaRequest(t, plan, step, driver)
	if err := RunStep(ctx, journal, req, driver); err != nil {
		t.Fatal(err)
	}

	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := history[len(history)-1]
	if last.State != contracts.LifecycleCommitted {
		t.Fatalf("expected committed, got %+v", last)
	}
	if last.PlanID != req.Plan.PlanID || last.PlanDigest != req.Plan.Digest || last.InstallationID != req.Plan.InstallationID || last.StepID != req.StepID || last.PreconditionDigest != req.PreconditionDigest || last.SnapshotDigest != req.SnapshotDigest || last.ReadinessDigest != req.Plan.TargetManifestDigest {
		t.Fatalf("driver outcome was not bound to the exact RunRequest: %+v", last)
	}
	decision := storageSchemaAuthorityDecision(t)
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if last.AuthorityRef != decision.AuthorityRef || last.AuthorityVersion != decision.AuthorityVersion || last.AuthorityDecisionRef != decision.DecisionRef || last.AuthorityDecisionDigest != decisionDigest || last.AuthorityGenerationDigest != decision.AuthorityGenerationDigest {
		t.Fatalf("driver outcome was not bound to RunStep's validated authority decision: %+v", last)
	}
	if guard.calls != 1 {
		t.Fatalf("authority guard calls = %d, want 1", guard.calls)
	}

	shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if classifyInvocationRuntimeBindingsShape(shape) != shapeCanonicalRuntime {
		t.Fatalf("live shape is not canonical after rebuild: %+v", shape)
	}
	if !shapesEqual(shape, CanonicalInvocationRuntimeBindingsShape()) {
		t.Fatalf("rebuilt live schema differs from exact current migration shape: %+v", shape)
	}
	if shape.ForeignKeys[0].OnDelete != "CASCADE" {
		t.Fatalf("expected ON DELETE CASCADE after rebuild: %+v", shape.ForeignKeys[0])
	}

	afterRootpage := tableRootpage(t, db, InvocationRuntimeBindingsTable)
	if afterRootpage == beforeRootpage {
		t.Fatal("expected the rebuild to actually recreate the table (different rootpage)")
	}

	var afterCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM invocation_runtime_bindings`).Scan(&afterCount); err != nil {
		t.Fatal(err)
	}
	if afterCount != len(rows) {
		t.Fatalf("row count changed across rebuild: before=%d after=%d", beforeCount, afterCount)
	}
	afterRows := snapshotBindingRows(t, db, false)
	if !reflect.DeepEqual(afterRows, beforeRows) {
		t.Fatalf("retained rows were not preserved byte-for-byte, including inactive/null history:\nbefore=%#v\nafter=%#v", beforeRows, afterRows)
	}
	assertForeignKeyCheckClean(t, db)
}

func TestStorageSchemaDriverAlreadyCanonicalIsALegalNoOp(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	beforeRootpage := tableRootpage(t, db, InvocationRuntimeBindingsTable)

	target := canonicalDigest(t)
	step := storageSchemaStep(t, target)
	step.Current.Digest = target
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	guard := &allowingGuard{}
	var domainWrites []string
	inner := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: guard, observeDomainMutation: func(statement string) { domainWrites = append(domainWrites, statement) }})
	driver := &resultCapturingStorageSchemaDriver{inner: inner}
	req := storageSchemaRequest(t, plan, step, inner)
	if err := RunStep(ctx, journal, req, driver); err != nil {
		t.Fatal(err)
	}

	afterRootpage := tableRootpage(t, db, InvocationRuntimeBindingsTable)
	if afterRootpage != beforeRootpage {
		t.Fatal("already-canonical Apply must issue zero mutating DDL (rootpage must not change)")
	}
	if len(domainWrites) != 0 {
		t.Fatalf("already-canonical Apply issued domain-mutating SQL: %q", domainWrites)
	}
	if driver.result.Outcome != ApplyCommitted || driver.result.ResultingManifestDigest != step.Target.Digest || !driver.result.OutcomeAlreadyRecorded {
		t.Fatalf("already-canonical Apply returned the wrong qualified result: %+v", driver.result)
	}

	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := history[len(history)-1]
	if last.State != contracts.LifecycleCommitted {
		t.Fatalf("already-canonical case must still reach committed: %+v", last)
	}
}

func TestStorageSchemaDriverRejectsUnrecognizedShape(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `ALTER TABLE invocation_runtime_bindings ADD COLUMN unexpected_column TEXT`); err != nil {
		t.Fatal(err)
	}

	target := canonicalDigest(t)
	step := storageSchemaStep(t, target)
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: StorageSchemaPreparation{SchemaDigest: step.Current.Digest, RetainedPopulationDigest: migrationDigest("d"), TargetPreStateDigest: migrationDigest("e")}}
	req := storageSchemaRequest(t, plan, step, driver)
	if err := RunStep(ctx, journal, req, driver); !errors.Is(err, ErrDowngradeRefused) {
		t.Fatalf("expected ErrDowngradeRefused for an unrecognized shape, got %v", err)
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("a Preflight refusal must not append any journal entry: %+v", history)
	}
}

func TestStorageSchemaIntrospectionRejectsEveryBehaviorAffectingStructuralDrift(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
	}{
		{name: "default", sql: `ALTER TABLE invocation_runtime_bindings ADD COLUMN drift TEXT DEFAULT 'x'`},
		{name: "unique constraint", sql: `CREATE UNIQUE INDEX drift_unique ON invocation_runtime_bindings(package_id, content_digest)`},
		{name: "partial index predicate", sql: `CREATE INDEX drift_partial ON invocation_runtime_bindings(package_id) WHERE package_version='1'`},
		{name: "trigger", sql: `CREATE TRIGGER drift_trigger AFTER UPDATE ON invocation_runtime_bindings BEGIN SELECT 1; END`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newHistoricalFixtureDB(t, nil)
			if _, err := db.Exec(tc.sql); err != nil {
				t.Fatal(err)
			}
			shape, err := IntrospectInvocationRuntimeBindingsShape(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if classifyInvocationRuntimeBindingsShape(shape) != shapeUnrecognized {
				t.Fatalf("%s drift was not represented in the observed structural identity: %+v", tc.name, shape)
			}
			if _, err := PrepareStorageSchema(context.Background(), db); !errors.Is(err, ErrUnrecognizedSchemaShape) {
				t.Fatalf("%s drift was admitted for repair: %v", tc.name, err)
			}
		})
	}
}

type failingGuard struct{ err error }

func (g failingGuard) RevalidateInTx(context.Context, *sql.Tx) error { return g.err }

type lifecycleAuthorityTestWrapper struct{}

func (lifecycleAuthorityTestWrapper) Capabilities(context.Context, string) (praxiscrypto.Capabilities, error) {
	return praxiscrypto.Capabilities{Classical: true}, nil
}
func (lifecycleAuthorityTestWrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, key []byte) (praxiscrypto.WrappedKey, error) {
	return praxiscrypto.WrappedKey{Ciphertext: append([]byte(nil), key...), SuiteID: "test", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (lifecycleAuthorityTestWrapper) Unwrap(_ context.Context, wrapped praxiscrypto.WrappedKey) ([]byte, error) {
	return append([]byte(nil), wrapped.Ciphertext...), nil
}

func sealedLifecycleAuthorityRecord(t *testing.T, service praxiscrypto.EnvelopeService, namespace, id, version, digest string, now time.Time) state.SecureBlobRecord {
	t.Helper()
	envelope, err := service.Seal(context.Background(), "test-key", contracts.CryptoClassicalCompatible, []byte("authority-evidence"), state.SecureBlobAAD(namespace, id, version, digest))
	if err != nil {
		t.Fatal(err)
	}
	return state.SecureBlobRecord{Namespace: namespace, ObjectID: id, ObjectVersion: version, ObjectDigest: digest, Sensitivity: state.SensitivityConfidential, CryptoProfile: contracts.CryptoClassicalCompatible, Envelope: envelope, CreatedAt: now}
}

func persistLifecycleRepairRoot(t *testing.T, db *sql.DB, service praxiscrypto.EnvelopeService, now time.Time) contracts.AuthorityGeneration {
	t.Helper()
	ctx := context.Background()
	bootstrap := migrationDigest("9")
	owner, _ := contracts.InstallationOwnerPrincipal(bootstrap)
	scope, _ := contracts.InstallationGovernanceScope(bootstrap)
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: "test-key", Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: bootstrap}
	root := contracts.AuthorityGeneration{Ref: scope, Version: "1", Principal: owner, Scope: scope, Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:" + bootstrap + ":os-user:test", ProvenanceDigest: bootstrap, State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-time.Hour), AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelVersion, AuthorityModelDigest: contracts.AuthorityModelDigest()}
	var err error
	root.Digest, err = root.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	proposal, err := contracts.BuildRootAuthoritySuccession(root, bootstrap, now)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := repo.SaveRootAuthoritySuccessionProposal(ctx, proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	_, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(ctx, proposalDigest, owner, "test", "REVIEW-ROOT-SUCCESSOR "+proposalDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	successor, _, err := repo.AcceptRootAuthoritySuccession(ctx, proposalDigest, reviewDigest, bootstrap, "test", "ACCEPT-ROOT-SUCCESSOR "+proposalDigest+" "+reviewDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	return successor
}

func TestStorageSchemaTransactionalGuardObservesExpiryAndRevocationAfterPrepared(t *testing.T) {
	for _, mode := range []string{"expiry", "revocation", "generation-invalidation"} {
		t.Run(mode, func(t *testing.T) {
			db := newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: migrationDigest("a"), active: true}})
			ctx := context.Background()
			now := time.Now().UTC()
			service := praxiscrypto.EnvelopeService{Wrapper: lifecycleAuthorityTestWrapper{}}
			store := state.New(db)
			generation := persistLifecycleRepairRoot(t, db, service, now)
			var err error
			expires := now.Add(time.Minute)
			decision := storageSchemaAuthorityDecision(t)
			decision.AuthorityRef, decision.AuthorityVersion, decision.AuthorityGenerationDigest = generation.Ref, generation.Version, generation.Digest
			decision.IssuedAt, decision.ExpiresAt = now, &expires
			decisionRecord := sealedLifecycleAuthorityRecord(t, service, authorityDecisionNamespace, decision.RequestID, decision.RequestVersion, migrationDigest("8"), now)
			if err := store.PutSecureBlob(ctx, decisionRecord); err != nil {
				t.Fatal(err)
			}
			guard, err := NewTransactionalAuthorityGuard(ctx, store, decision, now)
			if err != nil {
				t.Fatal(err)
			}
			clock := now
			guard.Now = func() time.Time { return clock }
			step := storageSchemaStep(t, canonicalDigest(t))
			decisionDigest, err := decision.Digest()
			if err != nil {
				t.Fatal(err)
			}
			step.Authority.DecisionRef, step.Authority.DecisionVersion, step.Authority.DecisionDigest = decision.DecisionRef, decision.DecisionVersion, decisionDigest
			step.Authority.AuthorityRef, step.Authority.AuthorityVersion, step.Authority.AuthorityGenerationDigest = generation.Ref, generation.Version, generation.Digest
			plan := storageSchemaPlan(t, step)
			journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
			if err != nil {
				t.Fatal(err)
			}
			driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: guard})
			driver.applyCheckpoint = func(stage string) error {
				if stage != "before-transaction" {
					return nil
				}
				if mode == "expiry" {
					clock = expires
					return nil
				}
				namespace, id, version := authorityRevocationNamespace, decision.RequestID, decision.RequestVersion
				if mode == "generation-invalidation" {
					namespace, id, version = authorityGenerationInvalidationNamespace, generation.Ref, generation.Version
				}
				revocation := sealedLifecycleAuthorityRecord(t, service, namespace, id, version, migrationDigest("7"), time.Now().UTC())
				return store.PutSecureBlob(ctx, revocation)
			}
			precondition, err := driver.Prepared.Digest()
			if err != nil {
				t.Fatal(err)
			}
			req := RunRequest{Plan: plan, StepID: step.ID, PreconditionDigest: precondition, SnapshotDigest: migrationDigest("f"), Authority: migrationAuthority{decision: decision}, Now: now}
			if err := RunStep(ctx, journal, req, driver); err == nil {
				t.Fatalf("transaction-time authority %s was accepted", mode)
			}
			shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
			if err != nil || classifyInvocationRuntimeBindingsShape(shape) != shapeHistoricalHandler {
				t.Fatalf("authority %s rejection occurred after mutation: shape=%+v err=%v", mode, shape, err)
			}
			history := mustLoadHistory(t, journal)
			if history[len(history)-1].State != contracts.LifecycleApplying {
				t.Fatalf("authority %s rejection appended an outcome: %+v", mode, history)
			}
		})
	}
}

func TestStorageSchemaDriverGuardFailureRollsBackWithNoMutationAndNoDuplicateOutcome(t *testing.T) {
	rows := []bindingRowFixture{{entryPointID: "ep-a", packageID: "pkg-a", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true}}
	db := newHistoricalFixtureDB(t, rows)
	ctx := context.Background()
	beforeRootpage := tableRootpage(t, db, InvocationRuntimeBindingsTable)

	target := canonicalDigest(t)
	step := storageSchemaStep(t, target)
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: failingGuard{err: errors.New("authority expired")}})
	req := storageSchemaRequest(t, plan, step, driver)
	if err := RunStep(ctx, journal, req, driver); err == nil {
		t.Fatal("expired authority was accepted")
	}

	afterRootpage := tableRootpage(t, db, InvocationRuntimeBindingsTable)
	if afterRootpage != beforeRootpage {
		t.Fatal("a failed authority guard must leave the table completely untouched")
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := history[len(history)-1]
	if last.State != contracts.LifecycleApplying {
		t.Fatalf("authority rejection must not append an outcome outside the rolled-back transaction, got %+v", last)
	}
}

func TestStorageSchemaDriverRejectsPreparedTargetPreStateDriftBeforeRebuild(t *testing.T) {
	db := newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: migrationDigest("a"), active: false, executableBinding: []byte("before")}})
	ctx := context.Background()
	step := storageSchemaStep(t, canonicalDigest(t))
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET executable_binding_json=? WHERE entry_point_id='ep'`, []byte("after")); err != nil {
		t.Fatal(err)
	}
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err == nil {
		t.Fatal("prepared target pre-state drift was accepted")
	}
	shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if classifyInvocationRuntimeBindingsShape(shape) != shapeHistoricalHandler {
		t.Fatal("target pre-state refusal occurred after structural mutation")
	}
	history := mustLoadHistory(t, journal)
	if history[len(history)-1].State != contracts.LifecycleApplying {
		t.Fatalf("pre-state refusal appended a non-transactional outcome: %+v", history)
	}
}

func TestStoragePreparationDistinguishesNullFromEmptyExecutableBinding(t *testing.T) {
	db := newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: migrationDigest("a"), active: false, executableBinding: nil}})
	before, err := PrepareStorageSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET executable_binding_json=x'' WHERE entry_point_id='ep'`); err != nil {
		t.Fatal(err)
	}
	after, err := PrepareStorageSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if before.TargetPreStateDigest == after.TargetPreStateDigest {
		t.Fatal("prepared target identity collapsed NULL and empty executable_binding_json")
	}
}

func TestStoragePreparationHashesExactMalformedTextBytes(t *testing.T) {
	db := newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: migrationDigest("a"), active: false}})
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET registered_at=CAST(X'80' AS TEXT) WHERE entry_point_id='ep'`); err != nil {
		t.Fatal(err)
	}
	first, err := PrepareStorageSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET registered_at=CAST(X'81' AS TEXT) WHERE entry_point_id='ep'`); err != nil {
		t.Fatal(err)
	}
	second, err := PrepareStorageSchema(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if first.TargetPreStateDigest == second.TargetPreStateDigest {
		t.Fatal("prepared target identity normalized distinct malformed SQLite TEXT bytes")
	}
}

func TestStorageSchemaDriverRejectsRequiredAuthorityWithoutGuard(t *testing.T) {
	db := newHistoricalFixtureDB(t, nil)
	ctx := context.Background()
	step := storageSchemaStep(t, canonicalDigest(t))
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal})
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err == nil {
		t.Fatal("authority-bound storage_schema step accepted a nil in-transaction guard")
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("missing guard must be rejected in Preflight before journaling: %+v", history)
	}
}

func TestStorageSchemaDriverCrashMidRebuildLeavesOriginalTableCompletelyIntact(t *testing.T) {
	fixtures := []bindingRowFixture{
		{entryPointID: "ep-active", packageID: "pkg-a", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true, executableBinding: []byte{0x00, 0xff}},
		{entryPointID: "ep-history", packageID: "pkg-a", packageVersion: "0", contentDigest: "sha256:" + repeatHex("b"), active: false, executableBinding: nil},
	}
	db := newHistoricalFixtureDB(t, fixtures)
	ctx := context.Background()
	beforeShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	beforeRows := snapshotBindingRows(t, db, true)
	step := storageSchemaStep(t, canonicalDigest(t))
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := &StorageSchemaDriver{
		DB: db, Journal: journal, Guard: &allowingGuard{},
		rebuildCheckpoint: func(stage string) error {
			if stage == "original-dropped" {
				return errors.New("injected crash after original table drop")
			}
			return nil
		},
	}
	preparedStorageDriver(t, db, driver)
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err == nil {
		t.Fatal("injected rebuild crash was accepted")
	}
	afterShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !shapesEqual(afterShape, beforeShape) || classifyInvocationRuntimeBindingsShape(afterShape) != shapeHistoricalHandler {
		t.Fatalf("crash-aborted rebuild changed the original schema: before=%+v after=%+v", beforeShape, afterShape)
	}
	if afterRows := snapshotBindingRows(t, db, true); !reflect.DeepEqual(afterRows, beforeRows) {
		t.Fatalf("crash-aborted rebuild changed retained bytes: before=%#v after=%#v", beforeRows, afterRows)
	}
	var stagingCount int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='invocation_runtime_bindings_storage_schema_rebuild'`).Scan(&stagingCount); err != nil {
		t.Fatal(err)
	}
	if stagingCount != 0 {
		t.Fatal("crash-aborted rebuild left a staging table behind")
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if history[len(history)-1].State != contracts.LifecycleApplying {
		t.Fatalf("crash-aborted rebuild appended an outcome outside the rolled-back transaction: %+v", history)
	}
	driver.rebuildCheckpoint = nil
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err != nil {
		t.Fatalf("restart after rebuild crash: %v", err)
	}
}

func TestStorageSchemaDriverRollsBackEveryApplyBoundary(t *testing.T) {
	for _, stage := range []string{"before-transaction", "after-guard", "after-predicate-revalidation", "after-mutation", "after-result-check", "after-journal-append"} {
		t.Run(stage, func(t *testing.T) {
			fixtures := []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true, executableBinding: []byte{0x00, 0xff}}}
			db := newHistoricalFixtureDB(t, fixtures)
			ctx := context.Background()
			beforeShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			beforeRows := snapshotBindingRows(t, db, true)
			step := storageSchemaStep(t, canonicalDigest(t))
			plan := storageSchemaPlan(t, step)
			journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
			if err != nil {
				t.Fatal(err)
			}
			driver := &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, applyCheckpoint: func(got string) error {
				if got == stage {
					return errors.New("injected crash at " + stage)
				}
				return nil
			}}
			preparedStorageDriver(t, db, driver)
			prepared := driver.Prepared
			path := sqliteFixturePath(t, db)
			if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err == nil {
				t.Fatal("injected Apply crash was accepted")
			}
			afterShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			if !shapesEqual(beforeShape, afterShape) || !reflect.DeepEqual(beforeRows, snapshotBindingRows(t, db, true)) {
				t.Fatalf("crash at %s left a schema or row mutation", stage)
			}
			history := mustLoadHistory(t, journal)
			if history[len(history)-1].State != contracts.LifecycleApplying {
				t.Fatalf("crash at %s journal = %+v", stage, history)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = state.OpenSQLite(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			journal, err = NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
			if err != nil {
				t.Fatal(err)
			}
			driver = &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: prepared}
			if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err != nil {
				t.Fatalf("restart from Applying after %s: %v", stage, err)
			}
			if got := mustLoadHistory(t, journal); got[len(got)-1].State != contracts.LifecycleCommitted {
				t.Fatalf("restart after %s did not commit: %+v", stage, got)
			}
		})
	}
}

func TestStorageSchemaReversibleStrategyReconstructsExactHistoricalTable(t *testing.T) {
	fixtures := []bindingRowFixture{
		{entryPointID: "ep-a", packageID: "pkg-a", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true, executableBinding: []byte(`{"argv":["a"]}`)},
		{entryPointID: "ep-old", packageID: "pkg-a", packageVersion: "0", contentDigest: "sha256:" + repeatHex("b"), active: false, executableBinding: nil},
	}
	db := newHistoricalFixtureDB(t, fixtures)
	ctx := context.Background()
	beforeRows := snapshotBindingRows(t, db, true)
	step := storageSchemaStep(t, canonicalDigest(t))
	if !step.Reversible || step.Effect != contracts.LifecycleAuthorityBound {
		t.Fatalf("storage_schema strategy declaration is not authority-bound and reversible: %+v", step)
	}
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	if err := RunStep(ctx, journal, storageSchemaRequest(t, plan, step, driver), driver); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := rebuildInvocationRuntimeBindingsToHistorical(ctx, tx); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !shapesEqual(shape, HistoricalInvocationRuntimeBindingsShape()) {
		t.Fatalf("rollback strategy did not reconstruct exact historical schema: %+v", shape)
	}
	if afterRows := snapshotBindingRows(t, db, true); !reflect.DeepEqual(afterRows, beforeRows) {
		t.Fatalf("rollback strategy did not reconstruct exact retained bytes: before=%#v after=%#v", beforeRows, afterRows)
	}
	assertForeignKeyCheckClean(t, db)
}

func TestStorageSchemaDriverStaleTargetDigestReconcilesInsteadOfCommitting(t *testing.T) {
	rows := []bindingRowFixture{{entryPointID: "ep-a", packageID: "pkg-a", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true}}
	db := newHistoricalFixtureDB(t, rows)
	ctx := context.Background()

	wrongTarget := migrationDigest("9") // deliberately not the canonical digest
	step := storageSchemaStep(t, wrongTarget)
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	req := storageSchemaRequest(t, plan, step, driver)
	if err := RunStep(ctx, journal, req, driver); err != nil {
		t.Fatal(err)
	}

	// The rebuild itself still committed (the resulting live shape is
	// genuinely canonical) — only the STEP's outcome is fenced, because
	// what actually resulted does not match what this specific plan
	// declared as its target.
	shape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if classifyInvocationRuntimeBindingsShape(shape) != shapeCanonicalRuntime {
		t.Fatal("expected the mutation to have actually produced the canonical shape")
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertReconcileEntriesValidate(t, history)
	last := history[len(history)-1]
	if last.State != contracts.LifecycleReconcileRequired || last.RecoveryAction != "resulting-manifest-mismatch" {
		t.Fatalf("expected reconcile_required for a stale/substituted target digest, got %+v", last)
	}
}

func TestStorageSchemaDriverResumesFromApplyingWithoutDuplicateOutcome(t *testing.T) {
	rows := []bindingRowFixture{{entryPointID: "ep-a", packageID: "pkg-a", packageVersion: "1", contentDigest: "sha256:" + repeatHex("a"), active: true}}
	db := newHistoricalFixtureDB(t, rows)
	ctx := context.Background()

	target := canonicalDigest(t)
	step := storageSchemaStep(t, target)
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	req := storageSchemaRequest(t, plan, step, driver)
	seedThroughApplying(t, journal, req, step)
	if err := RunStep(ctx, journal, req, driver); err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 {
		t.Fatalf("resume from Applying must not append a second Applying entry; want 5 total entries, got %d: %+v", len(history), history)
	}
	last := history[len(history)-1]
	if last.State != contracts.LifecycleCommitted {
		t.Fatalf("expected resumed Apply to reach committed: %+v", last)
	}
}

func TestStorageSchemaDriverApplyingRestartFencesStalePlanBeforeMutation(t *testing.T) {
	db := newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: migrationDigest("a"), active: true}})
	ctx := context.Background()
	step := storageSchemaStep(t, canonicalDigest(t))
	planA := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), planA.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	reqA := storageSchemaRequest(t, planA, step, driver)
	seedThroughApplying(t, journal, reqA, step)
	beforeShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	beforeRows := snapshotBindingRows(t, db, true)
	planB := planA
	planB.Steps = append([]contracts.LifecycleTransitionStep(nil), planA.Steps...)
	planB.Steps[0].RecoveryStrategy = "substituted-recovery-strategy"
	planB.Digest = ""
	planB.Digest, err = planB.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunStep(ctx, journal, storageSchemaRequest(t, planB, planB.Steps[0], driver), driver); err != nil {
		t.Fatal(err)
	}
	afterShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !shapesEqual(beforeShape, afterShape) || !reflect.DeepEqual(beforeRows, snapshotBindingRows(t, db, true)) {
		t.Fatal("stale plan reached the production storage mutation")
	}
	history := mustLoadHistory(t, journal)
	if history[len(history)-1].State != contracts.LifecycleReconcileRequired || history[len(history)-1].RecoveryAction != "plan-digest-mismatch" {
		t.Fatalf("stale Applying restart was not durably fenced: %+v", history)
	}
}

func TestStorageSchemaDriverApplyingRestartRequiresProductionDriverIdempotence(t *testing.T) {
	db := newHistoricalFixtureDB(t, []bindingRowFixture{{entryPointID: "ep", packageID: "pkg", packageVersion: "1", contentDigest: migrationDigest("a"), active: true}})
	ctx := context.Background()
	step := storageSchemaStep(t, canonicalDigest(t))
	plan := storageSchemaPlan(t, step)
	journal, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	inner := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	req := storageSchemaRequest(t, plan, step, inner)
	seedThroughApplying(t, journal, req, step)
	beforeShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	beforeRows := snapshotBindingRows(t, db, true)
	driver := &nonIdempotentStorageSchemaDriver{inner: inner}
	if err := RunStep(ctx, journal, req, driver); !errors.Is(err, ErrRetryNotSafe) {
		t.Fatalf("production storage restart ignored idempotence refusal: %v", err)
	}
	afterShape, err := IntrospectInvocationRuntimeBindingsShape(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if !shapesEqual(beforeShape, afterShape) || !reflect.DeepEqual(beforeRows, snapshotBindingRows(t, db, true)) {
		t.Fatal("non-idempotent restart reached the production storage mutation")
	}
}

func repeatHex(seed string) string {
	out := ""
	for len(out) < 64 {
		out += seed
	}
	return out[:64]
}
