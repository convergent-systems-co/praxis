package publisher

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/convergent-systems-co/praxis/internal/capability"
	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type Provenance struct {
	Version                   string    `json:"version"`
	PublisherPrincipal        string    `json:"publisher_principal"`
	PublisherGenerationDigest string    `json:"publisher_generation_digest"`
	KeyID                     string    `json:"key_id"`
	Algorithm                 string    `json:"algorithm"`
	PackageID                 string    `json:"package_id"`
	PackageVersion            string    `json:"package_version"`
	ManifestDigest            string    `json:"manifest_digest"`
	ArtifactDigest            string    `json:"artifact_digest"`
	SourceIdentity            string    `json:"source_identity"`
	BuilderIdentity           string    `json:"builder_identity"`
	QualificationRef          string    `json:"qualification_ref,omitempty"`
	SignedAt                  time.Time `json:"signed_at"`
	SignatureEnvelopeDigest   string    `json:"signature_envelope_digest"`
}

func (p Provenance) Digest() (string, error) {
	b, e := json.Marshal(p)
	if e != nil {
		return "", e
	}
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:]), nil
}

type SignedPackage struct {
	Envelope         packagecatalog.SignatureEnvelope
	Provenance       Provenance
	ProvenanceDigest string
}

// Sign verifies publisher authority before invoking the protected signer. The
// signer cannot authorize itself and receives only the canonical envelope
// statement after all package identities have been checked.
func Sign(ctx context.Context, store *state.Store, signer praxiscrypto.PublisherSigner, generationDigest string, built packagecatalog.BuiltPackage, sourceIdentity, builderIdentity, qualificationRef string, now time.Time) (SignedPackage, error) {
	if store == nil || signer == nil {
		return SignedPackage{}, errors.New("publisher state and signer are required")
	}
	if now.IsZero() {
		return SignedPackage{}, errors.New("signing time is required")
	}
	if err := built.Manifest.Validate(); err != nil {
		return SignedPackage{}, err
	}
	if generationDigest == "" {
		return SignedPackage{}, errors.New("publisher generation digest is required")
	}
	record, err := store.PublisherGeneration(ctx, generationDigest)
	if err != nil {
		return SignedPackage{}, err
	}
	if record.State != "active" {
		return SignedPackage{}, errors.New("publisher generation is not active")
	}
	if !record.Generation.PackageNamespaceAllowed(built.Manifest.PackageID) {
		return SignedPackage{}, errors.New("publisher generation namespace does not allow package")
	}
	pub, err := signer.PublicKey(ctx)
	if err != nil {
		return SignedPackage{}, fmt.Errorf("publisher public key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return SignedPackage{}, errors.New("publisher signer returned invalid public key")
	}
	sum := sha256.Sum256(pub)
	pubDigest := "sha256:" + hex.EncodeToString(sum[:])
	if signer.KeyID() != record.Generation.KeyID || signer.Algorithm() != record.Generation.Algorithm || pubDigest != record.Generation.PublicKeyDigest {
		return SignedPackage{}, errors.New("publisher signer does not match enrolled generation")
	}
	leases, err := store.LeasesForPrincipal(ctx, record.Generation.Principal, contracts.PackagePublishCapability)
	if err != nil {
		return SignedPackage{}, err
	}
	authorized := false
	for _, lease := range leases {
		if err := capability.Evaluate(lease, capability.Request{Principal: record.Generation.Principal, Capability: contracts.PackagePublishCapability, Operation: "sign", Scope: "package:" + built.Manifest.PackageID, Now: now}); err == nil {
			authorized = true
			break
		}
	}
	if !authorized {
		return SignedPackage{}, errors.New("package.publish authority denied")
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: built.ManifestDigest, ArtifactDigest: built.ArtifactDigest}
	signature, err := signer.Sign(ctx, envelope.Statement())
	if err != nil {
		return SignedPackage{}, fmt.Errorf("protected publisher signing: %w", err)
	}
	envelope.Proofs = []packagecatalog.SignatureProof{{Algorithm: signer.Algorithm(), KeyID: signer.KeyID(), Signature: base64.StdEncoding.EncodeToString(signature)}}
	if err := packagecatalog.VerifySignature(envelope, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{signer.KeyID(): pub}}}, false); err != nil {
		return SignedPackage{}, fmt.Errorf("verify publisher signature: %w", err)
	}
	envelopeBytes, _ := json.Marshal(envelope)
	envelopeDigest := sha256.Sum256(envelopeBytes)
	provenance := Provenance{Version: "v1", PublisherPrincipal: record.Generation.Principal.ID, PublisherGenerationDigest: generationDigest, KeyID: signer.KeyID(), Algorithm: signer.Algorithm(), PackageID: built.Manifest.PackageID, PackageVersion: built.Manifest.Version, ManifestDigest: built.ManifestDigest, ArtifactDigest: built.ArtifactDigest, SourceIdentity: sourceIdentity, BuilderIdentity: builderIdentity, QualificationRef: qualificationRef, SignedAt: now.UTC(), SignatureEnvelopeDigest: "sha256:" + hex.EncodeToString(envelopeDigest[:])}
	provenanceDigest, err := provenance.Digest()
	if err != nil {
		return SignedPackage{}, err
	}
	if err := store.PersistPublisherSigningReceipt(ctx, provenanceDigest, generationDigest, built.Manifest.PackageID, built.Manifest.Version, envelope, provenance, now); err != nil {
		return SignedPackage{}, fmt.Errorf("persist publisher signing provenance: %w", err)
	}
	return SignedPackage{Envelope: envelope, Provenance: provenance, ProvenanceDigest: provenanceDigest}, nil
}
