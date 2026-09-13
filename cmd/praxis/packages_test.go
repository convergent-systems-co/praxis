package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func signedFixtureRelease(t *testing.T, profile contracts.CryptoProfile) (distribution.Release, []byte, string) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	artifact := []byte("package-payload")
	sum := sha256.Sum256(artifact)
	artifactDigest := "sha256:" + hex.EncodeToString(sum[:])
	manifestDigest := "sha256:manifest"
	envelope := packagecatalog.SignatureEnvelope{
		Version: packagecatalog.SignatureEnvelopeVersion,
		Profile: profile,
		ManifestDigest: manifestDigest,
		ArtifactDigest: artifactDigest,
		Proofs: []packagecatalog.SignatureProof{{
			Algorithm: packagecatalog.SignatureAlgorithmEd25519,
			KeyID: "publisher-1",
		}},
	}
	envelope.Proofs[0].Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, envelope.Statement()))
	keysJSON, err := json.Marshal(map[string]string{"publisher-1": base64.StdEncoding.EncodeToString(pub)})
	if err != nil { t.Fatal(err) }
	return distribution.Release{
		ManifestDigest: manifestDigest,
		Manifest: packagecatalog.Manifest{PackageID: "acme/pkg", Version: "1", ContentDigest: artifactDigest},
		Signature: envelope,
	}, artifact, string(keysJSON)
}

func TestVerifyReleasePackageRequiresLocallyTrustedSignature(t *testing.T) {
	release, artifact, trusted := signedFixtureRelease(t, contracts.CryptoClassicalCompatible)
	getenv := func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" { return trusted }
		return ""
	}
	if err := verifyReleasePackage(release, artifact, getenv, false); err != nil { t.Fatal(err) }
	if err := verifyReleasePackage(release, artifact, func(string) string { return "" }, false); err == nil { t.Fatal("missing local publisher trust must fail installation") }
	artifact[0] ^= 1
	if err := verifyReleasePackage(release, artifact, getenv, false); err == nil { t.Fatal("tampered package artifact must fail") }
}

func TestVerifyReleasePackagePQPreferredFallbackIsExplicit(t *testing.T) {
	release, artifact, trusted := signedFixtureRelease(t, contracts.CryptoPQPreferred)
	getenv := func(key string) string { if key == "PRAXIS_TRUSTED_KEYS" { return trusted }; return "" }
	if err := verifyReleasePackage(release, artifact, getenv, false); err == nil { t.Fatal("pq-preferred package must not silently fall back") }
	if err := verifyReleasePackage(release, artifact, getenv, true); err != nil { t.Fatalf("explicit fallback should succeed: %v", err) }
}

func TestInstallArgsRequireExplicitFallbackFlag(t *testing.T) {
	ref, allow, err := parseInstallArgs([]string{"acme/pkg", "--allow-classical-signature-fallback"})
	if err != nil || ref != "acme/pkg" || !allow { t.Fatalf("unexpected install parse: %q %v %v", ref, allow, err) }
	if _, _, err := parseInstallArgs([]string{"acme/pkg", "--anything"}); err == nil { t.Fatal("unknown install option must fail") }
}
