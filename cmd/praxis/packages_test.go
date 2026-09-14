package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func signedFixtureRelease(t *testing.T, profile contracts.CryptoProfile) (distribution.Release, []byte, string) {
	t.Helper()
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "acme/pkg", Version: "1"}
	return signedManifestRelease(t, manifest, []byte("package-payload"), profile)
}

func signedManifestRelease(t *testing.T, manifest packagecatalog.Manifest, artifact []byte, profile contracts.CryptoProfile) (distribution.Release, []byte, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(artifact)
	artifactDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifest.ContentDigest = artifactDigest
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestSum := sha256.Sum256(manifestBytes)
	manifestDigest := "sha256:" + hex.EncodeToString(manifestSum[:])
	envelope := packagecatalog.SignatureEnvelope{
		Version:        packagecatalog.SignatureEnvelopeCurrentVersion(),
		Profile:        profile,
		ManifestDigest: manifestDigest,
		ArtifactDigest: artifactDigest,
		Proofs: []packagecatalog.SignatureProof{{
			Algorithm: packagecatalog.SignatureAlgorithmEd25519,
			KeyID:     "publisher-1",
		}},
	}
	envelope.Proofs[0].Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, envelope.Statement()))
	keysJSON, err := json.Marshal(map[string]string{"publisher-1": base64.StdEncoding.EncodeToString(pub)})
	if err != nil {
		t.Fatal(err)
	}
	return distribution.Release{
		Ref:            distribution.PackageRef{Source: "github-releases", Owner: "acme", Repo: "pkg"},
		ManifestDigest: manifestDigest,
		Manifest:       manifest, ManifestBytes: manifestBytes,
		Signature: envelope,
	}, artifact, string(keysJSON)
}

func activateDynamicFixture(t *testing.T, ctx context.Context, db *sql.DB, manifest packagecatalog.Manifest, alias string, now time.Time) packagecatalog.Manifest {
	t.Helper()
	manifest.Invocations = []contracts.InvocationContract{{Version: contracts.InvocationContractCurrentVersion(), PackageID: manifest.PackageID, PackageVersion: manifest.Version, GraphID: manifest.PackageID + ".graph", GraphVersion: manifest.Version, EntryPointID: manifest.PackageID + ".run", Aliases: []string{alias}, Options: []contracts.InvocationOption{{Name: "mode", Type: "string", Default: "safe"}}}}
	release, artifact, trusted := signedManifestRelease(t, manifest, []byte("package:"+manifest.PackageID+"@"+manifest.Version), contracts.CryptoClassicalCompatible)
	verified, err := verifyReleasePackage(release, artifact, func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" {
			return trusted
		}
		return ""
	}, false, now)
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "operator", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(verified, actor)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	approvalID := "approval:" + manifest.Version
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := state.New(db).ActivatePackage(ctx, packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: approvalID}, now); err != nil {
		t.Fatal(err)
	}
	return release.Manifest
}

func TestDynamicInvocationFollowsExactActiveGenerationAcrossRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 13, 22, 0, 0, 0, time.UTC)
	v1 := activateDynamicFixture(t, ctx, db, packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "research/dynamic", Version: "1"}, "investigate", now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "PRAXIS_DB" {
			return path
		}
		return ""
	}
	resolved, err := resolveDynamicInvocation(ctx, []string{"investigate", "topic", "--mode=deep"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.PackageID != v1.PackageID || resolved.PackageVersion != "1" || resolved.PackageDigest != v1.ContentDigest || resolved.GraphVersion != "1" || resolved.Options["mode"] != "deep" {
		t.Fatalf("dynamic client lost exact active generation: %+v", resolved)
	}

	db, err = state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v2 := activateDynamicFixture(t, ctx, db, packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "research/dynamic", Version: "2"}, "investigate-v2", now.Add(time.Minute))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"investigate"}, getenv); err == nil {
		t.Fatal("stale projected alias resolved after package generation changed")
	}
	resolved, err = resolveDynamicInvocation(ctx, []string{"investigate-v2"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.PackageVersion != "2" || resolved.PackageDigest != v2.ContentDigest || resolved.GraphVersion != "2" {
		t.Fatalf("updated command did not atomically follow v2: %+v", resolved)
	}

	db, err = state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "operator", Kind: "user"}
	transition, err := packagecatalog.NewTransitionRequest(packagecatalog.PackageIdentity{PackageID: v2.PackageID, Version: v2.Version, ContentDigest: v2.ContentDigest}, packagecatalog.TransitionDisable, actor, "approval:disable")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := transition.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, transition.ApprovalID, actor.ID, actor.Kind, digest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := state.New(db).TransitionPackage(ctx, transition, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"investigate-v2"}, getenv); err == nil {
		t.Fatal("disabled package command resolved after restart")
	}
}

