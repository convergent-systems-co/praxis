package packagecatalog

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type fakePQVerifier struct{}

func (fakePQVerifier) Algorithm() string { return "ml-dsa-fixture" }
func (fakePQVerifier) Class() SignatureClass { return SignatureClassPQ }
func (fakePQVerifier) Verify(proof SignatureProof, statement []byte) error {
	if proof.KeyID != "pq-publisher" || proof.Signature != base64.StdEncoding.EncodeToString(statement) {
		return errors.New("invalid fixture PQ proof")
	}
	return nil
}

func signedEd25519Envelope(t *testing.T, profile contracts.CryptoProfile) (SignatureEnvelope, Ed25519Verifier) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	envelope := SignatureEnvelope{
		Version: SignatureEnvelopeVersion, Profile: profile,
		ManifestDigest: "sha256:manifest", ArtifactDigest: "sha256:artifact",
	}
	proof := SignatureProof{Algorithm: SignatureAlgorithmEd25519, KeyID: "publisher-1"}
	proof.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, envelope.Statement()))
	envelope.Proofs = []SignatureProof{proof}
	return envelope, Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{"publisher-1": pub}}
}

func TestVerifySignatureEd25519(t *testing.T) {
	envelope, verifier := signedEd25519Envelope(t, contracts.CryptoClassicalCompatible)
	if err := VerifySignature(envelope, []SignatureVerifier{verifier}, false); err != nil { t.Fatal(err) }
	envelope.ArtifactDigest = "sha256:changed"
	if err := VerifySignature(envelope, []SignatureVerifier{verifier}, false); err == nil { t.Fatal("changed signed digest must fail verification") }
}

func TestVerifySignaturePQRequiredFailsClosedWithoutPQVerifier(t *testing.T) {
	envelope, verifier := signedEd25519Envelope(t, contracts.CryptoPQRequired)
	if err := VerifySignature(envelope, []SignatureVerifier{verifier}, true); err == nil { t.Fatal("pq-required package signature must not downgrade to Ed25519") }
}

func TestVerifySignaturePQPreferredRequiresExplicitFallback(t *testing.T) {
	envelope, verifier := signedEd25519Envelope(t, contracts.CryptoPQPreferred)
	if err := VerifySignature(envelope, []SignatureVerifier{verifier}, false); err == nil { t.Fatal("pq-preferred fallback must be explicit") }
	if err := VerifySignature(envelope, []SignatureVerifier{verifier}, true); err != nil { t.Fatalf("explicit classical fallback should verify: %v", err) }
}

func TestVerifySignaturePQAndHybridCanBeSatisfiedByProvider(t *testing.T) {
	envelope, classical := signedEd25519Envelope(t, contracts.CryptoPQRequired)
	pq := SignatureProof{Algorithm: "ml-dsa-fixture", KeyID: "pq-publisher"}
	pq.Signature = base64.StdEncoding.EncodeToString(envelope.Statement())
	envelope.Proofs = append(envelope.Proofs, pq)
	if err := VerifySignature(envelope, []SignatureVerifier{classical, fakePQVerifier{}}, false); err != nil { t.Fatalf("PQ provider should satisfy pq-required: %v", err) }

	envelope.Profile = contracts.CryptoHybridHighAssurance
	// The signed statement excludes the profile so profile policy can require a stronger
	// proof set without changing immutable package identity/digests.
	if err := VerifySignature(envelope, []SignatureVerifier{classical, fakePQVerifier{}}, false); err != nil { t.Fatalf("classical + PQ proofs should satisfy hybrid: %v", err) }
}
