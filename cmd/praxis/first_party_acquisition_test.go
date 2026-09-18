package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/distribution"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestFirstPartyReleaseVerifiesByTrustedKeyWithoutPublicationLineage proves
// that a consumer installation (one that never performed a Goals publication)
// admits a release from the first-party package repository only through
// trusted-key signature verification: the publisher-local acquisition
// lineage check neither blocks the consumer nor replaces the signature.
func TestFirstPartyReleaseVerifiesByTrustedKeyWithoutPublicationLineage(t *testing.T) {
	ctx := context.Background()
	governed, _ := governedInstallationFixture(t, ctx)
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "praxis.package.goals", Version: "0.1.1", Publisher: "publisher:praxis-first-party", Invocations: []contracts.InvocationContract{{Version: contracts.InvocationContractCurrentVersion(), PackageID: "praxis.package.goals", PackageVersion: "0.1.1", GraphID: "praxis.package.goals.default", GraphVersion: "0.3.0", EntryPointID: "goal-drive", Aliases: []string{"goal-drive"}}}}
	release, artifact, trusted := signedManifestRelease(t, manifest, []byte("goals-0.1.1-archive"), contracts.CryptoClassicalCompatible)
	release.Ref = distribution.PackageRef{Source: "github-releases", Owner: "convergent-systems-co", Repo: "praxis-packages"}
	release.Tag = "goals/v0.1.1"
	if release.Ref.String() != contracts.GoalsPublicationRepository {
		t.Fatalf("fixture locator %q is not the first-party repository", release.Ref.String())
	}
	now := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)
	withKeys := func(keys string) func(string) string {
		return func(key string) string {
			if key == "PRAXIS_TRUSTED_KEYS" {
				return keys
			}
			return governed(key)
		}
	}
	verified, err := verifyReleasePackage(release, artifact, withKeys(trusted), false, now)
	if err != nil {
		t.Fatalf("consumer installation must verify a first-party release by trusted key: %v", err)
	}
	if verified.Manifest().PackageID != "praxis.package.goals" || verified.Manifest().Version != "0.1.1" {
		t.Fatalf("verified identity: %+v", verified.Manifest())
	}
	if _, err := verifyReleasePackage(release, artifact, withKeys(`{"other":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="}`), false, now); err == nil || !strings.Contains(err.Error(), "verify package") {
		t.Fatalf("untrusted publisher key must still fail signature verification: %v", err)
	}
	// Without a governed installation the first-party locator still cannot
	// be admitted: lineage presence is decided from durable state, never
	// assumed from the absence of configuration.
	if _, err := verifyReleasePackage(release, artifact, func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" {
			return trusted
		}
		return ""
	}, false, now); err == nil {
		t.Fatal("first-party release verified without an installation to consult")
	}
}
