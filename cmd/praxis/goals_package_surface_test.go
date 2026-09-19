package main

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goaldrive"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// activateGoalsPackageFixture builds the canonical Goals package from the
// repository's own package definition, signs it with a fixture publisher key,
// verifies it against that trusted key, and activates it through the normal
// approval-gated registry path. It returns the activated manifest.
func activateGoalsPackageFixture(t *testing.T, ctx context.Context, db *sql.DB, manifest packagecatalog.Manifest, artifact []byte, base func(string) string, now time.Time) packagecatalog.Manifest {
	t.Helper()
	release, artifact, trusted := signedManifestRelease(t, manifest, artifact, contracts.CryptoClassicalCompatible)
	verified, err := verifyReleasePackage(release, artifact, func(key string) string {
		if key == "PRAXIS_TRUSTED_KEYS" {
			return trusted
		}
		if base != nil {
			return base(key)
		}
		return ""
	}, false, now)
	if err != nil {
		t.Fatal(err)
	}
	actor := contracts.PrincipalRef{ID: "operator", Kind: "user"}
	intent, err := packagecatalog.NewActivationIntent(verified, actor)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	approvalID := "approval:" + manifest.PackageID + "@" + manifest.Version
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, approvalID, actor.ID, actor.Kind, digest, now.Add(-time.Second).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := state.New(db).ActivatePackage(ctx, packagecatalog.ActivationRequest{Package: verified, Intent: intent, ApprovalID: approvalID}, now); err != nil {
		t.Fatal(err)
	}
	return release.Manifest
}

