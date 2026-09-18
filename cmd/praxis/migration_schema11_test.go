package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/migrations/sqlite"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// schema11EnrollmentRoot is the exact AuthorityGeneration wire form of commit
// 5850f27, the boundary that enrolled installation roots at storage schema 11
// before DelegatedBy, Capabilities, and the installation-governance scope
// existed. A real schema-11 dogfood installation carries exactly this form.
type schema11EnrollmentRoot struct {
	Ref              string                             `json:"ref"`
	Version          string                             `json:"version"`
	Digest           string                             `json:"digest"`
	Principal        contracts.PrincipalRef             `json:"principal"`
	Scope            string                             `json:"scope"`
	ProvenanceRef    string                             `json:"provenance_ref"`
	ProvenanceDigest string                             `json:"provenance_digest"`
	State            contracts.AuthorityGenerationState `json:"state"`
	EffectiveAt      time.Time                          `json:"effective_at"`
}

// historicalDatabaseAt applies the embedded migrations through the given
// schema exactly as migrations.Apply did when that schema was the latest,
// producing a real historical state file rather than a mocked one.
func historicalDatabaseAt(t *testing.T, ctx context.Context, path string, schema int) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "sqlite", "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("migration sources unavailable: %v", err)
	}
	sort.Strings(files)
	db, err := state.OpenSQLiteForGovernedMigration(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS praxis_schema_migrations (name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		name := filepath.Base(file)
		if name > fmt.Sprintf("%04d_", schema)+"\xff" {
			break
		}
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO praxis_schema_migrations(name,applied_at) VALUES(?,?)`, name, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	status, err := sqlite.StatusOf(ctx, db)
	if err != nil || status.CurrentSchema != schema {
		t.Fatalf("historical fixture is not at schema %d: %+v %v", schema, status, err)
	}
}

// enrollSchema11Root reproduces commit 5850f27's persistence path
// (SaveAuthorityGeneration -> putWorkPlanBlob -> PutSecureBlob) byte for
// byte: nine-field payload, digest over that exact form, secure-blob AAD, raw
// secure_blobs row. The reserved-namespace boundary is bypassed only here
// because the historical writer predates it.
func enrollSchema11Root(t *testing.T, ctx context.Context, path string, service praxiscrypto.EnvelopeService, keyRef string, bootstrapDigest, scope, osUser string, now time.Time) string {
	t.Helper()
	root := schema11EnrollmentRoot{Ref: "installation-governance:" + bootstrapDigest, Version: "1", Principal: contracts.PrincipalRef{ID: "installation-owner:" + bootstrapDigest, Kind: "human"}, Scope: scope, ProvenanceRef: "bootstrap-record:" + bootstrapDigest + ":os-user:" + osUser, ProvenanceDigest: bootstrapDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now}
	unsigned, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	root.Digest = exactContractDigest(unsigned)
	payload, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	objectDigest := exactContractDigest(payload)
	envelope, err := service.Seal(ctx, keyRef, contracts.CryptoClassicalCompatible, payload, state.SecureBlobAAD(state.AuthorityGenerationNamespace, root.Ref, root.Version, objectDigest))
	if err != nil {
		t.Fatal(err)
	}
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenSQLiteForGovernedMigration(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO secure_blobs(namespace,object_id,object_version,object_digest,sensitivity,crypto_profile,envelope_json,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,NULL)`, state.AuthorityGenerationNamespace, root.Ref, root.Version, objectDigest, string(state.SensitivityConfidential), string(contracts.CryptoClassicalCompatible), envelopeJSON, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return root.Digest
}

type schema11Installation struct {
	dbPath          string
	bootstrapDigest string
	rootDigest      string
	getenv          func(string) string
	service         praxiscrypto.EnvelopeService
	record          praxiscrypto.BootstrapRecord
	osUser          string
}

func schema11InstallationFixture(t *testing.T, ctx context.Context, schema int) schema11Installation {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "praxis.db")
	historicalDatabaseAt(t, ctx, dbPath, schema)
	now := time.Now().UTC().Truncate(time.Microsecond)
	record := praxiscrypto.BootstrapRecord{Version: praxiscrypto.BootstrapRecordVersion, ProviderID: "fixture", KeyID: "fixture-key", KeyVersion: "1", KeyMaterialHash: fixtureDigest('1'), Owner: "fixture", Purpose: "schema-11 migration qualification", Profile: contracts.CryptoClassicalCompatible, SecurityLevel: praxiscrypto.SecurityPortableUserControlled, Platform: "test", Architecture: "test", CreatedAt: now.Add(-24 * time.Hour)}
	bootstrapDigest, err := record.Digest()
	if err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		t.Skip("authenticated OS user unavailable")
	}
	service := praxiscrypto.EnvelopeService{Wrapper: recoveryTestWrapper{}}
	const keyRef = "fixture-migration-key"
	rootDigest := enrollSchema11Root(t, ctx, dbPath, service, keyRef, bootstrapDigest, "goal:dogfood/baseline/1/proposal/wp-proposal-v1", current.Username, now.Add(-time.Hour))
	original := openGovernedRepositoryReadOnly
	openGovernedRepositoryReadOnly = func(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, praxiscrypto.BootstrapRecord, error) {
		db, err := state.OpenSQLiteReadOnly(ctx, getenv("PRAXIS_DB"))
		if err != nil {
			return goalstore.Repository{}, nil, record, err
		}
		return goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: keyRef, Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: bootstrapDigest}, db, record, nil
	}
	t.Cleanup(func() { openGovernedRepositoryReadOnly = original })
	originalWrite := openGovernedRepository
	openGovernedRepository = func(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, error) {
		db, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
		if err != nil {
			return goalstore.Repository{}, nil, err
		}
		return goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: keyRef, Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: bootstrapDigest}, db, nil
	}
	t.Cleanup(func() { openGovernedRepository = originalWrite })
	env := map[string]string{"PRAXIS_DB": dbPath, "PRAXIS_BOOTSTRAP_RECORD": filepath.Join(dir, "bootstrap.json")}
	return schema11Installation{dbPath: dbPath, bootstrapDigest: bootstrapDigest, rootDigest: rootDigest, getenv: func(key string) string { return env[key] }, service: service, record: record, osUser: current.Username}
}