func TestVerifyReleasePackageRequiresLocallyTrustedSignature(t *testing.T) {
	release, artifact, trusted := signedFixtureRelease(t, contracts.CryptoClassicalCompatible)
	getenv := func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" {
			return trusted
		}
		return ""
	}
	verified, err := verifyReleasePackage(release, artifact, getenv, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := verified.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyReleasePackage(release, artifact, func(string) string { return "" }, false, time.Now().UTC()); err == nil {
		t.Fatal("missing local publisher trust must fail installation")
	}
	artifact[0] ^= 1
	if _, err := verifyReleasePackage(release, artifact, getenv, false, time.Now().UTC()); err == nil {
		t.Fatal("tampered package artifact must fail")
	}
}

func TestVerifyReleasePackagePQPreferredFallbackIsExplicit(t *testing.T) {
	release, artifact, trusted := signedFixtureRelease(t, contracts.CryptoPQPreferred)
	getenv := func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" {
			return trusted
		}
		return ""
	}
	if _, err := verifyReleasePackage(release, artifact, getenv, false, time.Now().UTC()); err == nil {
		t.Fatal("pq-preferred package must not silently fall back")
	}
	if _, err := verifyReleasePackage(release, artifact, getenv, true, time.Now().UTC()); err != nil {
		t.Fatalf("explicit fallback should succeed: %v", err)
	}
}

func TestVerificationCannotMintActivationAuthority(t *testing.T) {
	release, artifact, trusted := signedFixtureRelease(t, contracts.CryptoClassicalCompatible)
	verified, err := verifyReleasePackage(release, artifact, func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" {
			return trusted
		}
		return ""
	}, false, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := packageActivationRequest(verified, func(string) string { return "" }); err == nil {
		t.Fatal("verified package without local approval binding must not become activatable")
	}
}

func TestInstallArgsRequireExplicitFallbackFlag(t *testing.T) {
	ref, allow, err := parseInstallArgs([]string{"acme/pkg", "--allow-classical-signature-fallback"})
	if err != nil || ref != "acme/pkg" || !allow {
		t.Fatalf("unexpected install parse: %q %v %v", ref, allow, err)
	}
	if _, _, err := parseInstallArgs([]string{"acme/pkg", "--anything"}); err == nil {
		t.Fatal("unknown install option must fail")
	}
}

func TestPackageTransitionRequestBindsExternalAuthorityAndExactGeneration(t *testing.T) {
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "research/pkg", Version: "2", ContentDigest: "sha256:generation"}
	getenv := func(key string) string {
		switch key {
		case "PRAXIS_PACKAGE_APPROVAL_ID":
			return "approval-remove"
		case "PRAXIS_AUTHORITY_ID":
			return "operator"
		case "PRAXIS_AUTHORITY_KIND":
			return "user"
		default:
			return ""
		}
	}
	request, err := packageTransitionRequest(manifest, packagecatalog.TransitionRemove, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if request.Identity.PackageID != manifest.PackageID || request.Identity.Version != manifest.Version || request.Identity.ContentDigest != manifest.ContentDigest || request.Intent.Actor.ID != "operator" || request.ApprovalID != "approval-remove" {
		t.Fatalf("transition request lost exact authority/generation binding: %#v", request)
	}
	if _, err := packageTransitionRequest(manifest, packagecatalog.TransitionRemove, func(string) string { return "" }); err == nil {
		t.Fatal("package removal must not infer local authority from the requested operation")
	}
}
