package packagecatalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// VerificationInput contains observations needed to verify one immutable
// package generation. ResolvedDependencies must themselves have crossed this
// verifier; an unverified manifest cannot satisfy a dependency lock.
type VerificationInput struct {
	ManifestBytes        []byte
	ArtifactBytes        []byte
	Signature            SignatureEnvelope
	ResolvedDependencies map[string]VerifiedPackage
	SourceKind           string
	SourceRef            string
	VerifiedAt           time.Time
	AllowPQFallback      bool
}

// VerificationEvidence is durable evidence of integrity and provenance. It is
// deliberately not installation authority.
type VerificationEvidence struct {
	Version                 string                  `json:"version"`
	ID                      string                  `json:"id"`
	PackageID               string                  `json:"package_id"`
	PackageVersion          string                  `json:"package_version"`
	ArtifactDigest          string                  `json:"artifact_digest"`
	ManifestDigest          string                  `json:"manifest_digest"`
	SignatureEnvelopeDigest string                  `json:"signature_envelope_digest"`
	SignatureProfile        contracts.CryptoProfile `json:"signature_profile"`
	SignerKeyIDs            []string                `json:"signer_key_ids"`
	DependencyEvidenceIDs   []string                `json:"dependency_evidence_ids,omitempty"`
	EffectiveCapabilities   []string                `json:"effective_capabilities,omitempty"`
	RequiredEnforcement     []string                `json:"required_enforcement,omitempty"`
	Publisher               string                  `json:"publisher,omitempty"`
	SourceKind              string                  `json:"source_kind"`
	SourceRef               string                  `json:"source_ref"`
	VerifiedAt              time.Time               `json:"verified_at"`
}

// VerifiedPackage is an in-process capability minted only by VerifyPackage.
// Durable evidence remains inspectable, while bare caller-authored fields
// cannot cross the activation boundary as "verified".
type VerifiedPackage struct {
	manifest      Manifest
	evidence      VerificationEvidence
	manifestBytes []byte
	artifactBytes []byte
	artifactFiles map[string][]byte
	signature     SignatureEnvelope
	sealed        bool
}

func (v VerifiedPackage) Manifest() Manifest { return cloneManifest(v.manifest) }
func (v VerifiedPackage) Evidence() VerificationEvidence {
	return cloneVerificationEvidence(v.evidence)
}
func (v VerifiedPackage) ManifestBytes() []byte { return append([]byte(nil), v.manifestBytes...) }
func (v VerifiedPackage) ArtifactBytes() []byte { return append([]byte(nil), v.artifactBytes...) }
func (v VerifiedPackage) ContentBytes(content ContentRef) ([]byte, error) {
	if err := content.Validate(); err != nil {
		return nil, err
	}
	body, ok := v.artifactFiles[content.Artifact]
	if !ok || bytesDigest(body) != content.Digest {
		return nil, errors.New("verified package does not contain exact content artifact")
	}
	return append([]byte(nil), body...), nil
}
func (v VerifiedPackage) Signature() SignatureEnvelope {
	out := v.signature
	out.Proofs = append([]SignatureProof(nil), v.signature.Proofs...)
	return out
}
func (v VerifiedPackage) Validate() error {
	if !v.sealed {
		return errors.New("package verification must be minted by the verifier")
	}
	if err := v.manifest.Validate(); err != nil {
		return err
	}
	if bytesDigest(v.manifestBytes) != v.evidence.ManifestDigest || v.signature.ManifestDigest != v.evidence.ManifestDigest || v.signature.ArtifactDigest != v.evidence.ArtifactDigest {
		return errors.New("package verification bytes and signature do not bind the evidence")
	}
	if bytesDigest(v.artifactBytes) != v.evidence.ArtifactDigest {
		return errors.New("package verification artifact bytes do not bind the evidence")
	}
	files, err := verifyBundleContents(v.manifest, v.artifactBytes)
	if err != nil || !reflect.DeepEqual(files, v.artifactFiles) {
		return errors.New("package verification content inventory differs from immutable artifact bytes")
	}
	var decoded Manifest
	if err := json.Unmarshal(v.manifestBytes, &decoded); err != nil || !reflect.DeepEqual(decoded, v.manifest) {
		return errors.New("package verification manifest differs from immutable source bytes")
	}
	signatureBytes, err := json.Marshal(v.signature)
	if err != nil || bytesDigest(signatureBytes) != v.evidence.SignatureEnvelopeDigest {
		return errors.New("package verification signature envelope digest mismatch")
	}
	expected, err := freezeVerificationEvidence(v.evidence)
	if err != nil {
		return err
	}
	if expected.ID != v.evidence.ID || v.evidence.PackageID != v.manifest.PackageID || v.evidence.PackageVersion != v.manifest.Version || v.evidence.ArtifactDigest != v.manifest.ContentDigest {
		return errors.New("package verification evidence does not bind the manifest")
	}
	return nil
}

func cloneManifest(m Manifest) Manifest {
	body, _ := json.Marshal(m)
	var out Manifest
	_ = json.Unmarshal(body, &out)
	return out
}

func cloneVerificationEvidence(e VerificationEvidence) VerificationEvidence {
	e.SignerKeyIDs = append([]string(nil), e.SignerKeyIDs...)
	e.DependencyEvidenceIDs = append([]string(nil), e.DependencyEvidenceIDs...)
	e.EffectiveCapabilities = append([]string(nil), e.EffectiveCapabilities...)
	e.RequiredEnforcement = append([]string(nil), e.RequiredEnforcement...)
	return e
}

