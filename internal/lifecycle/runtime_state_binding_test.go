package lifecycle

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type runtimeFixture struct {
	entry, packageID, version, digest    string
	active, provenance, existing, plugin bool
	runtimeID                            string
}

func seedRuntimeFixture(t *testing.T, db *sql.DB, f runtimeFixture) {
	t.Helper()
	contract := contracts.InvocationContract{Version: contracts.InvocationContractCurrentVersion(), PackageID: f.packageID, PackageVersion: f.version, GraphID: "graph:" + f.entry, GraphVersion: "1", EntryPointID: f.entry, Aliases: []string{f.entry}}
	contractJSON, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: f.packageID, Version: f.version, ContentDigest: f.digest, Invocations: []contracts.InvocationContract{contract}}
	if f.plugin {
		manifest.ExecutableBindings = []contracts.ExecutableBinding{{EntryPointID: f.entry, PluginID: "plugin", PluginVersion: "1", PluginDefinitionDigest: migrationDigest("1"), ExecutableContentID: "exe", ExecutableContentVersion: "1", ExecutableDigest: migrationDigest("2"), RuntimeID: "plugin-runtime", RuntimeVersion: "1", RuntimeDigest: migrationDigest("3"), ProtocolMin: "1", ProtocolMax: "1"}}
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2026, 9, 17, 14, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	stateName := "installed"
	var activatedAt any
	if f.active {
		stateName = "active"
	}
	if f.provenance {
		activatedAt = stamp
	}
	if _, err := db.Exec(`INSERT INTO installed_packages(package_id,package_version,content_digest,state,source_kind,source_ref,manifest_json,installed_at,activated_at) VALUES(?,?,?,?,?,?,?,?,?)`, f.packageID, f.version, f.digest, stateName, "fixture", "fixture", manifestJSON, stamp, activatedAt); err != nil {
		t.Fatal(err)
	}
	active := 0
	if f.active {
		active = 1
	}
	if _, err := db.Exec(`INSERT INTO invocation_registry(entry_point_id,package_id,package_version,content_digest,graph_id,graph_version,contract_json,contract_digest,active,registered_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, f.entry, f.packageID, f.version, f.digest, contract.GraphID, contract.GraphVersion, contractJSON, state.DigestPackageBytes(contractJSON), active, stamp); err != nil {
		t.Fatal(err)
	}
	if f.existing {
		runtimeID := f.runtimeID
		if runtimeID == "" {
			runtimeID = "stale:" + f.entry
		}
		if _, err := db.Exec(`INSERT INTO invocation_runtime_bindings(entry_point_id,package_version,content_digest,package_id,contract_digest,runtime_id,runtime_version,runtime_digest,registered_at,executable_binding_json) VALUES(?,?,?,?,?,?,?,?,?,?)`, f.entry, f.version, f.digest, f.packageID, state.DigestPackageBytes(contractJSON), runtimeID, "stale", migrationDigest("f"), stamp, []byte(`{"stale":true}`)); err != nil {
			t.Fatal(err)
		}
	}
}

func newRuntimeFixtureDB(t *testing.T, fixtures ...runtimeFixture) *sql.DB {
	t.Helper()
	db, err := state.OpenSQLite(context.Background(), filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, fixture := range fixtures {
		seedRuntimeFixture(t, db, fixture)
	}
	return db
}

func preparedRuntimePlan(t *testing.T, db *sql.DB) (RuntimeStatePreparation, contracts.LifecyclePlan) {
	t.Helper()
	prepared, err := PrepareRuntimeState(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	storageStep := storageSchemaStep(t, canonicalDigest(t))
	storageStep.Current.Digest = canonicalDigest(t)
	runtimeStep := runtimeStateStep(t)
	runtimeStep.Target.Digest = prepared.ProposedResultDigest
	plan, err := NewRuntimeRecoveryPlan(RuntimeRecoveryPlanSpec{PlanID: "runtime-state-plan", PlanVersion: "1", InstallationID: "installation", CurrentManifestDigest: storageStep.Current.Digest, TargetManifestDigest: runtimeStep.Target.Digest, StorageSchemaStep: storageStep, RuntimeStateStep: runtimeStep, PreservedHistory: []contracts.LifecycleEvidenceRef{{ID: "history", Kind: "fixture", Source: "fixture", Digest: migrationDigest("9")}}, ExpectedReadiness: migrationReadiness()})
	if err != nil {
		t.Fatal(err)
	}
	return prepared, plan
}

func newRuntimeJournal(t *testing.T, db *sql.DB, plan contracts.LifecyclePlan) *Journal {
	t.Helper()
	j, err := NewJournal(state.NewSQLiteEventStore(db), plan.InstallationID, contracts.PrincipalRef{ID: "lifecycle", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func qualifyStorage(t *testing.T, db *sql.DB, journal *Journal, plan contracts.LifecyclePlan) {
	t.Helper()
	driver := preparedStorageDriver(t, db, &StorageSchemaDriver{DB: db, Journal: journal, Guard: &allowingGuard{}})
	if err := RunStep(context.Background(), journal, storageSchemaRequest(t, plan, plan.Steps[0], driver), driver); err != nil {
		t.Fatal(err)
	}
}

func runtimeRequestForPreparation(t *testing.T, plan contracts.LifecyclePlan, prepared RuntimeStatePreparation) RunRequest {
	t.Helper()
	digest, err := prepared.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return RunRequest{Plan: plan, StepID: plan.Steps[1].ID, PreconditionDigest: digest, SnapshotDigest: migrationDigest("b"), Authority: migrationAuthority{decision: runtimeStateAuthorityDecision(t)}, Now: time.Now().UTC()}
}

func runRuntime(t *testing.T, db *sql.DB, journal *Journal, plan contracts.LifecyclePlan, prepared RuntimeStatePreparation, guard AuthorityGuard, checkpoint func(string) error) error {
	t.Helper()
	driver := &RuntimeStateDriver{DB: db, Journal: journal, Guard: guard, Prepared: prepared, mutationCheckpoint: checkpoint}
	return RunStep(context.Background(), journal, runtimeRequestForPreparation(t, plan, prepared), driver)
}

type resultCapturingRuntimeStateDriver struct {
	inner  *RuntimeStateDriver
	result ApplyResult
}

type nonIdempotentRuntimeStateDriver struct{ inner *RuntimeStateDriver }

func (d *nonIdempotentRuntimeStateDriver) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	return d.inner.BindRunRequest(ctx, req, step, authority)
}
func (d *nonIdempotentRuntimeStateDriver) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	return d.inner.Preflight(ctx, step)
}
func (*nonIdempotentRuntimeStateDriver) Idempotent(contracts.LifecycleTransitionStep) bool {
	return false
}
func (d *nonIdempotentRuntimeStateDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	return d.inner.Apply(ctx, step)
}

func seedRuntimeThroughApplying(t *testing.T, journal *Journal, req RunRequest, step contracts.LifecycleTransitionStep) {
	t.Helper()
	history := mustLoadHistory(t, journal)
	sequence := len(history) + 1
	var previous contracts.LifecycleTransitionState
	for _, next := range []contracts.LifecycleTransitionState{contracts.LifecyclePlanned, contracts.LifecycleApproved, contracts.LifecyclePrepared, contracts.LifecycleApplying} {
		if err := journal.Append(context.Background(), journalEntry(req, step, sequence, next, previous, "", nil)); err != nil {
			t.Fatalf("seed runtime %s: %v", next, err)
		}
		sequence++
		previous = next
	}
}

func (d *resultCapturingRuntimeStateDriver) BindRunRequest(ctx context.Context, req RunRequest, step contracts.LifecycleTransitionStep, authority *contracts.AuthorityDecision) error {
	return d.inner.BindRunRequest(ctx, req, step, authority)
}
func (d *resultCapturingRuntimeStateDriver) Preflight(ctx context.Context, step contracts.LifecycleTransitionStep) error {
	return d.inner.Preflight(ctx, step)
}
func (d *resultCapturingRuntimeStateDriver) Idempotent(step contracts.LifecycleTransitionStep) bool {
	return d.inner.Idempotent(step)
}
func (d *resultCapturingRuntimeStateDriver) Apply(ctx context.Context, step contracts.LifecycleTransitionStep) (ApplyResult, error) {
	result, err := d.inner.Apply(ctx, step)
	d.result = result
	return result, err
}

func bindingBytes(t *testing.T, db *sql.DB) [][]byte {
	t.Helper()
	rows, err := db.Query(`SELECT entry_point_id||char(0)||package_version||char(0)||content_digest||char(0)||package_id||char(0)||contract_digest||char(0)||runtime_id||char(0)||runtime_version||char(0)||runtime_digest||char(0)||registered_at||char(0)||coalesce(hex(executable_binding_json),'NULL') FROM invocation_runtime_bindings ORDER BY entry_point_id,package_version,content_digest`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			t.Fatal(err)
		}
		out = append(out, append([]byte(nil), b...))
	}
	return out
}

func bindingBytesForEntry(t *testing.T, db *sql.DB, entryPointID string) []byte {
	t.Helper()
	var out []byte
	if err := db.QueryRow(`SELECT entry_point_id||char(0)||package_version||char(0)||content_digest||char(0)||package_id||char(0)||contract_digest||char(0)||runtime_id||char(0)||runtime_version||char(0)||runtime_digest||char(0)||registered_at||char(0)||coalesce(hex(executable_binding_json),'NULL') FROM invocation_runtime_bindings WHERE entry_point_id=?`, entryPointID).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), out...)
}

func bindingBytesForGeneration(t *testing.T, db *sql.DB, entryPointID, packageVersion string) []byte {
	t.Helper()
	var out []byte
	if err := db.QueryRow(`SELECT entry_point_id||char(0)||package_version||char(0)||content_digest||char(0)||package_id||char(0)||contract_digest||char(0)||runtime_id||char(0)||runtime_version||char(0)||runtime_digest||char(0)||registered_at||char(0)||coalesce(hex(executable_binding_json),'NULL') FROM invocation_runtime_bindings WHERE entry_point_id=? AND package_version=?`, entryPointID, packageVersion).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), out...)
}

func TestRuntimeStatePreparationBindsAllThreeIdentities(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true})
	prepared, plan := preparedRuntimePlan(t, db)
	for name, digest := range map[string]string{"population": prepared.PopulationDigest, "evidence": prepared.ReconstructionEvidenceDigest, "proposed result": prepared.ProposedResultDigest} {
		if err := contracts.ValidateSHA256Digest(digest); err != nil {
			t.Fatalf("%s digest: %v", name, err)
		}
	}
	if plan.Steps[1].Target.Digest != prepared.ProposedResultDigest {
		t.Fatal("runtime target is not prepared proposed-result identity")
	}
	if got, _ := prepared.Digest(); runtimeRequestForPreparation(t, plan, prepared).PreconditionDigest != got {
		t.Fatal("RunRequest does not bind preparation bundle")
	}
}

func TestRuntimeStatePreparationFailsClosedOnInvalidEvidence(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true})
	if _, err := PrepareRuntimeState(context.Background(), db); !errors.Is(err, ErrRuntimeStateEvidence) {
		t.Fatalf("PrepareRuntimeState error = %v, want %v", err, ErrRuntimeStateEvidence)
	}
}

func TestRuntimeStateClassificationUsesDurableManifestWithoutTrustRoots(t *testing.T) {
	t.Setenv("PRAXIS_TRUSTED_KEYS", "")
	t.Run("plugin despite null target-side executable binding", func(t *testing.T) {
		db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true, plugin: true})
		if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET executable_binding_json=NULL WHERE entry_point_id='ep'`); err != nil {
			t.Fatal(err)
		}
		if _, err := PrepareRuntimeState(context.Background(), db); !errors.Is(err, ErrRuntimeStatePluginPending) {
			t.Fatalf("manifest-backed plugin classification error = %v, want %v", err, ErrRuntimeStatePluginPending)
		}
	})
	t.Run("non-plugin without signature or trust configuration", func(t *testing.T) {
		db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
		if _, err := PrepareRuntimeState(context.Background(), db); err != nil {
			t.Fatalf("manifest-backed non-plugin classification unexpectedly required trust verification: %v", err)
		}
	})
}

