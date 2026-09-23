package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type localCLIContainment struct{}

func (localCLIContainment) ContainDelegation(parent contracts.AuthorityGeneration, request contracts.DelegationRequest, now time.Time) error {
	return contracts.ValidateBuiltinPackageDeployDelegation(parent, request, now)
}

func localCLIManager(t *testing.T, getenv func(string) string, root contracts.AuthorityGeneration) {
	t.Helper()
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelSuccessorVersion)
	adoptNextAuthorityModel(t, getenv, contracts.AuthorityModelDeploymentVersion)
	ctx := context.Background()
	repo, db, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	expires := now.Add(time.Hour)
	scope, err := contracts.PackageDeploymentScope(root.Digest)
	if err != nil {
		t.Fatal(err)
	}
	proposalDigest := "sha256:" + strings.Repeat("b", 64)
	reviewDigest := "sha256:" + strings.Repeat("c", 64)
	delegation := contracts.DelegationRequest{Profile: contracts.DelegationProfilePackageDeploy, ParentRef: root.Ref, ParentVersion: root.Version, ParentDigest: root.Digest, DelegatedPrincipal: contracts.PackageManagerPrincipal(), TargetKind: contracts.PackageManagerPrincipalKind, TargetIdentity: contracts.PackageManagerPrincipalID, TargetVersion: "1", TargetDigest: root.Digest, TargetConstraints: []string{root.Digest}, RequestedCapabilities: []string{}, RequestedOperations: []string{}, RequestedAuthority: contracts.GovernedPackageDeploy, RequestedOperation: "deploy", RequestedScope: scope, ProposalVersion: "1", ProposalDigest: proposalDigest, ReviewVersion: "1", ReviewDigest: reviewDigest, ExpiresAt: expires, Reason: "scratch local CLI package deployment", PolicyRef: contracts.AuthorityModelID, PolicyVersion: contracts.AuthorityModelDeploymentVersion, PolicyDigest: contracts.AuthorityModelDeploymentDigest(), SubjectKind: contracts.PackageManagerPrincipalKind, SubjectID: contracts.PackageManagerPrincipalID, SubjectVersion: "1", SubjectDigest: root.Digest}
	request := contracts.AuthorityRequest{ID: "package-manager-authority-request:" + proposalDigest + ":" + reviewDigest, Version: "1", RequestedAuthority: contracts.GovernedPackageDeploy, RequestedScope: scope, Reason: delegation.Reason, Status: contracts.AuthorityRequestPending, Delegation: &delegation}
	requestDigest, err := repo.SaveAuthorityRequest(ctx, request, now, &expires)
	if err != nil {
		t.Fatal(err)
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: contracts.AuthorityModelDeploymentDigest(), IssuedAt: now, ExpiresAt: &expires, Delegation: &delegation}
	if _, err := repo.SaveAuthorityDecisionAndDelegatedAuthorityGeneration(ctx, request.ID, request.Version, decision, localCLIContainment{}, now); err != nil {
		t.Fatal(err)
	}
}

func localCLIPackage(t *testing.T, root, packageID, version, alias string) string {
	t.Helper()
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: packageID, Version: version, Invocations: []contracts.InvocationContract{{Version: contracts.InvocationContractCurrentVersion(), PackageID: packageID, PackageVersion: version, GraphID: packageID + ".graph", GraphVersion: version, EntryPointID: packageID + ".run", Aliases: []string{alias}, Options: []contracts.InvocationOption{{Name: "mode", Type: "string", Default: "safe"}}}}}
	if packageID == "probe/dynamic" && version == "2" {
		manifest.Capabilities = []string{"network.read"}
	}
	release, artifact, trusted := signedManifestRelease(t, manifest, []byte("scratch:"+packageID+"@"+version), contracts.CryptoClassicalCompatible)
	dir := filepath.Join(root, packageID, version)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	signature, err := json.Marshal(release.Signature)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := json.Marshal(map[string]string{"manifest_digest": release.ManifestDigest, "artifact_digest": release.Manifest.ContentDigest})
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{"manifest.json": release.ManifestBytes, "artifact.tar.gz": artifact, "signature.json": signature, "identity.json": identity} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return trusted
}

