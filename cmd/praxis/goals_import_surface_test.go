package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

func writeBaselineImportDocument(t *testing.T, dir, name string, baseline goals.GoalBaseline, sourceRef string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if sourceRef == "" {
		sourceRef = path
	}
	canonical, err := baseline.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	baseline.Digest = digest
	body, err := json.MarshalIndent(map[string]any{"schema_version": "1", "source_ref": sourceRef, "source_digest": digest, "baseline": baseline}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestGoalsLifecycleImportReachesTheImportBoundary proves the ADR-068 import
// boundary is reachable through the installed Goals package: a canonical
// baseline document imports through goals-lifecycle, inspect then resolves
// it, a document that does not bind its own path is refused, and goal-drive
// on the imported Goal proceeds to the worker dependency gate.
func TestGoalsLifecycleImportReachesTheImportBoundary(t *testing.T) {
	ctx := context.Background()
	governed, _ := governedInstallationFixture(t, ctx)
	db, err := state.OpenSQLite(ctx, governed("PRAXIS_DB"))
	if err != nil {
		t.Fatal(err)
	}
	input, err := goals.PackageBuildInput([]byte("fixture-goals-plugin-executable"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	activateGoalsPackageFixture(t, ctx, db, built.Manifest, built.ArtifactBytes, governed, time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC))
	db.Close()
	dir := t.TempDir()
	baseline := goals.GoalBaseline{ID: "goal:scratch-weather", Version: "1", OriginalIntent: "Prove the external bootstrap path", RefinedOutcome: "One small Goal drives one real turn", Rigor: goals.RigorDirect, RecommendationMode: goals.RecommendationReviewAll}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import"}, governed); err == nil || !strings.Contains(err.Error(), "--input") {
		t.Fatalf("import without a document must be refused: %v", err)
	}
	wrong := writeBaselineImportDocument(t, dir, "wrong-ref.json", baseline, "/elsewhere/baseline.json")
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + wrong}, governed); err == nil || !strings.Contains(err.Error(), "source_ref") {
		t.Fatalf("document not binding its own path must be refused: %v", err)
	}
	path := writeBaselineImportDocument(t, dir, "baseline.json", baseline, "")
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + path}, governed); err != nil {
		t.Fatalf("canonical import through the package surface failed: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + path}, governed); err != nil {
		t.Fatalf("exact duplicate import must be idempotent: %v", err)
	}
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=inspect", "--goal-id=goal:scratch-weather", "--goal-version=1"}, governed); err != nil {
		t.Fatalf("imported Goal must be inspectable: %v", err)
	}
	conflicting := baseline
	conflicting.RefinedOutcome = "A different outcome under the same generation"
	conflict := writeBaselineImportDocument(t, dir, "conflict.json", conflicting, "")
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=import", "--input=" + conflict}, governed); err == nil {
		t.Fatal("conflicting generation must be refused")
	}
	// goal-drive resolves through the installed package and reaches the
	// controller's runtime construction; the fixture bootstrap provider is
	// not a real key provider, so construction fails closed there.
	err = runDynamicInvocation(ctx, []string{"goal-drive", "--goal-id=goal:scratch-weather", "--goal-version=1", "--provider=local", "--invocation-id=inv-1", "--repo=" + dir, "--branch=main"}, governed)
	if err == nil || !strings.Contains(err.Error(), "construct native Goal-drive runtime") {
		t.Fatalf("goal-drive on the imported Goal must reach runtime construction: %v", err)
	}
}
