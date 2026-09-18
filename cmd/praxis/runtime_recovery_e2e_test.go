package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/lifecycle"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type recoveryTestWrapper struct{}

func (recoveryTestWrapper) Capabilities(context.Context, string) (praxiscrypto.Capabilities, error) {
	return praxiscrypto.Capabilities{Classical: true}, nil
}
func (recoveryTestWrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, key []byte) (praxiscrypto.WrappedKey, error) {
	return praxiscrypto.WrappedKey{Ciphertext: append([]byte(nil), key...), SuiteID: "fixture", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (recoveryTestWrapper) Unwrap(_ context.Context, wrapped praxiscrypto.WrappedKey) ([]byte, error) {
	return append([]byte(nil), wrapped.Ciphertext...), nil
}

type recoveryFixtureRow struct {
	entryPointID, packageID, packageVersion, contentDigest string
	active                                                 bool
}

func fixtureDigest(fill byte) string { return "sha256:" + strings.Repeat(string(fill), 64) }

func exactContractDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func revertRuntimeBindingsToExact0084352(ctx context.Context, db *sql.DB) error {
	// historical0012 is byte-for-byte commit 0084352's migration body;
	// executable0013 is the exact following migration body.
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
	for _, statement := range []string{`DROP TABLE invocation_runtime_bindings`, historical0012, executable0013} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func seedRecoveryFixture(t *testing.T, db *sql.DB, rows []recoveryFixtureRow, now time.Time) map[string][]byte {
	t.Helper()
	inactive := map[string][]byte{}
	for i, row := range rows {
		stamp := now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		contract := contracts.InvocationContract{Version: contracts.InvocationContractCurrentVersion(), PackageID: row.packageID, PackageVersion: row.packageVersion, GraphID: "graph:" + row.entryPointID, GraphVersion: "1", EntryPointID: row.entryPointID, Aliases: []string{row.entryPointID}}
		contractJSON, err := json.Marshal(contract)
		if err != nil {
			t.Fatal(err)
		}
		contractDigest := exactContractDigest(contractJSON)
		manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: row.packageID, Version: row.packageVersion, ContentDigest: row.contentDigest, Invocations: []contracts.InvocationContract{contract}}
		manifestJSON, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		installedState, active := "installed", 0
		if row.active {
			installedState, active = "active", 1
		}
		if _, err := db.Exec(`INSERT INTO installed_packages(package_id,package_version,content_digest,state,source_kind,source_ref,manifest_json,installed_at,activated_at) VALUES(?,?,?,?,?,?,?,?,?)`, row.packageID, row.packageVersion, row.contentDigest, installedState, "fixture", "0084352", manifestJSON, stamp, stamp); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO invocation_registry(entry_point_id,package_id,package_version,content_digest,graph_id,graph_version,contract_json,contract_digest,active,registered_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, row.entryPointID, row.packageID, row.packageVersion, row.contentDigest, contract.GraphID, contract.GraphVersion, contractJSON, contractDigest, active, stamp); err != nil {
			t.Fatal(err)
		}
		approvalID := fmt.Sprintf("approval:%s:%s", row.packageID, row.packageVersion)
		if _, err := db.Exec(`INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,0)`, approvalID, "fixture-owner", "human", fixtureDigest('d'), stamp); err != nil {
			t.Fatal(err)
		}
		activationID := fmt.Sprintf("activation:%s:%s", row.packageID, row.packageVersion)
		if _, err := db.Exec(`INSERT INTO package_activation_receipts(activation_id,package_id,package_version,content_digest,verification_id,verification_json,manifest_bytes,signature_json,activation_intent_json,activation_intent_digest,approval_id,authority_id,authority_kind,activated_at,artifact_bytes) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, activationID, row.packageID, row.packageVersion, row.contentDigest, "verification:"+activationID, []byte(`{}`), manifestJSON, []byte(`{}`), []byte(`{}`), fixtureDigest('e'), approvalID, "fixture-owner", "human", stamp, []byte("fixture-artifact")); err != nil {
			t.Fatal(err)
		}
		executable := []byte(`{"historical":true}`)
		if _, err := db.Exec(`INSERT INTO invocation_runtime_bindings(entry_point_id,package_version,content_digest,package_id,contract_digest,handler_id,handler_version,handler_digest,registered_at,executable_binding_json) VALUES(?,?,?,?,?,?,?,?,?,?)`, row.entryPointID, row.packageVersion, row.contentDigest, row.packageID, contractDigest, "stale:"+row.entryPointID, "stale", fixtureDigest('f'), stamp, executable); err != nil {
			t.Fatal(err)
		}
		if !row.active {
			inactive[row.entryPointID+"\x00"+row.packageVersion] = bindingGenerationBytes(t, db, row.entryPointID, row.packageVersion, true)
		}
	}
	return inactive
}

func bindingGenerationBytes(t *testing.T, db *sql.DB, entryPointID, version string, historical bool) []byte {
	t.Helper()
	runtimeColumns := "runtime_id,runtime_version,runtime_digest"
	if historical {
		runtimeColumns = "handler_id,handler_version,handler_digest"
	}
	query := `SELECT entry_point_id||char(0)||package_version||char(0)||content_digest||char(0)||package_id||char(0)||contract_digest||char(0)||` + strings.ReplaceAll(runtimeColumns, ",", `||char(0)||`) + `||char(0)||registered_at||char(0)||coalesce(hex(executable_binding_json),'NULL') FROM invocation_runtime_bindings WHERE entry_point_id=? AND package_version=?`
	var out []byte
	if err := db.QueryRow(query, entryPointID, version).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), out...)
}

func saveRecoveryAuthority(t *testing.T, repo goalstore.Repository, generation contracts.AuthorityGeneration, operation, requestID string, now time.Time) contracts.AuthorityDecision {
	t.Helper()
	return saveRecoveryAuthorityUntil(t, repo, generation, operation, requestID, now, now.Add(2*time.Hour))
}

func saveRecoveryAuthorityUntil(t *testing.T, repo goalstore.Repository, generation contracts.AuthorityGeneration, operation, requestID string, now, expires time.Time) contracts.AuthorityDecision {
	t.Helper()
	successionDigest := fixtureDigest('4')
	request := contracts.AuthorityRequest{ID: requestID, Version: "1", RequestedAuthority: operation, RequestedScope: generation.Scope, Reason: "PLAN-016 fixture qualification", Status: contracts.AuthorityRequestPending, InstallationDigest: generation.ProvenanceDigest, Repair: &contracts.InstallationRepairAuthorityRequest{BootstrapDigest: generation.ProvenanceDigest, RootRef: generation.Ref, RootVersion: generation.Version, RootDigest: generation.Digest, SuccessionDecisionDigest: successionDigest, Operation: operation, ExpiresAt: expires}}
	requestDigest, err := repo.SaveAuthorityRequest(context.Background(), request, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "decision:" + requestID, DecisionVersion: "1", DecidedBy: generation.Principal, AuthorityRef: generation.Ref, AuthorityVersion: generation.Version, AuthorityGenerationDigest: generation.Digest, GrantedScope: generation.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: successionDigest, IssuedAt: now, ExpiresAt: &expires}
	if err := repo.SaveAuthorityDecision(context.Background(), request.ID, request.Version, decision, now, &expires); err != nil {
		t.Fatal(err)
	}
	return decision
}

func saveRecoveryRootSuccessor(t *testing.T, repo goalstore.Repository, predecessor contracts.AuthorityGeneration, installationDigest string, now time.Time) contracts.AuthorityGeneration {
	t.Helper()
	proposal, err := contracts.BuildRootAuthoritySuccession(predecessor, installationDigest, now)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest, err := repo.SaveRootAuthoritySuccessionProposal(context.Background(), proposal, now)
	if err != nil {
		t.Fatal(err)
	}
	confirmation := "REVIEW-ROOT-SUCCESSOR " + proposalDigest
	_, reviewDigest, err := repo.SaveRootAuthoritySuccessionReview(context.Background(), proposalDigest, predecessor.Principal, "fixture", confirmation, now)
	if err != nil {
		t.Fatal(err)
	}
	accept := "ACCEPT-ROOT-SUCCESSOR " + proposalDigest + " " + reviewDigest
	successor, _, err := repo.AcceptRootAuthoritySuccession(context.Background(), proposalDigest, reviewDigest, installationDigest, "fixture", accept, now)
	if err != nil {
		t.Fatal(err)
	}
	return successor
}

func captureRun(t *testing.T, args []string) error {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	runErr := run(args)
	_ = write.Close()
	os.Stdout = original
	_, _ = bytes.NewBuffer(nil).ReadFrom(read)
	_ = read.Close()
	return runErr
}

func TestLifecycleRecoveryCLIEndToEndFrom0084352ThroughDynamicResolution(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := revertRuntimeBindingsToExact0084352(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	inactiveBefore := seedRecoveryFixture(t, db, []recoveryFixtureRow{
		{entryPointID: "goal", packageID: "praxis.package.goals", packageVersion: "1", contentDigest: fixtureDigest('a')},
		{entryPointID: "goal", packageID: "praxis.package.goals", packageVersion: "2", contentDigest: fixtureDigest('b'), active: true},
		{entryPointID: "status", packageID: "praxis.package.status", packageVersion: "1", contentDigest: fixtureDigest('c'), active: true},
	}, now)
	const historicalGoalContract = `{"Version":"1","PackageID":"praxis.package.goals","PackageVersion":"2","GraphID":"graph:goal","GraphVersion":"1","EntryPointID":"goal","Aliases":["goal"],"Options":null,"RequiredCapabilities":null,"OptionalCapabilities":null,"RequiredEnforcement":null,"RequireExclusiveMediation":false}`
	historicalGoalDigest := exactContractDigest([]byte(historicalGoalContract))
	if _, err := db.Exec(`UPDATE invocation_registry SET contract_json=?,contract_digest=? WHERE entry_point_id='goal' AND package_version='2'`, []byte(historicalGoalContract), historicalGoalDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE invocation_runtime_bindings SET contract_digest=? WHERE entry_point_id='goal' AND package_version='2'`, historicalGoalDigest); err != nil {
		t.Fatal(err)
	}
	service := praxiscrypto.EnvelopeService{Wrapper: recoveryTestWrapper{}}
	installationDigest := fixtureDigest('9')
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: "fixture-recovery-key", Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: installationDigest}
	owner, err := contracts.InstallationOwnerPrincipal(installationDigest)
	if err != nil {
		t.Fatal(err)
	}
	governanceScope, err := contracts.InstallationGovernanceScope(installationDigest)
	if err != nil {
		t.Fatal(err)
	}
	generation := contracts.AuthorityGeneration{Ref: governanceScope, Version: "1", Principal: owner, Scope: governanceScope, ProvenanceRef: "bootstrap-record:" + installationDigest + ":os-user:fixture", ProvenanceDigest: installationDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-time.Hour), Capabilities: []string{contracts.AuthorityDelegateCapability}}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, now, nil); err != nil {
		t.Fatal(err)
	}
	generation = saveRecoveryRootSuccessor(t, repo, generation, installationDigest, now)
	saveRecoveryAuthority(t, repo, generation, contracts.GovernedInstallationRepairStorageSchema, "repair-storage", now)
	saveRecoveryAuthority(t, repo, generation, contracts.GovernedInstallationRepairRuntimeState, "repair-runtime", now)
	expiredAt := time.Now().UTC().Add(500 * time.Millisecond)
	saveRecoveryAuthorityUntil(t, repo, generation, contracts.GovernedInstallationRepairStorageSchema, "repair-expired-storage", time.Now().UTC(), expiredAt)
	revokedDecision := saveRecoveryAuthority(t, repo, generation, contracts.GovernedInstallationRepairStorageSchema, "repair-revoked-storage", now)
	revokedDigest, err := revokedDecision.Digest()
	if err != nil {
		t.Fatal(err)
	}
	revocation := contracts.AuthorityRevocation{RequestID: revokedDecision.RequestID, RequestVersion: revokedDecision.RequestVersion, DecisionRef: revokedDecision.DecisionRef, DecisionVersion: revokedDecision.DecisionVersion, DecisionDigest: revokedDigest, RevocationRef: "repair-revocation", RevocationVersion: "1", RevokedBy: owner, AuthorityDigest: fixtureDigest('8'), EffectiveAt: now, Reason: "fixture revocation before mutation"}
	if err := repo.SaveAuthorityRevocation(ctx, revokedDecision.RequestID, revokedDecision.RequestVersion, revocation, now, nil); err != nil {
		t.Fatal(err)
	}
	var structuralManifest []byte
	if err := db.QueryRow(`SELECT manifest_json FROM installed_packages WHERE package_id='praxis.package.status' AND package_version='1'`).Scan(&structuralManifest); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE installed_packages SET manifest_json=? WHERE package_id='praxis.package.status' AND package_version='1'`, []byte(`{"not":"a manifest"}`)); err != nil {
		t.Fatal(err)
	}
	structuralBuild, err := buildLifecycleRecoveryPlan(ctx, "storage-schema", repo, db, lifecycleRecoveryArgs{dbPath: dbPath, installation: "fixture-installation", planID: "structural-independent", planVersion: "1", storageRequest: "repair-storage", storageVersion: "1", runtimeRequest: "repair-runtime", runtimeVersion: "1", actor: owner}, now)
	if err != nil {
		t.Fatalf("structural planning depended on semantic runtime eligibility: %v", err)
	}
	if structuralBuild.RuntimeRefusal == "" || structuralBuild.StoragePrepared.SchemaDigest == "" {
		t.Fatalf("structural plan did not preserve a bounded runtime refusal: %+v", structuralBuild)
	}
	if _, err := db.Exec(`UPDATE installed_packages SET manifest_json=? WHERE package_id='praxis.package.status' AND package_version='1'`, structuralManifest); err != nil {
		t.Fatal(err)
	}
	if wait := time.Until(expiredAt) + 50*time.Millisecond; wait > 0 {
		<-time.After(wait)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	originalOpen := openLifecycleRecoveryRepository
	openLifecycleRecoveryRepository = func(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, error) {
		opened, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
		if err != nil {
			return goalstore.Repository{}, nil, err
		}
		return goalstore.Repository{Store: state.New(opened), Crypto: service, KeyRef: "fixture-recovery-key", Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: installationDigest}, opened, nil
	}
	t.Cleanup(func() { openLifecycleRecoveryRepository = originalOpen })
	t.Setenv("PRAXIS_DB", dbPath)
	t.Setenv("PRAXIS_BOOTSTRAP_RECORD", "fixture-opener-bypasses-platform-bootstrap")
	t.Setenv("PRAXIS_ACTOR_ID", "recovery-operator")
	t.Setenv("PRAXIS_ACTOR_KIND", "human")
	common := []string{"--installation", "fixture-installation", "--plan", "plan-016-wu10", "--storage-authority-request", "repair-storage", "--runtime-authority-request", "repair-runtime"}
	_, orphanSnapshotBase := lifecycleRecoveryEvidencePaths(lifecycleRecoveryArgs{dbPath: dbPath, installation: "fixture-installation", planID: "plan-016-wu10", planVersion: "1"})
	if err := os.WriteFile(orphanSnapshotBase, []byte("crash-orphan"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, refusal := range []struct {
		name, plan, request string
	}{{"expired", "plan-expired", "repair-expired-storage"}, {"revoked", "plan-revoked", "repair-revoked-storage"}} {
		args := []string{"--installation", "fixture-installation", "--plan", refusal.plan, "--storage-authority-request", refusal.request, "--runtime-authority-request", "repair-runtime"}
		if err := captureRun(t, append([]string{"lifecycle-recover", "storage-schema"}, args...)); err == nil {
			t.Fatalf("%s durable authority was accepted", refusal.name)
		}
	}
	refusalDB, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	refusalShape, err := lifecycle.IntrospectInvocationRuntimeBindingsShape(ctx, refusalDB)
	if err != nil {
		t.Fatal(err)
	}
	historicalDigest, _ := lifecycle.HistoricalInvocationRuntimeBindingsShape().Digest()
	refusalDigest, _ := refusalShape.Digest()
	if refusalDigest != historicalDigest {
		t.Fatal("durable authority refusal occurred after schema mutation")
	}
	refusalJournal, err := lifecycle.NewJournal(state.NewSQLiteEventStore(refusalDB), "fixture-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	refusalHistory, err := refusalJournal.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(refusalHistory) != 0 {
		t.Fatalf("durable authority rejection appended lifecycle events: %+v", refusalHistory)
	}
	_ = refusalDB.Close()
	if err := captureRun(t, append([]string{"lifecycle-recover", "storage-schema"}, common...)); err != nil {
		t.Fatalf("storage_schema CLI transition: %v", err)
	}

	// First restart: a new read-only connection reconstructs all assertions
	// solely from durable database state.
	readOnly, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	shape, err := lifecycle.IntrospectInvocationRuntimeBindingsShape(ctx, readOnly)
	if err != nil {
		t.Fatal(err)
	}
	shapeDigest, _ := shape.Digest()
	canonicalDigest, _ := lifecycle.CanonicalInvocationRuntimeBindingsShape().Digest()
	if shapeDigest != canonicalDigest {
		t.Fatalf("storage_schema did not produce canonical live shape: %s", shapeDigest)
	}
	storageJournal, err := lifecycle.NewJournal(state.NewSQLiteEventStore(readOnly), "fixture-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := storageJournal.Load(ctx)
	if err != nil || history[len(history)-1].StepID != "storage-schema" || history[len(history)-1].State != contracts.LifecycleCommitted {
		t.Fatalf("storage_schema durable journal after restart: history=%+v err=%v", history, err)
	}
	storageHistoryLength := len(history)
	_ = readOnly.Close()
	if err := captureRun(t, append([]string{"lifecycle-recover", "storage-schema"}, common...)); err != nil {
		t.Fatalf("terminal storage_schema restart: %v", err)
	}
	readOnly, err = state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	storageJournal, err = lifecycle.NewJournal(state.NewSQLiteEventStore(readOnly), "fixture-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	history, err = storageJournal.Load(ctx)
	if err != nil || len(history) != storageHistoryLength {
		t.Fatalf("committed storage_schema was re-attempted after restart: len=%d want=%d err=%v", len(history), storageHistoryLength, err)
	}
	_ = readOnly.Close()

	if err := captureRun(t, append([]string{"lifecycle-recover", "runtime-state"}, common...)); err != nil {
		t.Fatalf("runtime_state CLI transition: %v", err)
	}

	// Second restart: reconstruct again and verify actual state independently.
	readOnly, err = state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := lifecycle.NewJournal(state.NewSQLiteEventStore(readOnly), "fixture-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	history, err = journal.Load(ctx)
	if err != nil || history[len(history)-1].StepID != "runtime-state" || history[len(history)-1].State != contracts.LifecycleCommitted {
		t.Fatalf("runtime_state durable journal after restart: history=%+v err=%v", history, err)
	}
	rows, err := readOnly.Query(`SELECT ir.contract_json,rb.runtime_id,rb.runtime_version,rb.runtime_digest FROM invocation_registry ir JOIN invocation_runtime_bindings rb USING(entry_point_id,package_version,content_digest) WHERE ir.active=1 ORDER BY ir.entry_point_id`)
	if err != nil {
		t.Fatal(err)
	}
	activeKeys := map[string]bool{}
	for rows.Next() {
		var contractJSON []byte
		var runtimeID, runtimeVersion, runtimeDigest string
		if err := rows.Scan(&contractJSON, &runtimeID, &runtimeVersion, &runtimeDigest); err != nil {
			t.Fatal(err)
		}
		contract, err := contracts.DecodeInvocationContract(contractJSON)
		if err != nil {
			t.Fatal(err)
		}
		activeKeys[contract.EntryPointID+"\x00"+contract.PackageVersion] = true
		if runtimeID != "client-adapter:"+contract.PackageID+":"+contract.EntryPointID || runtimeVersion != contract.Version || runtimeDigest != exactContractDigest(contractJSON) {
			t.Fatalf("active binding not independently reconstructed: id=%s version=%s digest=%s contract=%+v", runtimeID, runtimeVersion, runtimeDigest, contract)
		}
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	wantActiveKeys := map[string]bool{"goal\x002": true, "status\x001": true}
	if !reflect.DeepEqual(activeKeys, wantActiveKeys) {
		t.Fatalf("recovered active key set/count = %+v, want %+v", activeKeys, wantActiveKeys)
	}
	for key, before := range inactiveBefore {
		parts := strings.Split(key, "\x00")
		after := bindingGenerationBytes(t, readOnly, parts[0], parts[1], false)
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("inactive retained generation %q changed", key)
		}
	}
	runtimeHistoryLength := len(history)
	if err := readOnly.Close(); err != nil {
		t.Fatal(err)
	}
	if err := captureRun(t, append([]string{"lifecycle-recover", "runtime-state"}, common...)); err != nil {
		t.Fatalf("terminal runtime_state restart: %v", err)
	}
	readOnly, err = state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	journal, err = lifecycle.NewJournal(state.NewSQLiteEventStore(readOnly), "fixture-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	history, err = journal.Load(ctx)
	if err != nil || len(history) != runtimeHistoryLength {
		t.Fatalf("committed runtime_state was re-attempted after restart: len=%d want=%d err=%v", len(history), runtimeHistoryLength, err)
	}
	resolved, err := resolveDynamicInvocation(ctx, []string{"goal", "recover"}, os.Getenv)
	if err != nil || resolved.EntryPointID != "goal" || resolved.PackageVersion != "2" {
		t.Fatalf("ordinary dynamic Goal resolution after recovery: resolved=%+v err=%v", resolved, err)
	}
}

func TestLifecycleRecoveryCLIProductionRetryOfFailedRecoverable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := revertRuntimeBindingsToExact0084352(ctx, db); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	seedRecoveryFixture(t, db, []recoveryFixtureRow{{entryPointID: "retry", packageID: "praxis.package.retry", packageVersion: "1", contentDigest: fixtureDigest('6'), active: true}}, now)
	service := praxiscrypto.EnvelopeService{Wrapper: recoveryTestWrapper{}}
	installationDigest := fixtureDigest('7')
	repo := goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: "fixture-retry-key", Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: installationDigest}
	owner, err := contracts.InstallationOwnerPrincipal(installationDigest)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := contracts.InstallationGovernanceScope(installationDigest)
	if err != nil {
		t.Fatal(err)
	}
	generation := contracts.AuthorityGeneration{Ref: scope, Version: "1", Principal: owner, Scope: scope, ProvenanceRef: "bootstrap-record:" + installationDigest + ":os-user:fixture", ProvenanceDigest: installationDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-time.Hour), Capabilities: []string{contracts.AuthorityDelegateCapability}}
	generation.Digest, err = generation.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveAuthorityGeneration(ctx, generation, now, nil); err != nil {
		t.Fatal(err)
	}
	generation = saveRecoveryRootSuccessor(t, repo, generation, installationDigest, now)
	saveRecoveryAuthority(t, repo, generation, contracts.GovernedInstallationRepairStorageSchema, "retry-storage", now)
	saveRecoveryAuthority(t, repo, generation, contracts.GovernedInstallationRepairRuntimeState, "retry-runtime", now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	originalOpen := openLifecycleRecoveryRepository
	openLifecycleRecoveryRepository = func(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, error) {
		opened, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
		if err != nil {
			return goalstore.Repository{}, nil, err
		}
		return goalstore.Repository{Store: state.New(opened), Crypto: service, KeyRef: "fixture-retry-key", Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: installationDigest}, opened, nil
	}
	t.Cleanup(func() { openLifecycleRecoveryRepository = originalOpen })
	t.Setenv("PRAXIS_DB", dbPath)
	t.Setenv("PRAXIS_BOOTSTRAP_RECORD", "fixture-opener-bypasses-platform-bootstrap")
	t.Setenv("PRAXIS_ACTOR_ID", "recovery-operator")
	t.Setenv("PRAXIS_ACTOR_KIND", "human")
	common := []string{"--installation", "retry-installation", "--plan", "retry-plan", "--storage-authority-request", "retry-storage", "--runtime-authority-request", "retry-runtime"}
	if err := captureRun(t, append([]string{"lifecycle-recover", "storage-schema"}, common...)); err != nil {
		t.Fatal(err)
	}

	writable, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var originalManifest []byte
	if err := writable.QueryRow(`SELECT manifest_json FROM installed_packages WHERE package_id='praxis.package.retry'`).Scan(&originalManifest); err != nil {
		t.Fatal(err)
	}
	var manifestValue any
	if err := json.Unmarshal(originalManifest, &manifestValue); err != nil {
		t.Fatal(err)
	}
	driftedManifest, err := json.MarshalIndent(manifestValue, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writable.Exec(`UPDATE installed_packages SET manifest_json=? WHERE package_id='praxis.package.retry'`, driftedManifest); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}
	if err := captureRun(t, append([]string{"lifecycle-recover", "runtime-state"}, common...)); err != nil {
		t.Fatalf("recoverable production attempt: %v", err)
	}

	writable, err = state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := lifecycle.NewJournal(state.NewSQLiteEventStore(writable), "retry-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	history, err := journal.Load(ctx)
	if err != nil || history[len(history)-1].State != contracts.LifecycleFailedRecoverable {
		t.Fatalf("recoverable production attempt was not durable: history=%+v err=%v", history, err)
	}
	if _, err := writable.Exec(`UPDATE installed_packages SET manifest_json=? WHERE package_id='praxis.package.retry'`, originalManifest); err != nil {
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}
	if err := captureRun(t, append([]string{"lifecycle-recover", "runtime-state"}, common...)); err != nil {
		t.Fatalf("production retry from FailedRecoverable: %v", err)
	}
	readOnly, err := state.OpenSQLiteReadOnly(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	journal, err = lifecycle.NewJournal(state.NewSQLiteEventStore(readOnly), "retry-installation", contracts.PrincipalRef{ID: "reader", Kind: "system"})
	if err != nil {
		t.Fatal(err)
	}
	history, err = journal.Load(ctx)
	if err != nil || history[len(history)-1].State != contracts.LifecycleCommitted {
		t.Fatalf("production retry did not commit: history=%+v err=%v", history, err)
	}
	committedHistoryLength := len(history)

	// Terminal recognition precedes live authority construction. Once the
	// exact committed attempt has been recognized, later expiry cannot turn a
	// read-only restart into a new authority-gated mutation attempt.
	expiryDB, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if _, err := expiryDB.Exec(`UPDATE secure_blobs SET expires_at=? WHERE namespace='authority_decision' AND object_id IN ('retry-storage','retry-runtime')`, past); err != nil {
		_ = expiryDB.Close()
		t.Fatal(err)
	}
	if err := expiryDB.Close(); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"storage-schema", "runtime-state"} {
		if err := captureRun(t, append([]string{"lifecycle-recover", operation}, common...)); err != nil {
			t.Fatalf("committed %s restart required expired authority: %v", operation, err)
		}
	}
	afterExpiry, err := journal.Load(ctx)
	if err != nil || len(afterExpiry) != committedHistoryLength {
		t.Fatalf("terminal restart after authority expiry mutated journal: len=%d want=%d err=%v", len(afterExpiry), committedHistoryLength, err)
	}

	// Adversarial artifact checks live outside the isolated WU10 happy path.
	evidencePath, _ := lifecycleRecoveryEvidencePaths(lifecycleRecoveryArgs{dbPath: dbPath, installation: "retry-installation", planID: "retry-plan", planVersion: "1"})
	evidenceBody, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var evidence lifecycleRecoveryBuild
	if err := json.Unmarshal(evidenceBody, &evidence); err != nil {
		t.Fatal(err)
	}
	snapshotPath := evidence.SnapshotPath
	evidence.StoragePrepared.RetainedPopulationDigest = fixtureDigest('0')
	tamperedEvidence, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, tamperedEvidence, 0600); err != nil {
		t.Fatal(err)
	}
	if err := captureRun(t, append([]string{"lifecycle-recover", "runtime-state"}, common...)); err == nil {
		t.Fatal("CLI accepted prepared evidence not bound to the exact durable plan")
	}
	if err := os.WriteFile(evidencePath, evidenceBody, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(snapshotPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("tampered")); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := captureRun(t, append([]string{"lifecycle-recover", "runtime-state"}, common...)); err == nil {
		t.Fatal("CLI accepted a digest-mismatched lifecycle recovery snapshot")
	}
}