func TestGovernedMigrationFromRealSchema11RootReaches18(t *testing.T) {
	ctx := context.Background()
	fixture := schema11InstallationFixture(t, ctx, 11)
	previewPath := filepath.Join(filepath.Dir(fixture.dbPath), "preview.json")

	var previewOut bytes.Buffer
	if err := runMigrationPreview([]string{"--output", previewPath}, fixture.getenv, &previewOut); err != nil {
		t.Fatalf("schema-11 installation must preview its governed migration: %v", err)
	}
	var preview struct {
		Plan         sqlite.Plan `json:"plan"`
		PlanDigest   string      `json:"plan_digest"`
		Confirmation string      `json:"confirmation"`
	}
	if err := json.Unmarshal(previewOut.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	plan := preview.Plan
	if plan.CurrentSchema != 11 || plan.TargetSchema != 18 || len(plan.Migrations) != 7 || plan.Migrations[0].Name != "0012_invocation_runtime_bindings.sql" || plan.Migrations[6].Name != "0018_authority_model_active_projection.sql" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.RootDigest != fixture.rootDigest || plan.InstallationRoot != "installation-governance:"+fixture.bootstrapDigest || plan.BootstrapDigest != fixture.bootstrapDigest {
		t.Fatalf("plan must be authorized by the schema-11 enrollment root: %+v", plan)
	}
	if err := plan.Verify(); err != nil || preview.PlanDigest != plan.PlanDigest || preview.Confirmation != "MIGRATE "+plan.PlanDigest {
		t.Fatalf("preview envelope invalid: %v", err)
	}

	if err := runMigrationExecuteWithTerminal([]string{"--preview-file", previewPath}, fixture.getenv, strings.NewReader("MIGRATE "+plan.PlanDigest+"\n"), &bytes.Buffer{}, false); err != errAuthorityBootstrapConfirmation {
		t.Fatalf("non-interactive execution must be refused: %v", err)
	}
	var executeOut bytes.Buffer
	if err := runMigrationExecuteWithTerminal([]string{"--preview-file", previewPath}, fixture.getenv, strings.NewReader("MIGRATE "+plan.PlanDigest+"\n"), &executeOut, true); err != nil {
		t.Fatalf("governed 11->18 execution failed: %v\n%s", err, executeOut.String())
	}
	if !strings.Contains(executeOut.String(), `"state": "committed"`) || !strings.Contains(executeOut.String(), "0018_authority_model_active_projection.sql") {
		t.Fatalf("journal must commit through 0018: %s", executeOut.String())
	}

	db, err := state.OpenSQLiteReadOnly(ctx, fixture.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	status, err := sqlite.StatusOf(ctx, db)
	if err != nil || status.CurrentSchema != 18 || len(status.Pending) != 0 {
		t.Fatalf("installation did not reach schema 18: %+v %v", status, err)
	}
	var pointer string
	if err := db.QueryRowContext(ctx, `SELECT object_id FROM authority_model_active WHERE singleton_id='authority-model-current'`).Scan(&pointer); err != nil || pointer != "active-authority-model" {
		t.Fatalf("0018 projection missing after migration: %q %v", pointer, err)
	}
	journal, err := sqlite.ReadJournal(ctx, db, plan.PlanDigest)
	if err != nil || journal.State != "committed" {
		t.Fatalf("journal not committed: %+v %v", journal, err)
	}

	var statusOut bytes.Buffer
	if err := runAuthorityModelStatus(nil, fixture.getenv, &statusOut); err != nil || !strings.Contains(statusOut.String(), "implicit-v1") {
		t.Fatalf("authority model-status must resolve after 0018: %v %s", err, statusOut.String())
	}
	// The migration ceremony is complete. Bringing the schema-11 enrollment
	// root itself forward to the current root semantics (governance scope,
	// capabilities, authority model) is a separate governed ceremony; until
	// then current-semantics root resolution reports no root at schema 18.
	if err := runMigrationPreview(nil, fixture.getenv, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("post-migration preview must fail closed on current root semantics, not on schema state: %v", err)
	}

	repo, readDB, _, err := openGovernedRepositoryReadOnly(ctx, fixture.getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer readDB.Close()
	generations, err := repo.ListAuthorityGenerations(ctx, time.Now().UTC())
	if err != nil || len(generations) != 1 || generations[0].Digest != fixture.rootDigest || !generations[0].PreDelegationForm() {
		t.Fatalf("the enrollment root must remain intact and verifiable after migration: %+v %v", generations, err)
	}
}

func TestGovernedMigrationDoesNotAdmitLegacyRootBeyondSchema11(t *testing.T) {
	ctx := context.Background()
	fixture := schema11InstallationFixture(t, ctx, 12)
	err := runMigrationPreview(nil, fixture.getenv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("a least-scope root must not authorize migration from schema 12: %v", err)
	}
}
