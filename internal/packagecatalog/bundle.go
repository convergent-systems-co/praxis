package packagecatalog

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// verifyBundleContents treats the signed archive as data and proves that its
// regular-file surface is exactly the typed immutable content inventory. It
// neither interprets graph/plugin semantics nor grants any execution authority.
func verifyBundleContents(manifest Manifest, artifact []byte) (map[string][]byte, error) {
	if len(manifest.Contents) == 0 {
		return nil, nil
	}
	gz, err := gzip.NewReader(bytes.NewReader(artifact))
	if err != nil {
		return nil, fmt.Errorf("open package artifact as gzip tar: %w", err)
	}
	defer gz.Close()

	files := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		header, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("read package archive: %w", nextErr)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if _, err := canonicalArtifactPath(strings.TrimSuffix(header.Name, "/")); err != nil {
				return nil, err
			}
			continue
		case tar.TypeReg, tar.TypeRegA:
		default:
			return nil, fmt.Errorf("package artifact %q uses forbidden archive type %d", header.Name, header.Typeflag)
		}
		name, err := canonicalArtifactPath(header.Name)
		if err != nil {
			return nil, err
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("duplicate package artifact path %q", name)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read package artifact %q: %w", name, err)
		}
		files[name] = body
	}

	referenced := make(map[string]struct{}, len(manifest.Contents))
	for _, content := range manifest.Contents {
		name, err := canonicalArtifactPath(content.Artifact)
		if err != nil {
			return nil, err
		}
		body, ok := files[name]
		if !ok {
			return nil, fmt.Errorf("package content %s/%s@%s is missing artifact %q", content.Kind, content.ID, content.Version, name)
		}
		if bytesDigest(body) != content.Digest {
			return nil, fmt.Errorf("package content %s/%s@%s artifact digest mismatch", content.Kind, content.ID, content.Version)
		}
		referenced[name] = struct{}{}
	}
	for name := range files {
		if _, ok := referenced[name]; !ok {
			return nil, fmt.Errorf("package archive contains unmanifested artifact %q", name)
		}
	}
	return cloneArtifactFiles(files), nil
}

func canonicalArtifactPath(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("package artifact path %q is not a canonical relative path", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean != name || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("package artifact path %q is not a canonical relative path", name)
	}
	return clean, nil
}

func cloneArtifactFiles(files map[string][]byte) map[string][]byte {
	if len(files) == 0 {
		return nil
	}
	out := make(map[string][]byte, len(files))
	for name, body := range files {
		out[name] = append([]byte(nil), body...)
	}
	return out
}
