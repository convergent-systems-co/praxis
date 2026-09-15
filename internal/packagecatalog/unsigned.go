package packagecatalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
)

// ParseUnsignedPackage independently reconstructs the builder input from the
// canonical manifest/archive pair. It is intentionally separate from trust
// verification: this proves internal consistency, not publisher authority.
func ParseUnsignedPackage(manifestBytes, artifactBytes []byte) (BuiltPackage, error) {
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return BuiltPackage{}, fmt.Errorf("decode package manifest: %w", err)
	}
	if bytesDigest(manifestBytes) != bytesDigest(mustCanonicalManifest(manifest)) {
		return BuiltPackage{}, fmt.Errorf("manifest is not canonical")
	}
	if err := manifest.Validate(); err != nil {
		return BuiltPackage{}, err
	}
	gz, err := gzip.NewReader(bytes.NewReader(artifactBytes))
	if err != nil {
		return BuiltPackage{}, fmt.Errorf("open package archive: %w", err)
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			gz.Close()
			return BuiltPackage{}, e
		}
		if h.Typeflag != tar.TypeReg {
			gz.Close()
			return BuiltPackage{}, fmt.Errorf("non-regular archive entry %q", h.Name)
		}
		if _, ok := files[h.Name]; ok {
			gz.Close()
			return BuiltPackage{}, fmt.Errorf("duplicate archive entry %q", h.Name)
		}
		body, e := io.ReadAll(tr)
		if e != nil {
			gz.Close()
			return BuiltPackage{}, e
		}
		files[h.Name] = body
	}
	if err := gz.Close(); err != nil {
		return BuiltPackage{}, err
	}
	built, err := BuildPackage(PackageBuildInput{Manifest: manifest, Files: files})
	if err != nil {
		return BuiltPackage{}, fmt.Errorf("verify unsigned package: %w", err)
	}
	if !bytes.Equal(built.ManifestBytes, manifestBytes) || !bytes.Equal(built.ArtifactBytes, artifactBytes) {
		return BuiltPackage{}, fmt.Errorf("package bytes are not canonical")
	}
	return built, nil
}

func mustCanonicalManifest(manifest Manifest) []byte { b, _ := json.Marshal(manifest); return b }
