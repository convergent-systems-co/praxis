package packagecatalog

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func TestVerifySignatureEd25519(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	envelope := SignatureEnvelope{
		Version: SignatureEnvelopeVersion, Profile: contracts.CryptoClassicalCompatible,
		Algorithm: SignatureAlgorithmEd25519, KeyID: "publisher-1",
		ManifestDigest: "sha256:manifest", ArtifactDigest: "sha256:artifact",
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, envelope.Statement()))
	if err := VerifySignature(envelope, map[string]ed25519.PublicKey{"publisher-1": pub}, false); err != nil { t.Fatal(err) }
	envelope.ArtifactDigest = "sha256:changed"
	if err := VerifySignature(envelope, map[string]ed25519.PublicKey{"publisher-1": pub}, false); err == nil { t.Fatal("changed signed digest must fail verification") }
}

func TestVerifySignaturePQRequiredFailsClosedWithoutPQVerifier(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	envelope := SignatureEnvelope{
		Version: SignatureEnvelopeVersion, Profile: contracts.CryptoPQRequired,
		Algorithm: SignatureAlgorithmEd25519, KeyID: "publisher-1",
		ManifestDigest: "sha256:manifest", ArtifactDigest: "sha256:artifact",
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, envelope.Statement()))
	if err := VerifySignature(envelope, map[string]ed25519.PublicKey{"publisher-1": pub}, true); err == nil { t.Fatal("pq-required package signature must not downgrade to Ed25519") }
}

func TestVerifySignaturePQPreferredRequiresExplicitFallback(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	envelope := SignatureEnvelope{
		Version: SignatureEnvelopeVersion, Profile: contracts.CryptoPQPreferred,
		Algorithm: SignatureAlgorithmEd25519, KeyID: "publisher-1",
		ManifestDigest: "sha256:manifest", ArtifactDigest: "sha256:artifact",
	}
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, envelope.Statement()))
	keys := map[string]ed25519.PublicKey{"publisher-1": pub}
	if err := VerifySignature(envelope, keys, false); err == nil { t.Fatal("pq-preferred fallback must be explicit") }
	if err := VerifySignature(envelope, keys, true); err != nil { t.Fatalf("explicit classical fallback should verify: %v", err) }
}
