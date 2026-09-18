package main

import (
	"context"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestInstallReverifiesAtApprovedVerificationTime proves that a deployment
// approval, which binds verification evidence produced at intent-preview
// time, is honoured by a later install: install re-verifies the exact bytes
// as of the approved instant and reproduces the approved intent identity,
// whereas verifying at the wall-clock install time would never match.
func TestInstallReverifiesAtApprovedVerificationTime(t *testing.T) {
	ctx := context.Background()
	getenv, root := governedInstallationFixture(t, ctx)
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "acme/tool", Version: "1"}
	release, artifact, trusted := signedManifestRelease(t, manifest, []byte("acme-tool-archive"), contracts.CryptoClassicalCompatible)
	const approvalID = "package-approval:reverification"
	env := func(key string) string {
		switch key {
		case "PRAXIS_TRUSTED_KEYS":
			return trusted
		case "PRAXIS_PACKAGE_APPROVAL_ID":
			return approvalID
		}
		return getenv(key)
	}
	adapter := distribution.GitHubReleases{}
	previewAt := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)
	installAt := previewAt.Add(45 * time.Minute)

	// Intent preview: verify, build the deployment intent, persist evidence.
	resolution, err := resolveReleasePackages(ctx, adapter, release, artifact, env, false, previewAt)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := packageDeploymentRequest(resolution, env)
	if err != nil {
		t.Fatal(err)
	}
	db, err := state.OpenSQLite(ctx, getenv("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	evidenceID, err := persistDeploymentEvidence(ctx, db, approved, previewAt, env)
	if err != nil {
		t.Fatal(err)
	}
	approved.Intent.Parameters["verification_evidence_digest"] = evidenceID
	approvedDigest, err := approved.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	repo, authDB, err := openGovernedRepository(ctx, getenv)
	if err != nil {
		t.Fatal(err)
	}
	request := contracts.AuthorityRequest{ID: "package-deploy-request:" + approvedDigest, Version: "1", RequestedAuthority: contracts.GovernedPackageDeploy, RequestedScope: root.Scope, Reason: "deploy exact verified package closure", Status: contracts.AuthorityRequestPending, IntentDigest: approvedDigest, InstallationDigest: root.Digest, ClosureDigest: approved.Intent.Parameters["closure_digest"], VerificationEvidenceDigest: evidenceID, Intent: &approved.Intent}
	if _, err := repo.SaveAuthorityRequest(ctx, request, previewAt, nil); err != nil {
		t.Fatal(err)
	}
	authDB.Close()
	manager := contracts.PackageManagerPrincipal()
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses,version,authority_request_id,authority_request_version) VALUES(?,?,?,?,?,1,1,?,?)`, approvalID, manager.ID, manager.Kind, approvedDigest, previewAt.Format(time.RFC3339Nano), request.ID, request.Version); err != nil {
		t.Fatal(err)
	}

	// Wall-clock re-verification never reproduces the approved identity.
	later, err := resolveReleasePackages(ctx, adapter, release, artifact, env, false, installAt)
	if err != nil {
		t.Fatal(err)
	}
	drifted, err := packageDeploymentRequest(later, env)
	if err != nil {
		t.Fatal(err)
	}
	driftedDigest, _ := drifted.Intent.Digest()
	if driftedDigest == approvedDigest {
		t.Fatal("fixture does not exercise time-bound verification identity")
	}

	// Install resolves the approved instant and reproduces the exact intent.
	at, err := approvedVerificationTime(ctx, db, env, installAt)
	if err != nil {
		t.Fatalf("approved verification time must resolve from durable state: %v", err)
	}
	if !at.Equal(previewAt) {
		t.Fatalf("approved verification time %s, want %s", at, previewAt)
	}
	again, err := resolveReleasePackages(ctx, adapter, release, artifact, env, false, at)
	if err != nil {
		t.Fatal(err)
	}
	reproduced, err := packageDeploymentRequest(again, env)
	if err != nil {
		t.Fatal(err)
	}
	reproducedEvidence, err := persistDeploymentEvidence(ctx, db, reproduced, at, env)
	if err != nil {
		t.Fatal(err)
	}
	reproduced.Intent.Parameters["verification_evidence_digest"] = reproducedEvidence
	if reproducedEvidence != evidenceID {
		t.Fatalf("re-verification evidence %s, want approved %s", reproducedEvidence, evidenceID)
	}
	if digest, _ := reproduced.Intent.Digest(); digest != approvedDigest {
		t.Fatalf("re-verified intent %s, want approved %s", digest, approvedDigest)
	}

	// Different bytes under the same approval still fail closed.
	other, otherArtifact, _ := signedManifestRelease(t, manifest, []byte("acme-tool-archive-tampered"), contracts.CryptoClassicalCompatible)
	_ = other
	if _, err := resolveReleasePackages(ctx, adapter, release, otherArtifact, env, false, at); err == nil {
		t.Fatal("tampered artifact verified under the approved manifest")
	}
	if _, err := approvedVerificationTime(ctx, db, func(key string) string {
		if key == "PRAXIS_PACKAGE_APPROVAL_ID" {
			return "package-approval:unknown"
		}
		return env(key)
	}, installAt); err == nil {
		t.Fatal("unknown approval resolved a verification time")
	}
}
