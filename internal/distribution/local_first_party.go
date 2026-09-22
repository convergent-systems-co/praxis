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

// Containment policy (Astra Review 2 finding L2, Astra Review 3 finding L3):
// two prior attempts at this were both defeated. A lexical check alone
// (filepath.Abs + filepath.Rel) proves nothing about the filesystem: a
// symlink placed beneath Root can resolve to a location outside it
// regardless of what the lexical path looks like (L2). A hand-rolled
// os.Lstat "no symlinks below Root" walk, performed as a separate step
// before a plain os.ReadFile, is not atomic: the filesystem can be mutated
// between the check and the read, so the check observing "no symlink" does
// not mean the subsequent read still sees the same object (L3) — this is
// the classic TOCTOU shape of any check-then-open sequence built from
// independent pathname lookups.
//
// The fix is to stop performing a separate check at all and instead use
// os.Root (Go's stdlib primitive built specifically for "access files only
// within a single directory tree"): every read below goes through one
// os.Root opened once on the configured Root, and each of its methods
// independently, atomically enforces "no component may reference a
// location outside the root" as part of the single underlying open
// operation — there is no window between a check and a use, because there
// is no separate check. Root itself may still be a symlink (operator
// configuration, not attacker-influenced input; os.OpenRoot follows
// symlinks in the root's own name, exactly as filepath.EvalSymlinks did
// before). Per the os.Root documentation, an internal symlink that stays
// within the root is followed (this is a narrowing of the strictly
// "no symlinks at all" policy the L2 fix attempted — that stricter policy
// cannot be enforced this way without reintroducing a separate check-then-
// open race for a purely cosmetic property, since escape is already
// impossible through os.Root regardless of what any prior check observed).
// A lexical "../" rejection is kept ahead of os.Root purely for a clearer,
// more specific error message; os.Root's own containment enforcement does
// not depend on it and refuses an escaping reference either way.

func lexicallyEscapesRoot(rel string) bool {
	if rel == ".." {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(rel, ".."+sep) || strings.Contains(rel, sep+".."+sep) || strings.HasSuffix(rel, sep+"..")
}

// openRoot opens a.Root as an os.Root and returns the package/version
// relative path repo/version has been validated to represent. It does not
// itself prove containment beyond the lexical pre-check; os.Root's later
// methods (ReadFile, in load below) are what atomically enforce it against
// the real filesystem, on every individual call.
func (a LocalFirstParty) openRoot(repo, version string) (*os.Root, string, error) {
	if a.Root == "" {
		return nil, "", errors.New("local-first-party adapter requires an explicit Root")
	}
	if repo == "" || version == "" {
		return nil, "", errors.New("local-first-party package id and version are required")
	}
	rel := filepath.Join(repo, version)
	if lexicallyEscapesRoot(rel) {
		return nil, "", fmt.Errorf("local package %s/%s resolves outside the configured local package root", repo, version)
	}
	root, err := os.OpenRoot(a.Root)
	if err != nil {
		return nil, "", fmt.Errorf("open local-first-party root: %w", err)
	}
	return root, rel, nil
}

// load reads, pins, and cross-checks one local package version. Every
// returned Release has already been proven, byte-for-byte, to match its own
// pinned identity record; a mismatch anywhere fails closed with no partial
// result.
func (a LocalFirstParty) load(ref PackageRef, version string) (Release, []byte, error) {
	if ref.Source != SourceLocalFirstParty {
		return Release{}, nil, fmt.Errorf("local-first-party adapter cannot resolve source %q", ref.Source)
	}
	root, rel, err := a.openRoot(ref.Repo, version)
	if err != nil {
		return Release{}, nil, err
	}
	defer root.Close()
	pinnedBytes, err := root.ReadFile(filepath.Join(rel, "identity.json"))
	if err != nil {
		return Release{}, nil, fmt.Errorf("local package %s/%s: missing pinned identity: %w", ref.Repo, version, err)
	}
	var pinned localPinnedIdentity
	if err := json.Unmarshal(pinnedBytes, &pinned); err != nil || pinned.ManifestDigest == "" || pinned.ArtifactDigest == "" {
		return Release{}, nil, fmt.Errorf("local package %s/%s: pinned identity is malformed", ref.Repo, version)
	}
	manifestBytes, err := root.ReadFile(filepath.Join(rel, "manifest.json"))
	if err != nil {
		return Release{}, nil, fmt.Errorf("local package %s/%s: missing manifest: %w", ref.Repo, version, err)
	}
	if d := localDigest(manifestBytes); d != pinned.ManifestDigest {
		return Release{}, nil, fmt.Errorf("local package %s/%s: manifest on disk does not match its pinned identity (got %s, want %s)", ref.Repo, version, d, pinned.ManifestDigest)
	}
	artifactBytes, err := root.ReadFile(filepath.Join(rel, "artifact.tar.gz"))
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
	if sigBytes, err := root.ReadFile(filepath.Join(rel, "signature.json")); err == nil {
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
