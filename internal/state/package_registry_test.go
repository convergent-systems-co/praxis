package state

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

func fixturePackage(id, version, digest, alias string) packagecatalog.Manifest {
	return packagecatalog.Manifest{
		PackageID: id, Version: version, ContentDigest: digest,
		Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentGraph, ID: id + ".graph", Version: version, Digest: "sha256:graph-" + version, Artifact: "graphs/default.json"}},
		Invocations: []contracts.InvocationContract{{
			Version: "v1", PackageID: id, PackageVersion: version,
			GraphID: id + ".graph", GraphVersion: version,
			EntryPointID: id + ".default", Aliases: []string{alias},
			Options: []contracts.InvocationOption{{Name: "flag", Type: "bool"}},
		}},
	}
}

func TestPackageActivationPublishesAndRemovesAliasesAndContents(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	s := New(db)
	m := fixturePackage("example/pkg", "1.0.0", "sha256:one", "example")
	if err := s.ActivatePackage(ctx, m, "github-release", "owner/repo@v1.0.0", time.Now().UTC()); err != nil { t.Fatal(err) }
	got, err := s.ResolveInvocationAlias(ctx, "example")
	if err != nil { t.Fatal(err) }
	if got.Contract.PackageID != m.PackageID || got.ContentDigest != m.ContentDigest { t.Fatalf("wrong registered package: %#v", got) }
	content, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1.0.0")
	if err != nil { t.Fatal(err) }
	if content.PackageID != m.PackageID || content.Content.Digest != "sha256:graph-1.0.0" { t.Fatalf("wrong graph registration: %#v", content) }
	if err := s.DeactivatePackage(ctx, m.PackageID); err != nil { t.Fatal(err) }
	if _, err := s.ResolveInvocationAlias(ctx, "example"); err == nil { t.Fatal("disabled package alias must disappear") }
	if _, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1.0.0"); err == nil { t.Fatal("disabled package graph must disappear") }
}

func TestAgentOnlyAndMixedPackagesAreFirstClassContents(t *testing.T) {
	ctx := context.Background()
	db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db"))
	if err != nil { t.Fatal(err) }
	defer db.Close()
	s := New(db)
	agentOnly := packagecatalog.Manifest{PackageID: "agent/pkg", Version: "1", ContentDigest: "sha256:a", Contents: []packagecatalog.ContentRef{{Kind: packagecatalog.ContentAgentDefinition, ID: "researcher", Version: "1", Digest: "sha256:agent", Artifact: "agents/researcher.json"}}}
	if err := s.ActivatePackage(ctx, agentOnly, "test", "agent", time.Now().UTC()); err != nil { t.Fatal(err) }
	agents, err := s.ActiveContents(ctx, packagecatalog.ContentAgentDefinition)
	if err != nil || len(agents) != 1 || agents[0].Content.ID != "researcher" { t.Fatalf("agent contents: %#v err=%v", agents, err) }
	mixed := packagecatalog.Manifest{PackageID: "mixed/pkg", Version: "1", ContentDigest: "sha256:m", Contents: []packagecatalog.ContentRef{
		{Kind: packagecatalog.ContentGraph, ID: "mixed.graph", Version: "1", Digest: "sha256:g", Artifact: "graphs/mixed.json"},
		{Kind: packagecatalog.ContentPlugin, ID: "mixed.provider", Version: "1", Digest: "sha256:p", Artifact: "plugins/provider"},
	}}
	if err := s.ActivatePackage(ctx, mixed, "test", "mixed", time.Now().UTC()); err != nil { t.Fatal(err) }
	plugins, err := s.ActiveContents(ctx, packagecatalog.ContentPlugin)
	if err != nil || len(plugins) != 1 || plugins[0].Content.ID != "mixed.provider" { t.Fatalf("plugin contents: %#v err=%v", plugins, err) }
}

func TestPackageCannotShadowCoreCommand(t *testing.T) {
	ctx := context.Background(); db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db")); if err != nil { t.Fatal(err) }; defer db.Close()
	if err := New(db).ActivatePackage(ctx, fixturePackage("bad/pkg", "1", "sha256:x", "status"), "github-release", "x", time.Now().UTC()); err == nil { t.Fatal("reserved core command must be rejected") }
}

func TestPackageAliasCollisionFailsWithoutChangingExistingOwner(t *testing.T) {
	ctx := context.Background(); db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db")); if err != nil { t.Fatal(err) }; defer db.Close(); s := New(db); now := time.Now().UTC()
	if err := s.ActivatePackage(ctx, fixturePackage("a/pkg", "1", "sha256:a", "shared"), "github-release", "a", now); err != nil { t.Fatal(err) }
	if err := s.ActivatePackage(ctx, fixturePackage("b/pkg", "1", "sha256:b", "shared"), "github-release", "b", now); err == nil { t.Fatal("alias collision must fail") }
	got, err := s.ResolveInvocationAlias(ctx, "shared"); if err != nil { t.Fatal(err) }; if got.Contract.PackageID != "a/pkg" { t.Fatalf("existing alias owner changed: %s", got.Contract.PackageID) }
}

func TestPackageUpdateAtomicallyReplacesAliasAndContentSurface(t *testing.T) {
	ctx := context.Background(); db, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "praxis.db")); if err != nil { t.Fatal(err) }; defer db.Close(); s := New(db); now := time.Now().UTC()
	v1 := fixturePackage("example/pkg", "1", "sha256:1", "old"); if err := s.ActivatePackage(ctx, v1, "github-release", "v1", now); err != nil { t.Fatal(err) }
	v2 := fixturePackage("example/pkg", "2", "sha256:2", "new"); if err := s.ActivatePackage(ctx, v2, "github-release", "v2", now.Add(time.Second)); err != nil { t.Fatal(err) }
	if _, err := s.ResolveInvocationAlias(ctx, "old"); err == nil { t.Fatal("old alias must not remain active") }
	if _, err := s.ResolveContent(ctx, packagecatalog.ContentGraph, "example/pkg.graph", "1"); err == nil { t.Fatal("old graph generation must not remain active") }
	got, err := s.ResolveInvocationAlias(ctx, "new"); if err != nil { t.Fatal(err) }; if got.Contract.PackageVersion != "2" || got.ContentDigest != "sha256:2" { t.Fatalf("new generation not active: %#v", got) }
}
