package packagecatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// PackageBuildInput describes one immutable package source tree. The builder
// owns manifest/content identity construction; signing and publisher
// authority remain separate operations.
type PackageBuildInput struct {
	Manifest Manifest
	Files    map[string][]byte
}

// BuiltPackage contains the canonical manifest and archive bytes. It is not a
// signed package until an authorized publisher signer produces an envelope.
type BuiltPackage struct {
	Manifest       Manifest
	ManifestBytes  []byte
	ArtifactBytes  []byte
	ManifestDigest string
	ArtifactDigest string
}

// BuildPackage deterministically constructs the repository's canonical
// package manifest/archive pair. It refuses unmanifested files, missing
// content, noncanonical paths, and caller-supplied digest disagreement.
func BuildPackage(input PackageBuildInput) (BuiltPackage, error) {
	manifest := cloneManifest(input.Manifest)
	providedArtifactDigest := manifest.ContentDigest
	if manifest.ContentDigest == "" {
		// The archive digest is the builder's output identity. A zero digest is
		// only a pre-build placeholder and never leaves this function.
		manifest.ContentDigest = "sha256:" + strings.Repeat("0", 64)
	}
	if err := manifest.Validate(); err != nil {
		return BuiltPackage{}, fmt.Errorf("package manifest: %w", err)
	}
	if len(manifest.Contents) == 0 {
		return BuiltPackage{}, errors.New("package build requires at least one typed content item")
	}
	files := make(map[string][]byte, len(input.Files))
	for name, body := range input.Files {
		canonical, err := canonicalArtifactPath(name)
		if err != nil {
			return BuiltPackage{}, err
		}
		if canonical != name {
			return BuiltPackage{}, fmt.Errorf("package file path %q is not canonical", name)
		}
		if _, exists := files[name]; exists {
			return BuiltPackage{}, fmt.Errorf("duplicate package file %q", name)
		}
		files[name] = append([]byte(nil), body...)
	}
	for _, content := range manifest.Contents {
		body, ok := files[content.Artifact]
		if !ok {
			return BuiltPackage{}, fmt.Errorf("package content %s/%s@%s is missing artifact %q", content.Kind, content.ID, content.Version, content.Artifact)
		}
		if bytesDigest(body) != content.Digest {
			return BuiltPackage{}, fmt.Errorf("package content %s/%s@%s digest does not match artifact", content.Kind, content.ID, content.Version)
		}
	}
	referenced := make(map[string]struct{}, len(manifest.Contents))
	for _, content := range manifest.Contents {
		referenced[content.Artifact] = struct{}{}
	}
	for name := range files {
		if _, ok := referenced[name]; !ok {
			return BuiltPackage{}, fmt.Errorf("package file %q is not declared in manifest contents", name)
		}
	}
	archive, err := deterministicPackageArchive(files)
	if err != nil {
		return BuiltPackage{}, err
	}
	artifactDigest := bytesDigest(archive)
	if providedArtifactDigest != "" && providedArtifactDigest != artifactDigest {
		return BuiltPackage{}, errors.New("package manifest content digest does not match deterministic archive")
	}
	manifest.ContentDigest = artifactDigest
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return BuiltPackage{}, fmt.Errorf("marshal canonical package manifest: %w", err)
	}
	var roundTrip Manifest
	if err := json.Unmarshal(manifestBytes, &roundTrip); err != nil || !equalManifest(manifest, roundTrip) {
		return BuiltPackage{}, errors.New("canonical package manifest round-trip mismatch")
	}
	return BuiltPackage{Manifest: manifest, ManifestBytes: manifestBytes, ArtifactBytes: archive, ManifestDigest: bytesDigest(manifestBytes), ArtifactDigest: artifactDigest}, nil
}

func equalManifest(a, b Manifest) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

// CanonicalContentFiles returns a defensive, stable listing useful to
// package builders and provenance records.
func CanonicalContentFiles(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
