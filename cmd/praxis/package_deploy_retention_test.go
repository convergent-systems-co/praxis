package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// governedInstallationFixture creates a current-schema governed installation
// with a bootstrap record file, an enrolled installation root owned by the
// authenticated OS user, and no adopted authority model. CLI entry points
// resolve the installation through PRAXIS_DB / PRAXIS_BOOTSTRAP_RECORD.
func governedInstallationFixture(t *testing.T, ctx context.Context) (func(string) string, contracts.AuthorityGeneration) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "praxis.db")
	db, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	record := praxiscrypto.BootstrapRecord{Version: praxiscrypto.BootstrapRecordVersion, ProviderID: "fixture", KeyID: "fixture-key", KeyVersion: "1", KeyMaterialHash: fixtureDigest('7'), Owner: "fixture", Purpose: "package-deploy retention qualification", Profile: contracts.CryptoClassicalCompatible, SecurityLevel: praxiscrypto.SecurityPortableUserControlled, Platform: "test", Architecture: "test", CreatedAt: now.Add(-24 * time.Hour)}
	bootstrapPath := filepath.Join(dir, "bootstrap.json")
	if err := praxiscrypto.SaveBootstrapRecord(bootstrapPath, record); err != nil {
		t.Fatal(err)
	}
	record, err = praxiscrypto.LoadBootstrapRecord(bootstrapPath)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapDigest, err := record.Digest()
	if err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil || current.Username == "" {
		t.Skip("authenticated OS user unavailable")
	}
	service := praxiscrypto.EnvelopeService{Wrapper: recoveryTestWrapper{}}
	const keyRef = "fixture-retention-key"
	repository := func(db *sql.DB) goalstore.Repository {
		return goalstore.Repository{Store: state.New(db), Crypto: service, KeyRef: keyRef, Profile: contracts.CryptoClassicalCompatible, Sensitivity: state.SensitivityConfidential, InstallationDigest: bootstrapDigest}
	}
	owner, _ := contracts.InstallationOwnerPrincipal(bootstrapDigest)
	scope, _ := contracts.InstallationGovernanceScope(bootstrapDigest)
	root := contracts.AuthorityGeneration{Ref: scope, Version: "1", Principal: owner, Scope: scope, Capabilities: []string{contracts.AuthorityDelegateCapability}, ProvenanceRef: "bootstrap-record:" + bootstrapDigest + ":os-user:" + current.Username, ProvenanceDigest: bootstrapDigest, State: contracts.AuthorityGenerationActive, EffectiveAt: now.Add(-time.Hour), AuthorityModel: contracts.AuthorityModelID, AuthorityModelVersion: contracts.AuthorityModelVersion, AuthorityModelDigest: contracts.AuthorityModelDigest()}
	root.Digest, err = root.ComputeDigest()
	if err != nil {
		t.Fatal(err)
	}
	writeDB, err := state.OpenSQLite(ctx, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository(writeDB).SaveAuthorityGeneration(ctx, root, root.EffectiveAt, nil); err != nil {
		t.Fatal(err)
	}
	writeDB.Close()
	original := openGovernedRepositoryReadOnly
	openGovernedRepositoryReadOnly = func(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, praxiscrypto.BootstrapRecord, error) {
		db, err := state.OpenSQLiteReadOnly(ctx, getenv("PRAXIS_DB"))
		if err != nil {
			return goalstore.Repository{}, nil, record, err
		}
		return repository(db), db, record, nil
	}
	t.Cleanup(func() { openGovernedRepositoryReadOnly = original })
	originalWrite := openGovernedRepository
	openGovernedRepository = func(ctx context.Context, getenv func(string) string) (goalstore.Repository, *sql.DB, error) {
		db, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
		if err != nil {
			return goalstore.Repository{}, nil, err
		}
		return repository(db), db, nil
	}
	t.Cleanup(func() { openGovernedRepository = originalWrite })
	t.Setenv("PRAXIS_DB", dbPath)
	t.Setenv("PRAXIS_BOOTSTRAP_RECORD", bootstrapPath)
	env := map[string]string{"PRAXIS_DB": dbPath, "PRAXIS_BOOTSTRAP_RECORD": bootstrapPath}
	return func(key string) string { return env[key] }, root
}

func adoptNextAuthorityModel(t *testing.T, getenv func(string) string, want string) {
	t.Helper()
	previewFile := filepath.Join(t.TempDir(), "model-preview-"+want+".json")
	var previewOut bytes.Buffer
	if err := runAuthorityModelPreview([]string{"--output", previewFile}, getenv, &previewOut); err != nil {
		t.Fatalf("adoption preview toward %s failed: %v", want, err)
	}
	var preview struct {
		Adoption      contracts.AuthorityModelAdoption `json:"adoption"`
		PreviewDigest string                           `json:"preview_digest"`
	}
	if err := json.Unmarshal(previewOut.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Adoption.ToVersion != want {
		t.Fatalf("adoption preview offered %s, want %s", preview.Adoption.ToVersion, want)
	}
	if err := runAuthorityModelAdoptWithTerminal([]string{"--preview-file", previewFile}, getenv, strings.NewReader("ADOPT "+preview.PreviewDigest+"\n"), &bytes.Buffer{}, true); err != nil {
		t.Fatalf("adoption to %s failed: %v", want, err)
	}
}

func packageDeployPreview(t *testing.T, dir, name string) (contracts.GovernedAuthorityProposal, string, string, error) {
	t.Helper()
	previewFile := filepath.Join(dir, "package-deploy-preview-"+name+".json")
	var out bytes.Buffer
	err := runPackageManagerAuthorityPreview([]string{"--expires-at", time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), "--output", previewFile}, &out)
	if err != nil {
		return contracts.GovernedAuthorityProposal{}, "", "", err
	}
	var preview struct {
		Proposal contracts.GovernedAuthorityProposal `json:"proposal"`
		Digest   string                              `json:"proposal_digest"`
	}
	if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	return preview.Proposal, preview.Digest, previewFile, nil
}