func TestRuntimeStatePreparationRejectsContractIdentityMismatch(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true})
	var body []byte
	if err := db.QueryRow(`SELECT contract_json FROM invocation_registry WHERE entry_point_id='ep'`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var contract contracts.InvocationContract
	if err := json.Unmarshal(body, &contract); err != nil {
		t.Fatal(err)
	}
	contract.PackageID = "substituted-package"
	body, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE invocation_registry SET contract_json=?,contract_digest=? WHERE entry_point_id='ep'`, body, state.DigestPackageBytes(body)); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRuntimeState(context.Background(), db); !errors.Is(err, ErrRuntimeStateEvidence) {
		t.Fatalf("contract/source identity mismatch was accepted: %v", err)
	}
}

func TestRuntimeStatePreparationRejectsTargetRowPackageSubstitution(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET package_id='substituted-package' WHERE entry_point_id='ep'`); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareRuntimeState(context.Background(), db); !errors.Is(err, ErrRuntimeStateEvidence) {
		t.Fatalf("target-row package substitution was accepted: %v", err)
	}
}

func TestRuntimeStateDerivesFromOldestHistoricalInvocationWireShape(t *testing.T) {
	// Before commit 788f7a1, InvocationContract had no JSON tags, so the
	// durable encoding used exported Go field names. Version "1" is the
	// oldest schema version the current compatibility catalog supports.
	const historical = `{"Version":"1","PackageID":"pkg","PackageVersion":"1","GraphID":"graph:historical","GraphVersion":"1","EntryPointID":"ep","Aliases":["historical"],"Options":null,"RequiredCapabilities":null,"OptionalCapabilities":null,"RequiredEnforcement":null,"RequireExclusiveMediation":false}`
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	if _, err := db.Exec(`UPDATE invocation_registry SET graph_id='graph:historical',contract_json=?,contract_digest=? WHERE entry_point_id='ep'`, []byte(historical), state.DigestPackageBytes([]byte(historical))); err != nil {
		t.Fatal(err)
	}
	derived, err := deriveRuntimeState(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if len(derived.Proposals) != 1 {
		t.Fatalf("historical derivation proposals = %d", len(derived.Proposals))
	}
	proposal := derived.Proposals[0]
	// These expected values are fixed from the historical bytes above, not
	// produced by a second call through the derivation under test.
	const expectedDigest = "sha256:a8f9186435696c659becfb8de746a6503801a1b7206adb6bbc456b8b245e25f5"
	if proposal.RuntimeID != "client-adapter:pkg:ep" || proposal.RuntimeVersion != "1" || proposal.RuntimeDigest != expectedDigest {
		t.Fatalf("historical contract derivation = %+v", proposal)
	}
}

func TestRuntimeStateRejectsPreparedDriftWithoutMutation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drift func(*testing.T, *sql.DB, RuntimeStatePreparation)
	}{
		{"population", func(t *testing.T, db *sql.DB, _ RuntimeStatePreparation) {
			seedRuntimeFixture(t, db, runtimeFixture{entry: "new", packageID: "newpkg", version: "1", digest: migrationDigest("c"), active: true, provenance: true})
		}},
		{"evidence", func(t *testing.T, db *sql.DB, _ RuntimeStatePreparation) {
			var body []byte
			if err := db.QueryRow(`SELECT manifest_json FROM installed_packages WHERE package_id='pkg'`).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var raw any
			if err := json.Unmarshal(body, &raw); err != nil {
				t.Fatal(err)
			}
			pretty, _ := json.MarshalIndent(raw, "", "  ")
			if _, err := db.Exec(`UPDATE installed_packages SET manifest_json=? WHERE package_id='pkg'`, pretty); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
			prepared, plan := preparedRuntimePlan(t, db)
			journal := newRuntimeJournal(t, db, plan)
			qualifyStorage(t, db, journal, plan)
			before := bindingBytes(t, db)
			tc.drift(t, db, prepared)
			if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
				t.Fatal(err)
			}
			if after := bindingBytes(t, db); !reflect.DeepEqual(after, before) {
				t.Fatalf("%s drift caused mutation", tc.name)
			}
			stateNow, _, _ := lastStepState(mustLoadHistory(t, journal), plan.PlanID, plan.Steps[1].ID)
			if stateNow != contracts.LifecycleFailedRecoverable {
				t.Fatalf("got %s", stateNow)
			}
		})
	}
}