func transitionPackageFixture(t *testing.T, ctx context.Context, db *sql.DB, manifest packagecatalog.Manifest, operation packagecatalog.TransitionOperation, now time.Time) {
	t.Helper()
	actor := contracts.PrincipalRef{ID: "operator", Kind: "user"}
	transition, err := packagecatalog.NewTransitionRequest(packagecatalog.PackageIdentity{PackageID: manifest.PackageID, Version: manifest.Version, ContentDigest: manifest.ContentDigest}, operation, actor, "approval:"+string(operation)+":"+manifest.Version)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := transition.Intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO approvals(approval_id,approver_id,approver_kind,intent_digest,issued_at,remaining_uses) VALUES(?,?,?,?,?,1)`, transition.ApprovalID, actor.ID, actor.Kind, digest, now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if err := state.New(db).TransitionPackage(ctx, transition, now); err != nil {
		t.Fatal(err)
	}
}

// TestGoalsPackageExposesGoalDriveThroughInstalledSurface proves the Gap 2
// repair: the canonical Goals package (0.1.1) exposes goal-drive through its
// manifest, the generic dynamic invocation path resolves it via the installed
// package generation, the first-party handler fails closed at every guard
// short of governed execution, and package disable/remove withdraws the
// surface without any kernel command.
func TestGoalsPackageExposesGoalDriveThroughInstalledSurface(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)
	input, err := goals.PackageBuildInput([]byte("fixture-goals-plugin-executable"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	if built.Manifest.Version != goals.PackageVersion {
		t.Fatalf("canonical Goals package version is %q, want %s", built.Manifest.Version, goals.PackageVersion)
	}
	activated := activateGoalsPackageFixture(t, ctx, db, built.Manifest, built.ArtifactBytes, nil, now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "PRAXIS_DB" {
			return path
		}
		return ""
	}

	// Goals lifecycle alias resolves through the installed package.
	lifecycle, err := resolveDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=goal-1", "--goal-version=1"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle.EntryPointID != "goals-lifecycle" || lifecycle.PackageID != goals.PackageID || lifecycle.PackageVersion != goals.PackageVersion || lifecycle.PackageDigest != activated.ContentDigest {
		t.Fatalf("goals-lifecycle did not resolve to the installed generation: %+v", lifecycle)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"goals-lifecycle"}, getenv); err == nil || !strings.Contains(err.Error(), `required option "operation"`) {
		t.Fatalf("goals-lifecycle must still require --operation: %v", err)
	}

	// goal-drive resolves through the installed package, not a kernel command.
	drive, err := resolveDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal-1", "--goal-version=1", "--provider=local", "--invocation-id=inv-1"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if drive.EntryPointID != "goal-drive" || drive.PackageID != goals.PackageID || drive.PackageVersion != goals.PackageVersion || drive.PackageDigest != activated.ContentDigest || drive.GraphID != "praxis.package.goals.default" || drive.GraphVersion != "0.3.0" {
		t.Fatalf("goal-drive did not resolve to the installed generation: %+v", drive)
	}

	// Dispatch through the generic path reaches the registered first-party
	// handler and fails closed at each guard, in order.
	// goal-drive runs an existing durable Goal only (#103, ADR-101): the
	// successor contract no longer declares prose input, so the registry
	// refuses it before any handler runs.
	for _, prose := range []string{"--goal=free text", "--goal-file=/tmp/GOAL.md"} {
		if err := runDynamicInvocation(ctx, []string{"goal-drive", prose, "--provider=local", "--invocation-id=inv-1"}, getenv); err == nil || !strings.Contains(err.Error(), "unknown option") {
			t.Fatalf("prose goal input %q must not execute: %v", prose, err)
		}
	}
	if err := runDynamicInvocation(ctx, []string{"goal-drive", "--provider=local", "--invocation-id=inv-1"}, getenv); err == nil {
		t.Fatal("goal-drive without an existing Goal identity must not execute")
	}
	if err := runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal-1", "--provider=local", "--invocation-id=inv-1"}, getenv); !errors.Is(err, goaldrive.ErrExactGoalVersionRequired) {
		t.Fatalf("goal-drive must require an exact durable Goal version: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal-1", "--goal-version=1", "--invocation-id=inv-1"}, getenv); err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("goal-drive must require a provider: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal-1", "--goal-version=1", "--provider=local", "--invocation-id=inv-1", "--repo=" + dir, "--branch=main"}, getenv); !errors.Is(err, errGoalDriveDispatchDependencies) {
		t.Fatalf("goal-drive must not execute without governed state and registered providers: %v", err)
	}

	// Disabling the package withdraws both aliases; removal keeps them gone.
	db, err = state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	transitionPackageFixture(t, ctx, db, activated, packagecatalog.TransitionDisable, now.Add(time.Minute))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, alias := range [][]string{{"goal-drive", "--goal-id=goal-1", "--goal-version=1", "--provider=local", "--invocation-id=inv-1"}, {"goals-lifecycle", "--operation=inspect"}} {
		if _, err := resolveDynamicInvocation(ctx, alias, getenv); err == nil {
			t.Fatalf("%s resolved after package disable", alias[0])
		}
		if err := runDynamicInvocation(ctx, alias, getenv); err == nil {
			t.Fatalf("%s dispatched after package disable", alias[0])
		}
	}
	db, err = state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	transitionPackageFixture(t, ctx, db, activated, packagecatalog.TransitionRemove, now.Add(2*time.Minute))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal-1", "--goal-version=1", "--provider=local", "--invocation-id=inv-1"}, getenv); err == nil {
		t.Fatal("goal-drive resolved after package removal")
	}
}

// TestPublishedGoalsLifecycleOnlyManifestDoesNotExposeGoalDrive records the
// published goals/v0.1.0 shape: a manifest that lists only goals-lifecycle
// cannot resolve goal-drive, which is why the successor 0.1.1 is required.
func TestPublishedGoalsLifecycleOnlyManifestDoesNotExposeGoalDrive(t *testing.T) {
	ctx := context.Background()
	// A goals@0.1.0-labelled release is the first-party publication identity,
	// so verification consults the governed installation for publication
	// lineage; this consumer installation holds none and verifies by key.
	governed, _ := governedInstallationFixture(t, ctx)
	path := governed("PRAXIS_DB")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)
	lifecycle := goals.LifecycleInvocation()
	lifecycle.PackageVersion = "0.1.0"
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: goals.PackageID, Version: "0.1.0", Publisher: "publisher:praxis-first-party", Invocations: []contracts.InvocationContract{lifecycle}}
	activateGoalsPackageFixture(t, ctx, db, manifest, []byte("published-0.1.0-shape"), governed, now)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	getenv := func(key string) string {
		if key == "PRAXIS_DB" {
			return path
		}
		return ""
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect"}, getenv); err != nil {
		t.Fatalf("goals-lifecycle must resolve from the 0.1.0 shape: %v", err)
	}
	if _, err := resolveDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal-1", "--goal-version=1", "--provider=local", "--invocation-id=inv-1"}, getenv); err == nil {
		t.Fatal("goal-drive resolved from a manifest that does not expose it")
	}
}
