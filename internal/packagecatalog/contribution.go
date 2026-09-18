package packagecatalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/transfer"
)

// CatalogContributionSpec contains package policy interpretation. Core does
// not infer a package content kind from a transfer artifact's domain-defined
// kind; it preserves the explicit, versioned mapping policy.
type CatalogContributionSpec struct {
	PackageID              string      `json:"package_id"`
	PackageVersion         string      `json:"package_version"`
	Publisher              string      `json:"publisher"`
	ContentKind            ContentKind `json:"content_kind"`
	ContentID              string      `json:"content_id"`
	ContentVersion         string      `json:"content_version"`
	ArtifactPath           string      `json:"artifact_path"`
	Compatibility          string      `json:"compatibility,omitempty"`
	PackagingPolicyID      string      `json:"packaging_policy_id"`
	PackagingPolicyVersion string      `json:"packaging_policy_version"`
}

type CatalogContributionEvidence struct {
	Version                string `json:"version"`
	ID                     string `json:"id"`
	PackageID              string `json:"package_id"`
	PackageVersion         string `json:"package_version"`
	PackageDigest          string `json:"package_digest"`
	ArtifactID             string `json:"artifact_id"`
	ArtifactDigest         string `json:"artifact_digest"`
	EvaluationID           string `json:"evaluation_id"`
	PublicationID          string `json:"publication_id"`
	TransferPolicyID       string `json:"transfer_policy_id"`
	PackagingPolicyID      string `json:"packaging_policy_id"`
	PackagingPolicyVersion string `json:"packaging_policy_version"`
}

type CatalogContribution struct {
	Manifest Manifest                    `json:"manifest"`
	Artifact []byte                      `json:"-"`
	Evidence CatalogContributionEvidence `json:"evidence"`
}

type catalogContributionProvenance struct {
	Version                string `json:"version"`
	ArtifactID             string `json:"artifact_id"`
	ArtifactDigest         string `json:"artifact_digest"`
	EvaluationID           string `json:"evaluation_id"`
	PublicationID          string `json:"publication_id"`
	TransferPolicyID       string `json:"transfer_policy_id"`
	PackagingPolicyID      string `json:"packaging_policy_id"`
	PackagingPolicyVersion string `json:"packaging_policy_version"`
}

