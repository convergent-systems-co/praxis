package distribution

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

// SourceGitHubReleases and SourceLocalFirstParty are the canonical
// distribution.PackageRef.Source values this repository recognizes. A
// resolver or CLI entry point selects a transport by this exact string; an
// unrecognized value must fail closed, never silently default to a
// transport.
const (
	SourceGitHubReleases  = "github-releases"
	SourceLocalFirstParty = "local-first-party"
)

// RootAdapter is satisfied by a transport usable both to fetch a root
// package release directly and to resolve its locked dependencies. Every
// distribution.Adapter implementation in this repository also implements
// LockedAdapter; this is their combined capability, used wherever a caller
// needs one concrete transport for both roles.
type RootAdapter interface {
	Adapter
	LockedAdapter
}

// LocalFirstParty acquires an exact, previously built package from a
// canonical local directory. It is a transport only: it supplies bytes to
// the SAME downstream packagecatalog.VerifyPackage pipeline every other
// transport uses, with no special trust treatment. Locality is not trust —
// an unsigned or otherwise unverifiable local package is refused by that
// pipeline exactly like any other source. This adapter's own, narrower job
// is to refuse acquisition itself when the bytes on disk do not match the
// identity pinned for them, so a modified local package is caught here, not
// only downstream.
//
// Layout, under Root/<package-id>/<version>/:
//
//	manifest.json    the exact packagecatalog.Manifest bytes
//	artifact.tar.gz  the exact package archive bytes
//	identity.json    {"manifest_digest":"sha256:...","artifact_digest":"sha256:..."}
//	                 pinned expected digests; acquisition refuses if the
//	                 files on disk do not match them
//	signature.json   optional packagecatalog.SignatureEnvelope bytes; a
//	                 package with none is still passed through (unsigned),
//	                 to be refused, as any other unsigned package would be,
//	                 by packagecatalog.VerifyPackage
type LocalFirstParty struct {
	// Root is the canonical local package directory. It must be supplied
	// explicitly by the caller; there is no default and no environment
	// variable read inside this package.
	Root string
}

type localPinnedIdentity struct {
	ManifestDigest string `json:"manifest_digest"`
	ArtifactDigest string `json:"artifact_digest"`
}

func localDigest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// versionDir resolves the on-disk directory for one package id/version and
// requires it to remain a descendant of Root. repo and version are attacker
// influenced (a package.PackageID has no character restriction, and repo is
// taken directly from a package.deploy-intent-preview CLI argument), so
// filepath.Join alone is not a containment guarantee: a crafted "../"-shaped
// segment can walk outside Root. This is resolved with filepath.Abs on both
// Root and the joined candidate and a filepath.Rel containment check,
// refusing (not merely warning) whenever the resolved directory is not
// exactly Root or a descendant of it.
func (a LocalFirstParty) versionDir(repo, version string) (string, error) {
	if a.Root == "" {
		return "", errors.New("local-first-party adapter requires an explicit Root")
	}
	if repo == "" || version == "" {
		return "", errors.New("local-first-party package id and version are required")
	}
	root, err := filepath.Abs(a.Root)
	if err != nil {
		return "", fmt.Errorf("resolve local-first-party root: %w", err)
	}
	dir, err := filepath.Abs(filepath.Join(root, repo, version))
	if err != nil {
		return "", fmt.Errorf("resolve local package %s/%s directory: %w", repo, version, err)
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local package %s/%s resolves outside the configured local package root", repo, version)
	}
	return dir, nil
}