// TestPackageDeployAdmissionFollowsRetainedV3Semantics walks a governed
// installation through the canonical adoption chain and proves package.deploy
// admission is decided by retained v3 semantics rather than an exact pin:
// v1 and v2 refuse, v3 admits, v6 admits, and every admitted proposal binds
// the v3 deployment identity (never the routing identity).
func TestPackageDeployAdmissionFollowsRetainedV3Semantics(t *testing.T) {
	ctx := context.Background()
	getenv, root := governedInstallationFixture(t, ctx)
	dir := t.TempDir()
	if _, _, _, err := packageDeployPreview(t, dir, "v1"); err == nil || !strings.Contains(err.Error(), "retains v3") {
		t.Fatalf("v1 installation must refuse package-deploy preview, got %v", err)
	}
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelSuccessorVersion)
	if _, _, _, err := packageDeployPreview(t, dir, "v2"); err == nil || !strings.Contains(err.Error(), "retains v3") {
		t.Fatalf("v2 installation must refuse package-deploy preview, got %v", err)
	}
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelDeploymentVersion)
	v3Proposal, _, _, err := packageDeployPreview(t, dir, "v3")
	if err != nil {
		t.Fatalf("v3 installation must admit package-deploy preview: %v", err)
	}
	if v3Proposal.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || v3Proposal.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() || v3Proposal.ParentDigest != root.Digest {
		t.Fatalf("v3 proposal identity: %#v", v3Proposal)
	}
	{
		repo, db, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
		if err != nil {
			t.Fatal(err)
		}
		if err := packageManagerProposalCurrent(ctx, repo, v3Proposal, time.Now().UTC()); err != nil {
			t.Fatalf("v3 proposal must be current under v3: %v", err)
		}
		db.Close()
	}

	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelRoutingVersion)
	var status bytes.Buffer
	if err := runAuthorityModelStatus(nil, getenv, &status); err != nil || !strings.Contains(status.String(), `"ActiveVersion": "v6"`) {
		t.Fatalf("installation is not under v6: %v %s", err, status.String())
	}
	v6Proposal, v6Digest, v6File, err := packageDeployPreview(t, dir, "v6")
	if err != nil {
		t.Fatalf("v6 installation must admit package-deploy preview (retains v3): %v", err)
	}
	if v6Proposal.AuthorityModelVersion != contracts.AuthorityModelDeploymentVersion || v6Proposal.AuthorityModelDigest != contracts.AuthorityModelDeploymentDigest() {
		t.Fatalf("proposal under v6 must bind the v3 deployment identity, not the active model: %s %s", v6Proposal.AuthorityModelVersion, v6Proposal.AuthorityModelDigest)
	}
	if v6Proposal.ParentDigest != root.Digest || v6Proposal.Profile != contracts.DelegationProfilePackageDeploy || v6Proposal.Capability != contracts.GovernedPackageDeploy {
		t.Fatalf("proposal under v6 is not the closed package-deploy profile: %#v", v6Proposal)
	}
	var proposalOut bytes.Buffer
	if err := runPackageManagerAuthorityProposal([]string{"--preview-file", v6File}, getenv, &proposalOut); err != nil {
		t.Fatalf("package-deploy proposal under v6 must persist: %v", err)
	}
	if !strings.Contains(proposalOut.String(), v6Digest) {
		t.Fatalf("persisted proposal digest mismatch: %s", proposalOut.String())
	}
	// A proposal is an attempt: renewing package-deploy authority for the
	// same root persists a distinct durable proposal.
	renewal, renewalDigest, renewalFile, err := packageDeployPreview(t, dir, "v6-renewal")
	if err != nil || renewalDigest == v6Digest || renewal.ID == v6Proposal.ID {
		t.Fatalf("renewal preview must be a distinct attempt: %v %s %s", err, renewal.ID, v6Proposal.ID)
	}
	if err := runPackageManagerAuthorityProposal([]string{"--preview-file", renewalFile}, getenv, &bytes.Buffer{}); err != nil {
		t.Fatalf("renewed package-deploy proposal must persist alongside the earlier attempt: %v", err)
	}
	repo, db, _, err := openGovernedRepositoryReadOnly(ctx, getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	stored, err := repo.LoadGovernedAuthorityProposalByDigest(ctx, v6Digest, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := packageManagerProposalCurrent(ctx, repo, stored, now); err != nil {
		t.Fatalf("stored proposal must be current under v6: %v", err)
	}
	// Routing capability never leaks backward into the deployment profile: a
	// proposal labelled with the active v6 identity is not the closed profile.
	forged := stored
	forged.AuthorityModelVersion, forged.AuthorityModelDigest = contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest()
	if err := packageManagerProposalCurrent(ctx, repo, forged, now); err == nil || !strings.Contains(err.Error(), "closed v3") {
		t.Fatalf("v6-labelled package-deploy proposal must be refused, got %v", err)
	}
	// The package-manager principal is not resolvable until the delegation
	// ceremony completes; the model predicate alone grants nothing.
	if _, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, now); err == nil {
		t.Fatal("package-manager authority resolved without a delegated generation")
	}
}