func TestRuntimeStateTransactionalGuardObservesGenerationInvalidationAfterPrepared(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	ctx := context.Background()
	prepared, plan := preparedRuntimePlan(t, db)
	now := time.Now().UTC()
	service := praxiscrypto.EnvelopeService{Wrapper: lifecycleAuthorityTestWrapper{}}
	store := state.New(db)
	decision := runtimeStateAuthorityDecision(t)
	generation := persistLifecycleRepairRoot(t, db, service, now)
	var err error
	expires := now.Add(time.Hour)
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
	guard.Now = func() time.Time { return now }
	decisionDigest, err := decision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	plan.Steps[1].Authority.DecisionRef, plan.Steps[1].Authority.DecisionVersion, plan.Steps[1].Authority.DecisionDigest = decision.DecisionRef, decision.DecisionVersion, decisionDigest
	plan.Steps[1].Authority.AuthorityRef, plan.Steps[1].Authority.AuthorityVersion, plan.Steps[1].Authority.AuthorityGenerationDigest = generation.Ref, generation.Version, generation.Digest
	plan.Digest = ""
	plan.Digest, err = plan.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	before := bindingBytes(t, db)
	invalidation := sealedLifecycleAuthorityRecord(t, service, authorityGenerationInvalidationNamespace, generation.Ref, generation.Version, migrationDigest("7"), now)
	if err := store.PutSecureBlob(ctx, invalidation); err != nil {
		t.Fatal(err)
	}
	precondition, err := prepared.Digest()
	if err != nil {
		t.Fatal(err)
	}
	req := RunRequest{Plan: plan, StepID: plan.Steps[1].ID, PreconditionDigest: precondition, SnapshotDigest: migrationDigest("b"), Authority: migrationAuthority{decision: decision}, Now: now}
	driver := &RuntimeStateDriver{DB: db, Journal: journal, Guard: guard, Prepared: prepared}
	if err := RunStep(ctx, journal, req, driver); err == nil {
		t.Fatal("runtime_state accepted a generation invalidated after preparation")
	}
	if !reflect.DeepEqual(before, bindingBytes(t, db)) {
		t.Fatal("runtime authority refusal occurred after mutation")
	}
	history := mustLoadHistory(t, journal)
	if history[len(history)-1].State != contracts.LifecycleApplying {
		t.Fatalf("runtime authority refusal appended an outcome: %+v", history)
	}
}

