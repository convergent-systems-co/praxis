package goalstore

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func verifiedPackageFixture(t *testing.T, verifiedAt time.Time) packagecatalog.VerifiedPackage {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("acme-tool-archive")
	artifactSum := sha256.Sum256(artifact)
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "acme/tool", Version: "1", ContentDigest: "sha256:" + hex.EncodeToString(artifactSum[:])}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestSum := sha256.Sum256(manifestBytes)
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: "sha256:" + hex.EncodeToString(manifestSum[:]), ArtifactDigest: manifest.ContentDigest, Proofs: []packagecatalog.SignatureProof{{Algorithm: packagecatalog.SignatureAlgorithmEd25519, KeyID: "publisher-1"}}}
	envelope.Proofs[0].Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, envelope.Statement()))
	verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{ManifestBytes: manifestBytes, ArtifactBytes: artifact, Signature: envelope, SourceKind: "github-release", SourceRef: "acme/tool@v1", VerifiedAt: verifiedAt}, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"publisher-1": public}}})
	if err != nil {
		t.Fatal(err)
	}
	return verified
}

// TestDerivedPackageApprovalIsConsumableByDeployment walks the governed
// deployment lineage exactly as the CLI does (intent, durable verification
// evidence, package-deploy request, owner decision bound to the
// package-manager generation, derived approval) and proves the derived
// approval is consumable by the deployment itself, including its expiry.
func TestDerivedPackageApprovalIsConsumableByDeployment(t *testing.T) {
	repo, root, manager, now := packageDeployDelegationFixture(t)
	ctx := context.Background()
	activateAuthorityModelRecordForTest(t, repo, "active-authority-model-v6", contracts.AuthorityModelRoutingVersion, contracts.AuthorityModelRoutingDigest(), now)
	verifiedAt := now.Add(time.Second)
	verified := verifiedPackageFixture(t, verifiedAt)
	deployment, err := packagecatalog.NewDeploymentRequest(verified, nil, contracts.PackageManagerPrincipal(), "pending")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := packagecatalog.NewVerificationEvidenceRecord(deployment.Root, deployment.Packages, root.Digest, verifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Store.SaveVerificationEvidence(ctx, evidence); err != nil {
		t.Fatal(err)
	}
	deployment.VerificationEvidenceDigest = evidence.ID
	deployment.Intent.Parameters["verification_evidence_digest"] = evidence.ID
	at := now.Add(2 * time.Second)
	request, requestDigest, err := repo.SavePackageDeploymentRequest(ctx, deployment.Intent, root.Digest, deployment.Intent.Parameters["closure_digest"], evidence.ID, at)
	if err != nil {
		t.Fatalf("package-deploy request under v6 must persist: %v", err)
	}
	expires := at.Add(24 * time.Hour)
	if manager.ExpiresAt != nil && manager.ExpiresAt.Before(expires) {
		expires = manager.ExpiresAt.UTC()
	}
	decision := contracts.AuthorityDecision{RequestID: request.ID, RequestVersion: request.Version, RequestDigest: requestDigest, DecisionRef: "authority-decision:" + request.ID, DecisionVersion: "1", DecidedBy: root.Principal, AuthorityRef: root.Ref, AuthorityVersion: root.Version, AuthorityGenerationDigest: root.Digest, GrantedScope: root.Scope, Outcome: contracts.AuthorityApprove, AuthorityDigest: root.AuthorityModelDigest, IssuedAt: at, ExpiresAt: &expires, OperationalAuthorityRef: manager.Ref, OperationalAuthorityVersion: manager.Version, OperationalAuthorityGenerationDigest: manager.Digest}
	if err := repo.SaveAuthorityDecision(ctx, request.ID, request.Version, decision, at, &expires); err != nil {
		t.Fatal(err)
	}
	approvalID, err := repo.DerivePackageDeploymentApproval(ctx, request.ID, request.Version, deployment.Intent, at)
	if err != nil {
		t.Fatal(err)
	}
	again, err := repo.DerivePackageDeploymentApproval(ctx, request.ID, request.Version, deployment.Intent, at.Add(time.Second))
	if err != nil || again != approvalID {
		t.Fatalf("derived approval must be stable and idempotent: %v %s %s", err, again, approvalID)
	}
	var expiresText string
	if err := repo.Store.DB().QueryRowContext(ctx, `SELECT expires_at FROM approvals WHERE approval_id=?`, approvalID).Scan(&expiresText); err != nil {
		t.Fatal(err)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, expiresText); err != nil || !parsed.Equal(expires) {
		t.Fatalf("derived approval expiry %q is not canonical RFC3339Nano for %s: %v", expiresText, expires, err)
	}
	deployment.ApprovalID = approvalID
	if err := repo.Store.DeployPackages(ctx, deployment, at.Add(2*time.Second)); err != nil {
		t.Fatalf("deployment must consume the derived approval: %v", err)
	}
	if err := repo.Store.DeployPackages(ctx, deployment, at.Add(3*time.Second)); err == nil {
		t.Fatal("single-use approval consumed twice")
	}
	invocations, err := repo.Store.ActiveInvocations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = invocations
	if _, err := repo.Store.ActivePackage(ctx, "acme/tool"); err != nil {
		t.Fatalf("package must be active after governed deployment: %v", err)
	}
}