func localCLIApproval(t *testing.T, getenv func(string) string, root contracts.AuthorityGeneration, ref string) string {
	t.Helper()
	ctx := context.Background()
	parsed, version, adapter, err := parsePackageDeployRef(ref, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	release, err := adapter.Resolve(ctx, parsed, version)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := adapter.FetchArtifact(ctx, release)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Second)
	resolution, err := resolveReleasePackages(ctx, adapter, release, artifact, os.Getenv, false, at)
	if err != nil {
		t.Fatal(err)
	}
	request, err := packagecatalog.NewDeploymentRequest(resolution.Root, resolution.Dependencies, contracts.PackageManagerPrincipal(), "scratch-pending")
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	evidenceID, err := persistDeploymentEvidence(ctx, db, request, at, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	request.VerificationEvidenceDigest = evidenceID
	request.Intent.Parameters["verification_evidence_digest"] = evidenceID
	repo, authDB, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer authDB.Close()
	now := time.Now().UTC()
	governed, digest, err := repo.SavePackageDeploymentRequest(ctx, request.Intent, root.Digest, request.Intent.Parameters["closure_digest"], evidenceID, now)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := repo.ResolvePackageManagerAuthority(ctx, root.Digest, now)
	if err != nil {
		t.Fatal(err)
	}
	expires := now.Add(30 * time.Minute)
	decision := contracts.AuthorityDecision{RequestID: governed.ID, RequestVersion: governed.Version, RequestDigest: digest, DecisionRef: "authority-decision:" + governed.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: root.AuthorityModelDigest, IssuedAt: now, ExpiresAt: &expires, OperationalAuthorityRef: manager.Ref, OperationalAuthorityVersion: manager.Version, OperationalAuthorityGenerationDigest: manager.Digest}
	if err := repo.SaveAuthorityDecision(ctx, governed.ID, governed.Version, decision, now, &expires); err != nil {
		t.Fatal(err)
	}
	approvalID, err := repo.DerivePackageDeploymentApproval(ctx, governed.ID, governed.Version, request.Intent, now)
	if err != nil {
		t.Fatal(err)
	}
	return approvalID
}

func localCLITransitionApproval(t *testing.T, db *sql.DB, manifest packagecatalog.Manifest, operation packagecatalog.TransitionOperation) string {
	t.Helper()
	actor := contracts.PrincipalRef{ID: "scratch-operator", Kind: "user"}
	approvalID := "scratch:" + string(operation) + ":" + manifest.Version
	request, err := packagecatalog.NewTransitionRequest(packagecatalog.PackageIdentity{PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}, operation, actor, approvalID)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := request.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, digest, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	return approvalID
}

func TestLocalCLIInstallUpdateAndRemovalScratch(t *testing.T) {
	ctx := context.Background()
	getenv, root := governedInstallationFixture(t, ctx)
	localCLIManager(t, getenv, root)
	packagesDir := filepath.Join(t.TempDir(), "packages")
	t.Setenv("PRAXIS_LOCAL_PACKAGES_DIR", packagesDir)
	trustedV1 := localCLIPackage(t, packagesDir, "probe/dynamic", "1", "probe-one")
	t.Setenv("PRAXIS_TRUSTED_KEYS", trustedV1)
	t.Setenv("PRAXIS_PACKAGE_APPROVAL_ID", localCLIApproval(t, getenv, root, "local:probe/dynamic@1"))
	t.Setenv("PRAXIS_TRUSTED_KEYS", `{}`)
	if err := run([]string{"install", "local:probe/dynamic@1"}); err == nil {
		t.Fatal("scratch CLI installed a package without its trusted signature key")
	}
	t.Setenv("PRAXIS_TRUSTED_KEYS", trustedV1)
	artifactPath := filepath.Join(packagesDir, "probe", "dynamic", "1", "artifact.tar.gz")
	artifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("tampered-local-artifact"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"install", "local:probe/dynamic@1"}); err == nil {
		t.Fatal("scratch CLI installed local bytes that changed after approval")
	}
	if err := os.WriteFile(artifactPath, artifact, 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"install", "local:probe/dynamic@1"}); err != nil {
		t.Fatalf("scratch local CLI install: %v", err)
	}
	assertAlias := func(alias, version string) {
		t.Helper()
		resolved, err := resolveDynamicInvocation(ctx, []string{alias, "--mode=deep"}, os.Getenv)
		if err != nil || resolved.PackageID != "probe/dynamic" || resolved.PackageVersion != version || resolved.Options["mode"] != "deep" {
			t.Fatalf("alias %q did not bind active local generation %q: %+v %v", alias, version, resolved, err)
		}
		if err := run([]string{alias, "--mode=deep"}); err != nil {
			t.Fatalf("actual CLI invocation %q: %v", alias, err)
		}
		var help bytes.Buffer
		if handled, err := dispatchCLIHelp([]string{"help", alias}, &help); !handled || err != nil || !strings.Contains(help.String(), alias) {
			t.Fatalf("installed help missing %q: %s %v", alias, help.String(), err)
		}
	}
	assertAlias("probe-one", "1")
	if err := run([]string{"help"}); err != nil {
		t.Fatalf("scratch CLI command discovery: %v", err)
	}
	if err := run([]string{"help", "probe-one"}); err != nil {
		t.Fatalf("scratch CLI installed-command help: %v", err)
	}
	trustedCollision := localCLIPackage(t, packagesDir, "probe/collision", "1", "probe-one")
	t.Setenv("PRAXIS_TRUSTED_KEYS", trustedCollision)
	collisionApproval := localCLIApproval(t, getenv, root, "local:probe/collision@1")
	t.Setenv("PRAXIS_PACKAGE_APPROVAL_ID", collisionApproval)
	if err := run([]string{"install", "local:probe/collision@1"}); err == nil {
		t.Fatal("scratch CLI accepted alias collision with active package")
	}
	collisionDB, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := collisionDB.QueryRow(`SELECT remaining_uses FROM approvals WHERE approval_id=?`, collisionApproval).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("alias collision consumed governed approval: %d %v", remaining, err)
	}
	if _, err := state.New(collisionDB).ActivePackage(ctx, "probe/collision"); err == nil {
		t.Fatal("alias collision left an active package")
	}
	if err := collisionDB.Close(); err != nil {
		t.Fatal(err)
	}
	assertAlias("probe-one", "1")
	if err := run([]string{"update", "probe/dynamic"}); err == nil || !strings.Contains(err.Error(), "--to") {
		t.Fatalf("local update without pinned target did not fail: %v", err)
	}
	trustedV2 := localCLIPackage(t, packagesDir, "probe/dynamic", "2", "probe-two")
	t.Setenv("PRAXIS_TRUSTED_KEYS", trustedV2)
	t.Setenv("PRAXIS_PACKAGE_APPROVAL_ID", localCLIApproval(t, getenv, root, "local:probe/dynamic@2"))
	if err := run([]string{"update", "probe/dynamic", "--to", "local:other/package@2"}); err == nil {
		t.Fatal("local update accepted a different package ID")
	}
	if err := run([]string{"update", "probe/dynamic", "--to", "local:probe/dynamic@2"}); err == nil || !strings.Contains(err.Error(), "--accept-permission-changes") {
		t.Fatalf("local update added capability without explicit review: %v", err)
	}
	if err := run([]string{"update", "probe/dynamic", "--to", "local:probe/dynamic@2", "--accept-permission-changes"}); err != nil {
		t.Fatalf("scratch local CLI update: %v", err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"probe-one"}, os.Getenv); err == nil {
		t.Fatal("old alias survived active generation replacement")
	}
	assertAlias("probe-two", "2")
	db, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	active, err := state.New(db).ActivePackage(ctx, "probe/dynamic")
	if err != nil {
		t.Fatal(err)
	}
	if active.SourceKind != distribution.SourceLocalFirstParty || active.Manifest.Version != "2" {
		t.Fatalf("local source lost after CLI update: %+v", active)
	}
	var receiptsBefore int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM package_activation_receipts WHERE package_id=?`, "probe/dynamic").Scan(&receiptsBefore); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"update", "probe/dynamic", "--to", "local:probe/dynamic@1", "--accept-permission-changes"}); err == nil || !strings.Contains(err.Error(), "use rollback") {
		t.Fatalf("historical local generation accepted through update: %v", err)
	}
	var receiptsAfter int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM package_activation_receipts WHERE package_id=?`, "probe/dynamic").Scan(&receiptsAfter); err != nil || receiptsAfter != receiptsBefore {
		t.Fatalf("rejected historical update changed activation receipts: before=%d after=%d err=%v", receiptsBefore, receiptsAfter, err)
	}
	active, err = state.New(db).ActivePackage(ctx, "probe/dynamic")
	if err != nil || active.Manifest.Version != "2" {
		t.Fatalf("rejected historical update changed active generation: %+v %v", active, err)
	}
	assertAlias("probe-two", "2")
	t.Setenv("PRAXIS_AUTHORITY_ID", "scratch-operator")
	t.Setenv("PRAXIS_AUTHORITY_KIND", "user")
	t.Setenv("PRAXIS_PACKAGE_APPROVAL_ID", localCLITransitionApproval(t, db, active.Manifest, packagecatalog.TransitionDisable))
	if err := run([]string{"disable", "probe/dynamic"}); err != nil {
		t.Fatalf("scratch CLI disable: %v", err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"probe-two"}, os.Getenv); err == nil {
		t.Fatal("disabled alias survived restart")
	}
	selected, err := state.New(db).SelectedPackage(ctx, "probe/dynamic")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAXIS_PACKAGE_APPROVAL_ID", localCLITransitionApproval(t, db, selected.Manifest, packagecatalog.TransitionRemove))
	if err := run([]string{"uninstall", "probe/dynamic"}); err != nil {
		t.Fatalf("scratch CLI uninstall: %v", err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"probe-two"}, os.Getenv); err == nil {
		t.Fatal("removed alias survived restart")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLocalCLIUpdateRequiresExplicitPinnedTarget(t *testing.T) {
	for _, args := range [][]string{{"probe/dynamic", "--to"}, {"probe/dynamic", "--to", ""}, {"probe/dynamic", "--to", "local:probe/dynamic@2", "--to", "local:probe/dynamic@3"}} {
		if _, _, _, _, err := parseUpdateArgs(args); err == nil {
			t.Fatalf("accepted ambiguous update args: %v", args)
		}
	}
	if id, target, accept, fallback, err := parseUpdateArgs([]string{"probe/dynamic", "--to", "local:probe/dynamic@2", "--accept-permission-changes", "--allow-classical-signature-fallback"}); err != nil || id != "probe/dynamic" || target != "local:probe/dynamic@2" || !accept || !fallback {
		t.Fatalf("pinned update parse: %q %q %v %v %v", id, target, accept, fallback, err)
	}
}