func TestRuntimeStateRejectsProposedResultMismatch(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	prepared, _ := preparedRuntimePlan(t, db)
	prepared.ProposedResultDigest = migrationDigest("d")
	storageStep := storageSchemaStep(t, canonicalDigest(t))
	storageStep.Current.Digest = canonicalDigest(t)
	runtimeStep := runtimeStateStep(t)
	runtimeStep.Target.Digest = prepared.ProposedResultDigest
	plan, err := NewRuntimeRecoveryPlan(RuntimeRecoveryPlanSpec{PlanID: "runtime-state-plan", PlanVersion: "1", InstallationID: "installation", CurrentManifestDigest: storageStep.Current.Digest, TargetManifestDigest: runtimeStep.Target.Digest, StorageSchemaStep: storageStep, RuntimeStateStep: runtimeStep, ExpectedReadiness: migrationReadiness()})
	if err != nil {
		t.Fatal(err)
	}
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	before := bindingBytes(t, db)
	if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, bindingBytes(t, db)) {
		t.Fatal("proposed-result mismatch mutated bindings")
	}
}

func TestRuntimeStateDriverApplyingRestartFencesStalePlanBeforeMutation(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	ctx := context.Background()
	prepared, planA := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, planA)
	qualifyStorage(t, db, journal, planA)
	driver := &RuntimeStateDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: prepared}
	reqA := runtimeRequestForPreparation(t, planA, prepared)
	seedRuntimeThroughApplying(t, journal, reqA, planA.Steps[1])
	before := bindingBytes(t, db)
	beforeHistory := len(mustLoadHistory(t, journal))
	planB := planA
	planB.Steps = append([]contracts.LifecycleTransitionStep(nil), planA.Steps...)
	planB.Steps[1].RecoveryStrategy = "substituted-recovery-strategy"
	planB.Digest = ""
	var err error
	planB.Digest, err = planB.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := RunStep(ctx, journal, runtimeRequestForPreparation(t, planB, prepared), driver); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, bindingBytes(t, db)) {
		t.Fatal("stale plan reached the production runtime mutation")
	}
	history := mustLoadHistory(t, journal)
	if len(history) != beforeHistory+1 || history[len(history)-1].State != contracts.LifecycleReconcileRequired || history[len(history)-1].RecoveryAction != "plan-digest-mismatch" {
		t.Fatalf("stale runtime Applying restart was not fenced exactly once: %+v", history)
	}
}