// load reads, pins, and cross-checks one local package version. Every
// returned Release has already been proven, byte-for-byte, to match its own
// pinned identity record; a mismatch anywhere fails closed with no partial
// result.
func (a LocalFirstParty) load(ref PackageRef, version string) (Release, []byte, error) {
	if ref.Source != SourceLocalFirstParty {
		return Release{}, nil, fmt.Errorf("local-first-party adapter cannot resolve source %q", ref.Source)
	}
	dir, err := a.versionDir(ref.Repo, version)
	if err != nil {
		return Release{}, nil, err
	}
	pinnedBytes, err := os.ReadFile(filepath.Join(dir, "identity.json"))
	if err != nil {
		return Release{}, nil, fmt.Errorf("local package %s/%s: missing pinned identity: %w", ref.Repo, version, err)
	}
	var pinned localPinnedIdentity
	if err := json.Unmarshal(pinnedBytes, &pinned); err != nil || pinned.ManifestDigest == "" || pinned.ArtifactDigest == "" {
		return Release{}, nil, fmt.Errorf("local package %s/%s: pinned identity is malformed", ref.Repo, version)
	}
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Release{}, nil, fmt.Errorf("local package %s/%s: missing manifest: %w", ref.Repo, version, err)
	}
	if d := localDigest(manifestBytes); d != pinned.ManifestDigest {
		return Release{}, nil, fmt.Errorf("local package %s/%s: manifest on disk does not match its pinned identity (got %s, want %s)", ref.Repo, version, d, pinned.ManifestDigest)
	}
	artifactBytes, err := os.ReadFile(filepath.Join(dir, "artifact.tar.gz"))
	if err != nil {
		return Release{}, nil, fmt.Errorf("local package %s/%s: missing artifact: %w", ref.Repo, version, err)
	}
	if d := localDigest(artifactBytes); d != pinned.ArtifactDigest {
		return Release{}, nil, fmt.Errorf("local package %s/%s: artifact on disk does not match its pinned identity (got %s, want %s)", ref.Repo, version, d, pinned.ArtifactDigest)
	}
	var manifest packagecatalog.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Release{}, nil, fmt.Errorf("local package %s/%s: decode manifest: %w", ref.Repo, version, err)
	}
	if manifest.PackageID != ref.Repo || manifest.Version != version {
		return Release{}, nil, fmt.Errorf("local package %s/%s: manifest identifies %s/%s, not the requested package", ref.Repo, version, manifest.PackageID, manifest.Version)
	}
	if manifest.ContentDigest != pinned.ArtifactDigest {
		return Release{}, nil, fmt.Errorf("local package %s/%s: manifest content digest does not match the pinned artifact digest", ref.Repo, version)
	}
	release := Release{
		Ref: ref, Tag: version, ManifestDigest: pinned.ManifestDigest,
		Manifest: manifest, ManifestBytes: manifestBytes,
	}
	if sigBytes, err := os.ReadFile(filepath.Join(dir, "signature.json")); err == nil {
		var sig packagecatalog.SignatureEnvelope
		if err := json.Unmarshal(sigBytes, &sig); err != nil {
			return Release{}, nil, fmt.Errorf("local package %s/%s: malformed signature envelope: %w", ref.Repo, version, err)
		}
		release.SignatureBytes = sigBytes
		release.Signature = sig
	} else if !os.IsNotExist(err) {
		return Release{}, nil, fmt.Errorf("local package %s/%s: read signature: %w", ref.Repo, version, err)
	}
	return release, artifactBytes, nil
}

func (a LocalFirstParty) Discover(ctx context.Context, query string) ([]Candidate, error) {
	if a.Root == "" {
		return nil, errors.New("local-first-party adapter requires an explicit Root")
	}
	entries, err := os.ReadDir(a.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Candidate
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if query != "" && e.Name() != query {
			continue
		}
		out = append(out, Candidate{Ref: PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: e.Name()}})
	}
	return out, nil
}

func (a LocalFirstParty) Info(ctx context.Context, ref PackageRef, version string) (Release, error) {
	release, _, err := a.load(ref, version)
	return release, err
}

func (a LocalFirstParty) Resolve(ctx context.Context, ref PackageRef, version string) (Release, error) {
	release, _, err := a.load(ref, version)
	return release, err
}

func (a LocalFirstParty) FetchArtifact(ctx context.Context, release Release) ([]byte, error) {
	if release.Ref.Source != SourceLocalFirstParty {
		return nil, fmt.Errorf("local-first-party adapter cannot fetch source %q", release.Ref.Source)
	}
	_, artifact, err := a.load(release.Ref, release.Tag)
	return artifact, err
}

// CheckUpdate reports no update: a local-first-party package is exactly the
// one pinned on disk, not a moving target this adapter polls.
func (a LocalFirstParty) CheckUpdate(ctx context.Context, ref PackageRef, installedVersion string) (Release, bool, error) {
	return Release{}, false, nil
}

func (a LocalFirstParty) ResolveLocked(ctx context.Context, dependency packagecatalog.Dependency) (Release, error) {
	if dependency.SourceKind != SourceLocalFirstParty {
		return Release{}, fmt.Errorf("local-first-party adapter cannot resolve locked source %q", dependency.SourceKind)
	}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: dependency.PackageID}
	release, _, err := a.load(ref, dependency.Version)
	if err != nil {
		return Release{}, err
	}
	if release.Manifest.ContentDigest != dependency.Digest {
		return Release{}, fmt.Errorf("local package %s/%s does not match its immutable dependency lock digest", dependency.PackageID, dependency.Version)
	}
	return release, nil
}
