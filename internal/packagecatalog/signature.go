package packagecatalog

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const (
	SignatureEnvelopeVersion = "v1"
	SignatureAlgorithmEd25519 = "ed25519"
)

// SignatureEnvelope authenticates the immutable manifest bytes and package
// artifact digest. Verification proves integrity/provenance against a locally
// trusted publisher key; it never grants package capabilities.
type SignatureEnvelope struct {
	Version        string                  `json:"version"`
	Profile        contracts.CryptoProfile `json:"profile"`
	Algorithm      string                  `json:"algorithm"`
	KeyID          string                  `json:"key_id"`
	ManifestDigest string                  `json:"manifest_digest"`
	ArtifactDigest string                  `json:"artifact_digest"`
	Signature      string                  `json:"signature"`
}

func (e SignatureEnvelope) Validate() error {
	if e.Version != SignatureEnvelopeVersion {
		return fmt.Errorf("unsupported package signature envelope version %q", e.Version)
	}
	if err := e.Profile.Validate(); err != nil {
		return err
	}
	if e.Algorithm == "" || e.KeyID == "" || e.ManifestDigest == "" || e.ArtifactDigest == "" || e.Signature == "" {
		return errors.New("package signature algorithm, key id, manifest digest, artifact digest, and signature are required")
	}
	if !strings.HasPrefix(e.ManifestDigest, "sha256:") || !strings.HasPrefix(e.ArtifactDigest, "sha256:") {
		return errors.New("package signature digests must use sha256")
	}
	return nil
}

func (e SignatureEnvelope) Statement() []byte {
	return []byte("praxis-package-signature-v1\n" + e.ManifestDigest + "\n" + e.ArtifactDigest + "\n")
}

// VerifySignature verifies an envelope using the currently available signing
// algorithms. PQ profiles fail closed until a configured verifier actually
// provides a standardized PQ signature implementation.
func VerifySignature(envelope SignatureEnvelope, trustedKeys map[string]ed25519.PublicKey, allowPQPreferredFallback bool) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	caps := praxiscrypto.Capabilities{Classical: true, PQ: false, Hybrid: false}
	resolution, err := praxiscrypto.Resolve(envelope.Profile, caps, allowPQPreferredFallback)
	if err != nil {
		return fmt.Errorf("resolve package signature profile: %w", err)
	}
	if resolution.Selected != contracts.CryptoClassicalCompatible {
		return fmt.Errorf("package signature profile %q resolved to unsupported verifier %q", envelope.Profile, resolution.Selected)
	}
	if envelope.Algorithm != SignatureAlgorithmEd25519 {
		return fmt.Errorf("unsupported package signature algorithm %q", envelope.Algorithm)
	}
	key, ok := trustedKeys[envelope.KeyID]
	if !ok {
		return fmt.Errorf("package signature key %q is not locally trusted", envelope.KeyID)
	}
	sig, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode package signature: %w", err)
	}
	if len(key) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519 package signature key or signature size")
	}
	if !ed25519.Verify(key, envelope.Statement(), sig) {
		return errors.New("package signature verification failed")
	}
	return nil
}