func TestRuntimeStateDriverApplyingRestartRequiresProductionDriverIdempotence(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	ctx := context.Background()
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	inner := &RuntimeStateDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: prepared}
	req := runtimeRequestForPreparation(t, plan, prepared)
	seedRuntimeThroughApplying(t, journal, req, plan.Steps[1])
	before := bindingBytes(t, db)
	beforeHistory := len(mustLoadHistory(t, journal))
	driver := &nonIdempotentRuntimeStateDriver{inner: inner}
	if err := RunStep(ctx, journal, req, driver); !errors.Is(err, ErrRetryNotSafe) {
		t.Fatalf("production runtime restart ignored idempotence refusal: %v", err)
	}
	if !reflect.DeepEqual(before, bindingBytes(t, db)) {
		t.Fatal("non-idempotent restart reached the production runtime mutation")
	}
	if history := mustLoadHistory(t, journal); len(history) != beforeHistory {
		t.Fatalf("idempotence refusal appended an outcome: %+v", history)
	}
}

func TestRuntimeStateRejectsPreparedTargetBindingDriftWithoutOverwrite(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET runtime_id='concurrent-writer' WHERE entry_point_id='ep'`); err != nil {
		t.Fatal(err)
	}
	drifted := bindingBytes(t, db)
	if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(drifted, bindingBytes(t, db)) {
		t.Fatal("runtime target pre-state drift was overwritten")
	}
	history := mustLoadHistory(t, journal)
	if history[len(history)-1].State != contracts.LifecycleFailedRecoverable {
		t.Fatalf("runtime target pre-state drift was not fenced recoverably: %+v", history)
	}
}

func TestRuntimeStateMixedPluginPopulationFailsClosed(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ok", packageID: "okpkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	before := bindingBytes(t, db)
	seedRuntimeFixture(t, db, runtimeFixture{entry: "plugin", packageID: "pluginpkg", version: "1", digest: migrationDigest("b"), active: true, provenance: true, plugin: true})
	if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, bindingBytes(t, db)) {
		t.Fatal("mixed plugin population partially mutated supported row")
	}
	last := mustLoadHistory(t, journal)[len(mustLoadHistory(t, journal))-1]
	if last.RecoveryAction != "plugin-verification-failed-pending-140" {
		t.Fatalf("plugin refusal did not cite issue #140: %+v", last)
	}
}

func TestRuntimeStateAmbiguousManifestClassificationReconcilesWithoutMutation(t *testing.T) {
	db := newRuntimeFixtureDB(t, runtimeFixture{entry: "ep", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true})
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	before := bindingBytes(t, db)
	if _, err := db.Exec(`UPDATE installed_packages SET manifest_json=? WHERE package_id='pkg'`, []byte(`{"not":"a manifest"}`)); err != nil {
		t.Fatal(err)
	}
	if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, bindingBytes(t, db)) {
		t.Fatal("ambiguous manifest classification mutated runtime bindings")
	}
	history := mustLoadHistory(t, journal)
	assertReconcileEntriesValidate(t, history)
	last := history[len(history)-1]
	if last.State != contracts.LifecycleReconcileRequired || last.RecoveryAction != "plugin-classification-ambiguous-pending-140" {
		t.Fatalf("ambiguous plugin classification routing = %+v", last)
	}
}

func TestRuntimeStateSuccessPreservesInactiveAndUsesObservedResult(t *testing.T) {
	db := newRuntimeFixtureDB(t,
		runtimeFixture{entry: "existing", packageID: "pkg-a", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true},
		runtimeFixture{entry: "missing", packageID: "pkg-b", version: "1", digest: migrationDigest("b"), active: true, provenance: true},
		runtimeFixture{entry: "inactive", packageID: "pkg-c", version: "0", digest: migrationDigest("c"), active: false, provenance: true, existing: true, runtimeID: "historical-byte-identity"},
	)
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	inactiveBefore := bindingBytesForEntry(t, db, "inactive")
	inner := &RuntimeStateDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: prepared}
	capturing := &resultCapturingRuntimeStateDriver{inner: inner}
	if err := RunStep(context.Background(), journal, runtimeRequestForPreparation(t, plan, prepared), capturing); err != nil {
		t.Fatal(err)
	}
	inactiveAfter := bindingBytesForEntry(t, db, "inactive")
	if !reflect.DeepEqual(inactiveBefore, inactiveAfter) {
		t.Fatal("inactive retained generation changed")
	}
	observed, err := observeActiveRuntimeResultDigest(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if observed != prepared.ProposedResultDigest || observed != plan.Steps[1].Target.Digest {
		t.Fatalf("observed result %s not prepared/target", observed)
	}
	if capturing.result.ResultingManifestDigest != observed || capturing.result.Outcome != ApplyCommitted || !capturing.result.OutcomeAlreadyRecorded {
		t.Fatalf("Apply did not return the post-write observed identity: %+v observed=%s", capturing.result, observed)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM invocation_runtime_bindings WHERE entry_point_id='missing'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("missing active binding was not inserted")
	}
}

func TestRuntimeStateDriverJournalUsesOnlyEventInserts(t *testing.T) {
	db, writeLog, _ := openWriteLoggedLifecycleDB(t)
	seedRuntimeFixture(t, db, runtimeFixture{entry: "active", packageID: "pkg", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true, runtimeID: "stale"})
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	writeLog.reset()
	qualifyStorage(t, db, journal, plan)
	if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
		t.Fatal(err)
	}
	writeLog.assertEventsAppendOnly(t, 10)
}

func TestRuntimeStateReactivationUpdatesRetainedGenerationByHistoricalKey(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := state.New(db)
	now := time.Date(2026, 9, 17, 15, 0, 0, 0, time.UTC)
	activateRuntimeRollbackGeneration(t, ctx, db, store, "1", now)
	activateRuntimeRollbackGeneration(t, ctx, db, store, "2", now.Add(time.Minute))
	retainedBefore := bindingBytesForGeneration(t, db, "ep", "1")
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET runtime_id='successor-stale',runtime_version='stale',runtime_digest=? WHERE package_id='pkg' AND package_version='2'`, migrationDigest("f")); err != nil {
		t.Fatal(err)
	}
	prepared, plan := preparedRuntimePlan(t, db)
	journal := newRuntimeJournal(t, db, plan)
	qualifyStorage(t, db, journal, plan)
	if err := runRuntime(t, db, journal, plan, prepared, &allowingGuard{}, nil); err != nil {
		t.Fatal(err)
	}

	actor := contracts.PrincipalRef{ID: "rollback-owner", Kind: "user"}
	request, err := store.PreparePackageRollback(ctx, "pkg", "approval:rollback", actor)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, request.ApprovalID, actor.ID, actor.Kind, intentDigest, now.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := store.RollbackPackage(ctx, request, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	active, err := store.ActivePackage(ctx, "pkg")
	if err != nil || active.Manifest.Version != "1" {
		t.Fatalf("real rollback path did not reactivate retained generation: active=%+v err=%v", active, err)
	}
	resolved, err := store.ResolveInvocationAlias(ctx, "ep")
	if err != nil || resolved.Contract.PackageVersion != "1" {
		t.Fatalf("real rollback path did not restore retained invocation: resolved=%+v err=%v", resolved, err)
	}
	retainedAfter := bindingBytesForGeneration(t, db, "ep", "1")
	if !reflect.DeepEqual(retainedBefore, retainedAfter) {
		t.Fatal("runtime recovery changed the inactive binding needed by real rollback/reactivation")
	}
}