func VerifyPackage(input VerificationInput, verifiers []SignatureVerifier) (VerifiedPackage, error) {
	if len(input.ManifestBytes) == 0 || len(input.ArtifactBytes) == 0 {
		return VerifiedPackage{}, errors.New("package manifest and artifact bytes are required")
	}
	if input.SourceKind == "" || input.SourceRef == "" || input.VerifiedAt.IsZero() {
		return VerifiedPackage{}, errors.New("package source and verification time are required")
	}
	var manifest Manifest
	if err := json.Unmarshal(input.ManifestBytes, &manifest); err != nil {
		return VerifiedPackage{}, fmt.Errorf("decode package manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return VerifiedPackage{}, err
	}
	manifestDigest := bytesDigest(input.ManifestBytes)
	artifactDigest := bytesDigest(input.ArtifactBytes)
	if manifestDigest != input.Signature.ManifestDigest || artifactDigest != input.Signature.ArtifactDigest || artifactDigest != manifest.ContentDigest {
		return VerifiedPackage{}, errors.New("package manifest, artifact, and signature digests do not bind the same immutable generation")
	}
	if err := VerifySignature(input.Signature, verifiers, input.AllowPQFallback); err != nil {
		return VerifiedPackage{}, err
	}
	artifactFiles, err := verifyBundleContents(manifest, input.ArtifactBytes)
	if err != nil {
		return VerifiedPackage{}, err
	}

	dependencies := make(map[string]Manifest, len(input.ResolvedDependencies))
	dependencyEvidenceIDs := make([]string, 0, len(input.ResolvedDependencies))
	for id, verified := range input.ResolvedDependencies {
		if err := verified.Validate(); err != nil {
			return VerifiedPackage{}, fmt.Errorf("dependency %q: %w", id, err)
		}
		dependency := verified.Manifest()
		if id != dependency.PackageID {
			return VerifiedPackage{}, fmt.Errorf("dependency map key %q does not match package %q", id, dependency.PackageID)
		}
		dependencies[id] = dependency
		dependencyEvidenceIDs = append(dependencyEvidenceIDs, verified.Evidence().ID)
	}
	effective, err := EffectiveCapabilities(manifest, dependencies)
	if err != nil {
		return VerifiedPackage{}, err
	}
	keyIDs := make([]string, 0, len(input.Signature.Proofs))
	for _, proof := range input.Signature.Proofs {
		keyIDs = append(keyIDs, proof.KeyID)
	}
	sort.Strings(keyIDs)
	sort.Strings(dependencyEvidenceIDs)
	envelopeBytes, err := json.Marshal(input.Signature)
	if err != nil {
		return VerifiedPackage{}, err
	}
	evidence, err := freezeVerificationEvidence(VerificationEvidence{
		Version: verificationEvidenceVersions.CurrentVersion(), PackageID: manifest.PackageID, PackageVersion: manifest.Version,
		ArtifactDigest: artifactDigest, ManifestDigest: manifestDigest, SignatureEnvelopeDigest: bytesDigest(envelopeBytes),
		SignatureProfile: input.Signature.Profile, SignerKeyIDs: keyIDs, DependencyEvidenceIDs: dependencyEvidenceIDs,
		EffectiveCapabilities: effective, RequiredEnforcement: append([]string(nil), manifest.RequiredEnforcement...),
		Publisher: manifest.Publisher, SourceKind: input.SourceKind, SourceRef: input.SourceRef, VerifiedAt: input.VerifiedAt.UTC(),
	})
	if err != nil {
		return VerifiedPackage{}, err
	}
	return VerifiedPackage{manifest: manifest, evidence: evidence, manifestBytes: append([]byte(nil), input.ManifestBytes...), artifactBytes: append([]byte(nil), input.ArtifactBytes...), artifactFiles: artifactFiles, signature: input.Signature, sealed: true}, nil
}

func freezeVerificationEvidence(e VerificationEvidence) (VerificationEvidence, error) {
	if err := requirePackageContractVersion(verificationEvidenceVersions, e.Version); err != nil {
		return VerificationEvidence{}, err
	}
	if e.PackageID == "" || e.PackageVersion == "" || e.ArtifactDigest == "" || e.ManifestDigest == "" || e.SignatureEnvelopeDigest == "" || e.SourceKind == "" || e.SourceRef == "" || e.VerifiedAt.IsZero() {
		return VerificationEvidence{}, errors.New("complete package verification evidence is required")
	}
	if err := e.SignatureProfile.Validate(); err != nil {
		return VerificationEvidence{}, err
	}
	e.ID = ""
	e.SignerKeyIDs = canonicalStrings(e.SignerKeyIDs)
	e.DependencyEvidenceIDs = canonicalStrings(e.DependencyEvidenceIDs)
	e.EffectiveCapabilities = canonicalStrings(e.EffectiveCapabilities)
	e.RequiredEnforcement = canonicalStrings(e.RequiredEnforcement)
	body, err := json.Marshal(e)
	if err != nil {
		return VerificationEvidence{}, err
	}
	e.ID = "package-verification:" + bytesDigest(body)
	return e, nil
}

func ValidateVerificationEvidence(e VerificationEvidence) error {
	frozen, err := freezeVerificationEvidence(e)
	if err != nil {
		return err
	}
	if e.ID == "" || frozen.ID != e.ID {
		return errors.New("package verification evidence digest mismatch")
	}
	return nil
}

func canonicalStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	n := 1
	for i := 1; i < len(out); i++ {
		if out[i] != out[n-1] {
			out[n] = out[i]
			n++
		}
	}
	return out[:n]
}

func bytesDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}
