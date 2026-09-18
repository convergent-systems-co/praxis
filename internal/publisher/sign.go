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
	"strconv"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func canonicalDigest(value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// BuildSigningPreview resolves and freezes the exact package, publisher, and
// effective package.publish authority state. The caller supplies only build
// provenance; security-sensitive relationships are resolved from durable state.
func BuildSigningPreview(ctx context.Context, store *state.Store, authority ExactPackagePublishAuthorizer, generationDigest string, built packagecatalog.BuiltPackage, sourceIdentity, builderIdentity, qualificationRef string, now time.Time) (contracts.SigningPreview, error) {
	if store == nil || authority == nil || generationDigest == "" {
		return contracts.SigningPreview{}, errors.New("signing preview state, authority, and generation are required")
	}
	if err := built.Manifest.Validate(); err != nil {
		return contracts.SigningPreview{}, err
	}
	record, err := store.PublisherGeneration(ctx, generationDigest)
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	if record.State != "active" || !record.Generation.PackageNamespaceAllowed(built.Manifest.PackageID) {
		return contracts.SigningPreview{}, errors.New("publisher generation is not current for package")
	}
	auth, err := authority.ResolvePackagePublishAuthority(ctx, generationDigest, built.Manifest.PackageID, now)
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	if auth.Generation.SubjectKeyDigest != "" && auth.Generation.SubjectKeyDigest != record.Generation.PublicKeyDigest {
		return contracts.SigningPreview{}, errors.New("package.publish authority key does not match publisher generation")
	}
	invocation, binding, err := executableBoundInvocation(built.Manifest)
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	contractDigest, err := canonicalDigest(invocation)
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	bindingDigest, err := canonicalDigest(binding)
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	plugin, ok := built.Manifest.Content(packagecatalog.ContentPlugin, binding.PluginID, binding.PluginVersion)
	if !ok || plugin.Digest != binding.PluginDefinitionDigest {
		return contracts.SigningPreview{}, errors.New("package plugin definition is not exactly bound")
	}
	executable, ok := built.Manifest.Content(packagecatalog.ContentPluginExecutable, binding.ExecutableContentID, binding.ExecutableContentVersion)
	if !ok || executable.Digest != binding.ExecutableDigest {
		return contracts.SigningPreview{}, errors.New("package executable is not exactly bound")
	}
	requestDigest, err := auth.Request.Digest()
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	decisionDigest, err := auth.Decision.Digest()
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	if auth.Generation.ExpiresAt == nil {
		return contracts.SigningPreview{}, errors.New("package.publish authority has no expiry")
	}
	createdAt := now.UTC()
	preview := contracts.SigningPreview{ID: signingPreviewAttemptID(generationDigest, built.ArtifactDigest, createdAt), Version: "1", PackageID: built.Manifest.PackageID, PackageVersion: built.Manifest.Version, PackageNamespace: record.Generation.PackageNamespace, ManifestDigest: built.ManifestDigest, ArtifactDigest: built.ArtifactDigest, ExecutableDigest: binding.ExecutableDigest, InvocationContractDigest: contractDigest, ExecutableBindingDigest: bindingDigest, PluginDefinitionDigest: binding.PluginDefinitionDigest, RuntimeID: binding.RuntimeID, RuntimeVersion: binding.RuntimeVersion, RuntimeDigest: binding.RuntimeDigest, SourceIdentity: sourceIdentity, BuilderIdentity: builderIdentity, QualificationRef: qualificationRef, PublisherPrincipal: record.Generation.Principal.ID, PublisherGenerationDigest: generationDigest, PublisherGenerationVersion: record.Generation.Version, PublicKeyDigest: record.Generation.PublicKeyDigest, KeyID: record.Generation.KeyID, Algorithm: record.Generation.Algorithm, AuthorityGenerationRef: auth.Generation.Ref, AuthorityGenerationVersion: auth.Generation.Version, AuthorityGenerationDigest: auth.Generation.Digest, AuthorityModel: auth.Generation.AuthorityModel, AuthorityModelVersion: auth.Generation.AuthorityModelVersion, AuthorityModelDigest: auth.Generation.AuthorityModelDigest, ParentRef: auth.Generation.ParentRef, ParentVersion: auth.Generation.ParentVersion, ParentDigest: auth.Generation.ParentDigest, AuthorityScope: auth.Generation.Scope, AuthorityRequestID: auth.Request.ID, AuthorityRequestVersion: auth.Request.Version, AuthorityRequestDigest: requestDigest, AuthorityDecisionRef: auth.Decision.DecisionRef, AuthorityDecisionVersion: auth.Decision.DecisionVersion, AuthorityDecisionDigest: decisionDigest, ExpiresAt: *auth.Generation.ExpiresAt, CreatedAt: createdAt}
	d, err := preview.DigestValue()
	if err != nil {
		return contracts.SigningPreview{}, err
	}
	preview.Digest = d
	return preview, nil
}

// executableBoundInvocation selects the one invocation the package binds to
// its plugin executable. A first-party package may expose further
// invocations that the control plane dispatches natively; those carry no
// executable binding and are bound into the signature through the manifest
// digest, which covers every invocation contract. Exactly one executable
// binding is required so the preview freezes one plugin, one executable, and
// one runtime identity.
func executableBoundInvocation(manifest packagecatalog.Manifest) (contracts.InvocationContract, contracts.ExecutableBinding, error) {
	if len(manifest.Invocations) == 0 {
		return contracts.InvocationContract{}, contracts.ExecutableBinding{}, errors.New("signing preview requires at least one invocation")
	}
	if len(manifest.ExecutableBindings) != 1 {
		return contracts.InvocationContract{}, contracts.ExecutableBinding{}, errors.New("signing preview requires exactly one executable binding")
	}
	binding := manifest.ExecutableBindings[0]
	for _, invocation := range manifest.Invocations {
		if invocation.EntryPointID == binding.EntryPointID {
			return invocation, binding, nil
		}
	}
	return contracts.InvocationContract{}, contracts.ExecutableBinding{}, errors.New("signing preview requires the executable-bound invocation")
}

// signingPreviewAttemptID separates a signing attempt from the immutable
// package identity. The timestamp is part of the attempt identity; the
// package and authority digests remain bound in the preview body.
func signingPreviewAttemptID(generationDigest, artifactDigest string, createdAt time.Time) string {
	return "publisher-signing-preview:" + generationDigest + ":" + artifactDigest + ":" + strconv.FormatInt(createdAt.UTC().UnixNano(), 10)
}

// SignWithPreview consumes only a durable, exact signing preview. It
// revalidates every bound identity immediately before the protected signer is
// called and never substitutes a newer authority generation.
func SignWithPreview(ctx context.Context, store *state.Store, authority ExactPackagePublishAuthorizer, signer praxiscrypto.PublisherSigner, preview contracts.SigningPreview, built packagecatalog.BuiltPackage, now time.Time) (SignedPackage, error) {
	if store == nil || authority == nil || signer == nil {
		return SignedPackage{}, errors.New("signing state, authority, and signer are required")
	}
	if err := preview.VerifyDigest(); err != nil {
		return SignedPackage{}, err
	}
	if err := built.Manifest.Validate(); err != nil {
		return SignedPackage{}, err
	}
	if built.ManifestDigest != preview.ManifestDigest || built.ArtifactDigest != preview.ArtifactDigest || built.Manifest.PackageID != preview.PackageID || built.Manifest.Version != preview.PackageVersion {
		return SignedPackage{}, errors.New("signing preview is stale for package bytes")
	}
	record, err := store.PublisherGeneration(ctx, preview.PublisherGenerationDigest)
	if err != nil {
		return SignedPackage{}, err
	}
	if record.State != "active" || record.Generation.PublicKeyDigest != preview.PublicKeyDigest || record.Generation.KeyID != preview.KeyID || record.Generation.Principal.ID != preview.PublisherPrincipal {
		return SignedPackage{}, errors.New("signing preview publisher generation is stale")
	}
	auth, err := authority.ResolvePackagePublishAuthority(ctx, preview.PublisherGenerationDigest, preview.PackageID, now)
	if err != nil {
		return SignedPackage{}, err
	}
	if auth.Generation.Digest != preview.AuthorityGenerationDigest || auth.Generation.Ref != preview.AuthorityGenerationRef || auth.Generation.Version != preview.AuthorityGenerationVersion || auth.Generation.Scope != preview.AuthorityScope || auth.Generation.AuthorityModelDigest != preview.AuthorityModelDigest {
		return SignedPackage{}, errors.New("signing preview package.publish authority is stale")
	}
	requestDigest, _ := auth.Request.Digest()
	decisionDigest, _ := auth.Decision.Digest()
	if requestDigest != preview.AuthorityRequestDigest || decisionDigest != preview.AuthorityDecisionDigest {
		return SignedPackage{}, errors.New("signing preview governance provenance is stale")
	}
	pub, err := signer.PublicKey(ctx)
	if err != nil {
		return SignedPackage{}, err
	}
	sum := sha256.Sum256(pub)
	pubDigest := "sha256:" + hex.EncodeToString(sum[:])
	if signer.KeyID() != preview.KeyID || signer.Algorithm() != preview.Algorithm || pubDigest != preview.PublicKeyDigest {
		return SignedPackage{}, errors.New("protected signing key does not match signing preview")
	}
	if auth.Generation.ExpiresAt == nil || !now.Before(*auth.Generation.ExpiresAt) {
		return SignedPackage{}, errors.New("package.publish authority is expired")
	}
	envelope := packagecatalog.SignatureEnvelope{Version: packagecatalog.SignatureEnvelopeCurrentVersion(), Profile: contracts.CryptoClassicalCompatible, ManifestDigest: built.ManifestDigest, ArtifactDigest: built.ArtifactDigest}
	signature, err := signer.Sign(ctx, envelope.Statement())
	if err != nil {
		return SignedPackage{}, fmt.Errorf("protected publisher signing: %w", err)
	}
	envelope.Proofs = []packagecatalog.SignatureProof{{Algorithm: signer.Algorithm(), KeyID: signer.KeyID(), Signature: base64.StdEncoding.EncodeToString(signature)}}
	if err := packagecatalog.VerifySignature(envelope, []packagecatalog.SignatureVerifier{packagecatalog.Ed25519Verifier{TrustedKeys: map[string]ed25519.PublicKey{signer.KeyID(): pub}}}, false); err != nil {
		return SignedPackage{}, err
	}
	envelopeBytes, _ := json.Marshal(envelope)
	envelopeSum := sha256.Sum256(envelopeBytes)
	provenance := Provenance{Version: "v2", PublisherPrincipal: preview.PublisherPrincipal, PublisherGenerationDigest: preview.PublisherGenerationDigest, KeyID: preview.KeyID, Algorithm: preview.Algorithm, PackageID: preview.PackageID, PackageVersion: preview.PackageVersion, ManifestDigest: preview.ManifestDigest, ArtifactDigest: preview.ArtifactDigest, SourceIdentity: preview.SourceIdentity, BuilderIdentity: preview.BuilderIdentity, QualificationRef: preview.QualificationRef, SignedAt: now.UTC(), SignatureEnvelopeDigest: "sha256:" + hex.EncodeToString(envelopeSum[:]), SigningPreviewDigest: preview.Digest, PackagePublishAuthorityDigest: preview.AuthorityGenerationDigest, PackagePublishRequestDigest: preview.AuthorityRequestDigest, PackagePublishDecisionDigest: preview.AuthorityDecisionDigest}
	provenanceDigest, err := provenance.Digest()
	if err != nil {
		return SignedPackage{}, err
	}
	if err := store.PersistPublisherSigningReceipt(ctx, provenanceDigest, preview.PublisherGenerationDigest, preview.PackageID, preview.PackageVersion, envelope, provenance, now); err != nil {
		return SignedPackage{}, err
	}
	return SignedPackage{Envelope: envelope, Provenance: provenance, ProvenanceDigest: provenanceDigest}, nil
}

type Provenance struct {
	Version                       string    `json:"version"`
	PublisherPrincipal            string    `json:"publisher_principal"`
	PublisherGenerationDigest     string    `json:"publisher_generation_digest"`
	KeyID                         string    `json:"key_id"`
	Algorithm                     string    `json:"algorithm"`
	PackageID                     string    `json:"package_id"`
	PackageVersion                string    `json:"package_version"`
	ManifestDigest                string    `json:"manifest_digest"`
	ArtifactDigest                string    `json:"artifact_digest"`
	SourceIdentity                string    `json:"source_identity"`
	BuilderIdentity               string    `json:"builder_identity"`
	QualificationRef              string    `json:"qualification_ref,omitempty"`
	SignedAt                      time.Time `json:"signed_at"`
	SignatureEnvelopeDigest       string    `json:"signature_envelope_digest"`
	SigningPreviewDigest          string    `json:"signing_preview_digest"`
	PackagePublishAuthorityDigest string    `json:"package_publish_authority_digest"`
	PackagePublishRequestDigest   string    `json:"package_publish_request_digest"`
	PackagePublishDecisionDigest  string    `json:"package_publish_decision_digest"`
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

type PackagePublishAuthorizer interface {
	ValidatePackagePublishAuthority(context.Context, string, string, time.Time) error
}

type ExactPackagePublishAuthorizer interface {
	ResolvePackagePublishAuthority(context.Context, string, string, time.Time) (contracts.PackagePublishAuthorization, error)
}

// Sign verifies publisher authority before invoking the protected signer. The
// signer cannot authorize itself and receives only the canonical envelope
// statement after all package identities have been checked.
func Sign(ctx context.Context, store *state.Store, authority PackagePublishAuthorizer, signer praxiscrypto.PublisherSigner, generationDigest string, built packagecatalog.BuiltPackage, sourceIdentity, builderIdentity, qualificationRef string, now time.Time) (SignedPackage, error) {
	return SignedPackage{}, errors.New("canonical signing preview required")
	/* Legacy direct-sign implementation retained below only as unreachable
	   source during this transition; all production callers must use
	   SignWithPreview. */
	/*
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
		if err := authority.ValidatePackagePublishAuthority(ctx, generationDigest, built.Manifest.PackageID, now); err != nil {
			return SignedPackage{}, fmt.Errorf("package.publish authority denied: %w", err)
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
		return SignedPackage{Envelope: envelope, Provenance: provenance, ProvenanceDigest: provenanceDigest}, nil */
}
