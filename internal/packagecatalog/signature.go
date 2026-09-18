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

const SignatureAlgorithmEd25519 = "ed25519"

type SignatureClass string

const (
	SignatureClassClassical SignatureClass = "classical"
	SignatureClassPQ        SignatureClass = "post_quantum"
)

type SignatureProof struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Signature string `json:"signature"`
}

// SignatureEnvelope authenticates immutable manifest bytes and the package
// artifact digest. Multiple proofs permit hybrid profiles without changing the
// package identity contract.
type SignatureEnvelope struct {
	Version        string                  `json:"version"`
	Profile        contracts.CryptoProfile `json:"profile"`
	ManifestDigest string                  `json:"manifest_digest"`
	ArtifactDigest string                  `json:"artifact_digest"`
	Proofs         []SignatureProof        `json:"proofs"`
}

func (e SignatureEnvelope) Validate() error {
	if err := requirePackageContractVersion(signatureEnvelopeVersions, e.Version); err != nil {
		return err
	}
	if err := e.Profile.Validate(); err != nil {
		return err
	}
	if e.ManifestDigest == "" || e.ArtifactDigest == "" || len(e.Proofs) == 0 {
		return errors.New("package signature manifest digest, artifact digest, and proofs are required")
	}
	if !strings.HasPrefix(e.ManifestDigest, "sha256:") || !strings.HasPrefix(e.ArtifactDigest, "sha256:") {
		return errors.New("package signature digests must use sha256")
	}
	seen := map[string]struct{}{}
	for _, proof := range e.Proofs {
		if proof.Algorithm == "" || proof.KeyID == "" || proof.Signature == "" {
			return errors.New("package signature proof algorithm, key id, and signature are required")
		}
		key := proof.Algorithm + "\x00" + proof.KeyID
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate signature proof %s/%s", proof.Algorithm, proof.KeyID)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (e SignatureEnvelope) Statement() []byte {
	return []byte("praxis-package-signature-" + e.Version + "\n" + e.ManifestDigest + "\n" + e.ArtifactDigest + "\n")
}

// SignatureVerifier is implemented by cryptographic providers. A provider
// advertises one algorithm and whether that algorithm is classical or PQ.
// Verification and local publisher trust are provider responsibilities.
type SignatureVerifier interface {
	Algorithm() string
	Class() SignatureClass
	Verify(proof SignatureProof, statement []byte) error
}

type Ed25519Verifier struct {
	TrustedKeys map[string]ed25519.PublicKey
}

func (Ed25519Verifier) Algorithm() string     { return SignatureAlgorithmEd25519 }
func (Ed25519Verifier) Class() SignatureClass { return SignatureClassClassical }
func (v Ed25519Verifier) Verify(proof SignatureProof, statement []byte) error {
	key, ok := v.TrustedKeys[proof.KeyID]
	if !ok {
		return fmt.Errorf("package signature key %q is not locally trusted", proof.KeyID)
	}
	sig, err := base64.StdEncoding.DecodeString(proof.Signature)
	if err != nil {
		return fmt.Errorf("decode package signature: %w", err)
	}
	if len(key) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return errors.New("invalid Ed25519 package signature key or signature size")
	}
	if !ed25519.Verify(key, statement, sig) {
		return errors.New("package signature verification failed")
	}
	return nil
}

// VerifySignature resolves the requested cryptographic profile against actual
// verifier capabilities and validates the required proof set. A future ML-DSA
// provider can satisfy PQ/hybrid profiles without changing this contract.
func VerifySignature(envelope SignatureEnvelope, verifiers []SignatureVerifier, allowPQPreferredFallback bool) error {
	if err := envelope.Validate(); err != nil {
		return err
	}
	byAlgorithm := map[string]SignatureVerifier{}
	caps := praxiscrypto.Capabilities{}
	for _, verifier := range verifiers {
		if verifier == nil || verifier.Algorithm() == "" {
			continue
		}
		byAlgorithm[verifier.Algorithm()] = verifier
		switch verifier.Class() {
		case SignatureClassClassical:
			caps.Classical = true
		case SignatureClassPQ:
			caps.PQ = true
		}
	}
	caps.Hybrid = caps.Classical && caps.PQ
	resolution, err := praxiscrypto.Resolve(envelope.Profile, caps, allowPQPreferredFallback)
	if err != nil {
		return fmt.Errorf("resolve package signature profile: %w", err)
	}

	valid := map[SignatureClass]bool{}
	var proofErrors []error
	for _, proof := range envelope.Proofs {
		verifier, ok := byAlgorithm[proof.Algorithm]
		if !ok {
			continue
		}
		if err := verifier.Verify(proof, envelope.Statement()); err != nil {
			proofErrors = append(proofErrors, fmt.Errorf("%s/%s: %w", proof.Algorithm, proof.KeyID, err))
			continue
		}
		valid[verifier.Class()] = true
	}

	switch resolution.Selected {
	case contracts.CryptoClassicalCompatible:
		if valid[SignatureClassClassical] {
			return nil
		}
	case contracts.CryptoPQPreferred, contracts.CryptoPQRequired:
		if valid[SignatureClassPQ] {
			return nil
		}
	case contracts.CryptoHybridHighAssurance:
		if valid[SignatureClassClassical] && valid[SignatureClassPQ] {
			return nil
		}
	}
	if len(proofErrors) != 0 {
		return errors.Join(append([]error{errors.New("no valid package signature proof satisfies resolved crypto profile")}, proofErrors...)...)
	}
	return errors.New("no package signature proof satisfies resolved crypto profile")
}