func BuildCatalogContribution(spec CatalogContributionSpec, published transfer.PublishedArtifact, generalizedContent []byte) (CatalogContribution, error) {
	if err := published.Validate(); err != nil {
		return CatalogContribution{}, err
	}
	if spec.PackageID == "" || spec.PackageVersion == "" || spec.Publisher == "" || spec.ContentKind == "" || spec.ContentID == "" || spec.ContentVersion == "" || spec.ArtifactPath == "" || spec.PackagingPolicyID == "" || spec.PackagingPolicyVersion == "" || len(generalizedContent) == 0 {
		return CatalogContribution{}, errors.New("catalog contribution requires package identity, typed content, mapping policy, publisher, and generalized bytes")
	}
	artifact := published.Artifact()
	if bytesDigest(generalizedContent) != artifact.ContentDigest {
		return CatalogContribution{}, errors.New("catalog contribution bytes do not match the published generalized artifact")
	}
	content := ContentRef{Kind: spec.ContentKind, ID: spec.ContentID, Version: spec.ContentVersion, Digest: artifact.ContentDigest, Artifact: spec.ArtifactPath, Compatibility: spec.Compatibility}
	policy := published.Policy()
	evaluation, publication := published.Evaluation(), published.Publication()
	provenance := catalogContributionProvenance{Version: catalogContributionVersions.CurrentVersion(), ArtifactID: artifact.ID, ArtifactDigest: artifact.ContentDigest, EvaluationID: evaluation.ID, PublicationID: publication.ID, TransferPolicyID: policy.ID, PackagingPolicyID: spec.PackagingPolicyID, PackagingPolicyVersion: spec.PackagingPolicyVersion}
	provenanceBytes, err := json.Marshal(provenance)
	if err != nil {
		return CatalogContribution{}, err
	}
	provenanceContent := ContentRef{Kind: ContentDocumentation, ID: spec.ContentID + ".catalog-provenance", Version: spec.ContentVersion, Digest: bytesDigest(provenanceBytes), Artifact: "provenance/catalog-contribution.json"}
	if content.Artifact == provenanceContent.Artifact {
		return CatalogContribution{}, errors.New("catalog contribution content path conflicts with reserved provenance path")
	}
	archive, err := deterministicPackageArchive(map[string][]byte{content.Artifact: generalizedContent, provenanceContent.Artifact: provenanceBytes})
	if err != nil {
		return CatalogContribution{}, err
	}
	manifest := Manifest{
		ContractVersion: ManifestContractCurrentVersion(), PackageID: spec.PackageID, Version: spec.PackageVersion,
		ContentDigest: bytesDigest(archive), Publisher: spec.Publisher, Contents: []ContentRef{content, provenanceContent},
		UpstreamPackageID: policy.PackageID, UpstreamDigest: artifact.ID,
	}
	if err := validateCatalogContributionOutput(manifest, archive, content, provenanceContent); err != nil {
		return CatalogContribution{}, err
	}
	evidence, err := freezeCatalogContributionEvidence(CatalogContributionEvidence{
		Version: catalogContributionVersions.CurrentVersion(), PackageID: manifest.PackageID, PackageVersion: manifest.Version, PackageDigest: manifest.ContentDigest,
		ArtifactID: artifact.ID, ArtifactDigest: artifact.ContentDigest, EvaluationID: evaluation.ID, PublicationID: publication.ID,
		TransferPolicyID: policy.ID, PackagingPolicyID: spec.PackagingPolicyID, PackagingPolicyVersion: spec.PackagingPolicyVersion,
	})
	if err != nil {
		return CatalogContribution{}, err
	}
	return CatalogContribution{Manifest: manifest, Artifact: archive, Evidence: evidence}, nil
}

// validateCatalogContributionOutput protects the operation-specific contract:
// successful construction contains both the package-policy-mapped generalized
// artifact and its safe provenance sidecar. It deliberately does not assign an
// exact total inventory size or ordering; compatible future sidecars may extend
// the package without changing either required semantic identity.
func validateCatalogContributionOutput(manifest Manifest, archive []byte, required ...ContentRef) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	files, err := verifyBundleContents(manifest, archive)
	if err != nil {
		return err
	}
	for _, expected := range required {
		actual, ok := manifest.Content(expected.Kind, expected.ID, expected.Version)
		if !ok || actual != expected {
			return errors.New("catalog contribution omitted or changed required semantic content")
		}
		if body, ok := files[expected.Artifact]; !ok || bytesDigest(body) != expected.Digest {
			return errors.New("catalog contribution required content does not bind archive bytes")
		}
	}
	return nil
}

func freezeCatalogContributionEvidence(evidence CatalogContributionEvidence) (CatalogContributionEvidence, error) {
	if _, _, err := catalogContributionVersions.Canonicalize(evidence.Version, nil); err != nil {
		return CatalogContributionEvidence{}, err
	}
	evidence.ID = ""
	if evidence.PackageID == "" || evidence.PackageVersion == "" || evidence.PackageDigest == "" || evidence.ArtifactID == "" || evidence.ArtifactDigest == "" || evidence.EvaluationID == "" || evidence.PublicationID == "" || evidence.TransferPolicyID == "" || evidence.PackagingPolicyID == "" || evidence.PackagingPolicyVersion == "" {
		return CatalogContributionEvidence{}, errors.New("catalog contribution evidence requires complete package, transfer, and mapping provenance")
	}
	body, err := json.Marshal(evidence)
	if err != nil {
		return CatalogContributionEvidence{}, err
	}
	evidence.ID = "catalog-contribution:" + bytesDigest(body)
	return evidence, nil
}

func deterministicPackageArchive(files map[string][]byte) ([]byte, error) {
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	gz.Header.ModTime = time.Unix(0, 0).UTC()
	gz.Header.OS = 255
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := files[name]
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(body); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