func activateRuntimeRollbackGeneration(t *testing.T, ctx context.Context, db *sql.DB, store *state.Store, version string, now time.Time) {
	t.Helper()
	artifact := []byte("runtime-rollback-fixture:" + version)
	manifest := packagecatalog.Manifest{
		ContractVersion: packagecatalog.ManifestContractCurrentVersion(),
		PackageID:       "pkg",
		Version:         version,
		ContentDigest:   state.DigestPackageBytes(artifact),
		Invocations: []contracts.InvocationContract{{
			Version: contracts.InvocationContractCurrentVersion(), PackageID: "pkg", PackageVersion: version,
			GraphID: "graph:ep", GraphVersion: version, EntryPointID: "ep", Aliases: []string{"ep"},
		}},
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	envelope := packagecatalog.SignatureEnvelope{
		Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible,
		ManifestDigest: state.DigestPackageBytes(manifestBytes), ArtifactDigest: manifest.ContentDigest,
	}
	proof := packagecatalog.SignatureProof{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "runtime-rollback-fixture"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, envelope.Statement()))
	envelope.Proofs = []packagecatalog.SignatureProof{proof}
	verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{
		ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope,
		SourceKind: "fixture", SourceRef: "pkg@" + version, VerifiedAt: now,
	}, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"runtime-rollback-fixture": publicKey}}})
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "activation-owner", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(verified, actor)
	if err != nil {
		t.Fatal(err)
	}
	intentDigest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	approvalID := "approval:activate:" + version
	if _, err := db.Exec(`INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, intentDigest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := store.ActivatePackage(ctx, packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: approvalID}, now); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeStateAuthorityAndMidApplyFailuresRollback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		guard      AuthorityGuard
		checkpoint func(string) error
	}{
		{"authority", failingGuard{err: errors.New("revoked")}, nil},
		{"mid-apply", &allowingGuard{}, func(string) error { return errors.New("injected write failure") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newRuntimeFixtureDB(t, runtimeFixture{entry: "a", packageID: "pkg-a", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true}, runtimeFixture{entry: "b", packageID: "pkg-b", version: "1", digest: migrationDigest("b"), active: true, provenance: true, existing: true})
			prepared, plan := preparedRuntimePlan(t, db)
			journal := newRuntimeJournal(t, db, plan)
			qualifyStorage(t, db, journal, plan)
			before := bindingBytes(t, db)
			if err := runRuntime(t, db, journal, plan, prepared, tc.guard, tc.checkpoint); err == nil {
				t.Fatal("injected Apply failure was accepted")
			}
			if !reflect.DeepEqual(before, bindingBytes(t, db)) {
				t.Fatalf("%s failure left partial mutation", tc.name)
			}
			history := mustLoadHistory(t, journal)
			if history[len(history)-1].State != contracts.LifecycleApplying {
				t.Fatalf("%s failure appended an outcome outside the rolled-back transaction: %+v", tc.name, history)
			}
		})
	}
}

func TestRuntimeStateRollsBackEveryApplyBoundary(t *testing.T) {
	for _, stage := range []string{"before-transaction", "after-guard", "after-predicate-revalidation", "mid-mutation", "after-mutation", "after-result-check", "after-journal-append"} {
		t.Run(stage, func(t *testing.T) {
			db := newRuntimeFixtureDB(t,
				runtimeFixture{entry: "a", packageID: "pkg-a", version: "1", digest: migrationDigest("a"), active: true, provenance: true, existing: true},
				runtimeFixture{entry: "b", packageID: "pkg-b", version: "1", digest: migrationDigest("b"), active: true, provenance: true, existing: true},
			)
			prepared, plan := preparedRuntimePlan(t, db)
			journal := newRuntimeJournal(t, db, plan)
			qualifyStorage(t, db, journal, plan)
			before := bindingBytes(t, db)
			path := sqliteFixturePath(t, db)
			driver := &RuntimeStateDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: prepared}
			driver.applyCheckpoint = func(got string) error {
				if got == stage {
					return errors.New("injected crash at " + stage)
				}
				return nil
			}
			if stage == "mid-mutation" {
				driver.mutationCheckpoint = func(string) error { return errors.New("injected crash at mid-mutation") }
			}
			if err := RunStep(context.Background(), journal, runtimeRequestForPreparation(t, plan, prepared), driver); err == nil {
				t.Fatal("injected Apply crash was accepted")
			}
			if !reflect.DeepEqual(before, bindingBytes(t, db)) {
				t.Fatalf("crash at %s left a partial runtime mutation", stage)
			}
			history := mustLoadHistory(t, journal)
			if history[len(history)-1].State != contracts.LifecycleApplying {
				t.Fatalf("crash at %s journal = %+v", stage, history)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := state.OpenSQLite(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			journal = newRuntimeJournal(t, db, plan)
			driver = &RuntimeStateDriver{DB: db, Journal: journal, Guard: &allowingGuard{}, Prepared: prepared}
			if err := RunStep(context.Background(), journal, runtimeRequestForPreparation(t, plan, prepared), driver); err != nil {
				t.Fatalf("restart from Applying after %s: %v", stage, err)
			}
			if got := mustLoadHistory(t, journal); got[len(got)-1].State != contracts.LifecycleCommitted {
				t.Fatalf("restart after %s did not commit: %+v", stage, got)
			}
		})
	}
}
