package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func signedFixtureRelease(t *testing.T, profile contracts.CryptoProfile) (distribution.Release, []byte, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	artifact := []byte("package-payload")
	sum := sha256.Sum256(artifact)
	artifactDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifest := packagecatalog.Manifest{PackageID: "acme/pkg", Version: "1", ContentDigest: artifactDigest}
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
		ManifestDigest: manifestDigest,
		Manifest:       manifest, ManifestBytes: manifestBytes,
		Signature: envelope,
	}, artifact, string(keysJSON)
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
