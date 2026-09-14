package distribution

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

type ResolvedIdentity struct {
	PackageID      string `json:"package_id"`
	PackageVersion string `json:"package_version"`
	ContentDigest  string `json:"content_digest"`
	VerificationID string `json:"verification_id"`
}

type Resolution struct {
	Root         packagecatalog.VerifiedPackage
	Dependencies map[string]packagecatalog.VerifiedPackage
	Order        []ResolvedIdentity
}

// Resolver traverses immutable dependency locks through replaceable transport
// adapters. Transport metadata locates bytes; it cannot weaken signed identity,
// signature, cycle, conflict, or transitive verification rules.
type Resolver struct {
	Sources         map[string]LockedAdapter
	Verifiers       []packagecatalog.SignatureVerifier
	AllowPQFallback bool
	VerifiedAt      time.Time
}

func (r Resolver) Resolve(ctx context.Context, root Release, rootArtifact []byte) (Resolution, error) {
	if len(root.ManifestBytes) == 0 || len(rootArtifact) == 0 || r.VerifiedAt.IsZero() || len(r.Verifiers) == 0 {
		return Resolution{}, errors.New("root release bytes, verification time, and signature verifiers are required")
	}
	resolved := map[string]packagecatalog.VerifiedPackage{}
	closures := map[string]map[string]packagecatalog.VerifiedPackage{}
	identities := map[string]packagecatalog.Dependency{}
	visiting := map[string]bool{}
	order := make([]ResolvedIdentity, 0)

	var walk func(Release, []byte, string, string) (packagecatalog.VerifiedPackage, map[string]packagecatalog.VerifiedPackage, error)
	walk = func(release Release, artifact []byte, sourceKind, sourceRef string) (packagecatalog.VerifiedPackage, map[string]packagecatalog.VerifiedPackage, error) {
		manifest := release.Manifest
		identity := packagecatalog.Dependency{PackageID: manifest.PackageID, Version: manifest.Version, Digest: manifest.ContentDigest, SourceKind: sourceKind, SourceRef: sourceRef}
		if prior, ok := identities[manifest.PackageID]; ok {
			if prior.Version != identity.Version || prior.Digest != identity.Digest {
				return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("conflicting dependency locks for %q", manifest.PackageID)
			}
			if verified, exists := resolved[manifest.PackageID]; exists {
				return verified, cloneVerifiedPackages(closures[manifest.PackageID]), nil
			}
		}
		identities[manifest.PackageID] = identity
		if visiting[manifest.PackageID] {
			return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("dependency cycle at %q", manifest.PackageID)
		}
		visiting[manifest.PackageID] = true
		deps := map[string]packagecatalog.VerifiedPackage{}
		locks := append([]packagecatalog.Dependency(nil), manifest.Dependencies...)
		sort.Slice(locks, func(i, j int) bool {
			if locks[i].PackageID != locks[j].PackageID {
				return locks[i].PackageID < locks[j].PackageID
			}
			return locks[i].Version < locks[j].Version
		})
		for _, lock := range locks {
			if lock.SourceKind == "" || lock.SourceRef == "" {
				return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("dependency %q lacks a transport locator", lock.PackageID)
			}
			adapter := r.Sources[lock.SourceKind]
			if adapter == nil {
				return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("dependency %q uses unavailable source %q", lock.PackageID, lock.SourceKind)
			}
			depRelease, err := adapter.ResolveLocked(ctx, lock)
			if err != nil {
				return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("resolve dependency %q: %w", lock.PackageID, err)
			}
			if depRelease.Manifest.PackageID != lock.PackageID || depRelease.Manifest.Version != lock.Version || depRelease.Manifest.ContentDigest != lock.Digest {
				return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("resolved dependency %q does not match immutable lock", lock.PackageID)
			}
			depArtifact, err := adapter.FetchArtifact(ctx, depRelease)
			if err != nil {
				return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("fetch dependency %q: %w", lock.PackageID, err)
			}
			verified, transitive, err := walk(depRelease, depArtifact, lock.SourceKind, lock.SourceRef)
			if err != nil {
				return packagecatalog.VerifiedPackage{}, nil, err
			}
			deps[lock.PackageID] = verified
			for transitiveID, transitivePackage := range transitive {
				deps[transitiveID] = transitivePackage
			}
		}
		verified, err := packagecatalog.VerifyPackage(packagecatalog.VerificationInput{
			ManifestBytes: release.ManifestBytes, ArtifactBytes: artifact, Signature: release.Signature,
			ResolvedDependencies: deps, SourceKind: sourceKind, SourceRef: sourceRef,
			VerifiedAt: r.VerifiedAt, AllowPQFallback: r.AllowPQFallback,
		}, r.Verifiers)
		if err != nil {
			return packagecatalog.VerifiedPackage{}, nil, fmt.Errorf("verify package %q: %w", manifest.PackageID, err)
		}
		visiting[manifest.PackageID] = false
		resolved[manifest.PackageID] = verified
		closures[manifest.PackageID] = cloneVerifiedPackages(deps)
		order = append(order, ResolvedIdentity{PackageID: manifest.PackageID, PackageVersion: manifest.Version, ContentDigest: manifest.ContentDigest, VerificationID: verified.Evidence().ID})
		return verified, cloneVerifiedPackages(deps), nil
	}

	rootSourceKind, rootSourceRef := root.Ref.Source, root.Ref.String()
	verifiedRoot, dependencies, err := walk(root, rootArtifact, rootSourceKind, rootSourceRef)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Root: verifiedRoot, Dependencies: dependencies, Order: order}, nil
}

func cloneVerifiedPackages(in map[string]packagecatalog.VerifiedPackage) map[string]packagecatalog.VerifiedPackage {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]packagecatalog.VerifiedPackage, len(in))
	for id, pkg := range in {
		out[id] = pkg
	}
	return out
}
