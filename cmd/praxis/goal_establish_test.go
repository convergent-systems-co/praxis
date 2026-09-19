package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

func writeGoalEstablishment(t *testing.T, dir, name string, document map[string]any) (string, []byte) {
	t.Helper()
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, body
}

// TestGoalsLifecycleEstablishesExternalGoal proves that a caller supplies an
// outcome, not internal database or canonical-digest state. Praxis derives and
// persists generation 1 with exact source evidence and exposes inspection as
// the next supported lifecycle transition.
func TestGoalsLifecycleEstablishesExternalGoal(t *testing.T) {
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
	activateGoalsPackageFixture(t, ctx, db, built.Manifest, built.ArtifactBytes, governed, time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC))
	db.Close()

	dir := t.TempDir()
	document := map[string]any{
		"schema_version": "1", "goal_id": "goal:external-weather", "original_intent": "Build a useful weather service",
		"refined_outcome": "A tested weather service can be operated from its documented interface",
		"scope":           "the weather service repository", "success_criteria": []string{"the service returns current conditions", "the supported test suite passes"},
		"constraints": []string{"preserve existing public behavior"}, "validity_predicates": []string{"all completion evidence binds this generation"},
		"rigor": "structured", "recommendation_mode": "review_all",
	}
	path, body := writeGoalEstablishment(t, dir, "goal.json", document)
	out := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=establish", "--input=" + path}, governed); err != nil {
			t.Fatalf("establish: %v", err)
		}
	})
	var result struct {
		GoalID       string `json:"goal_id"`
		GoalVersion  string `json:"goal_version"`
		GoalDigest   string `json:"goal_digest"`
		SourceRef    string `json:"source_ref"`
		SourceDigest string `json:"source_digest"`
		Status       string `json:"status"`
		InspectWith  string `json:"inspect_with"`
		Replay       bool   `json:"replay"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode establish result: %v: %s", err, out)
	}
	sum := sha256.Sum256(body)
	wantSourceDigest := "sha256:" + hex.EncodeToString(sum[:])
	if result.GoalID != "goal:external-weather" || result.GoalVersion != "1" || result.GoalDigest == "" || result.SourceRef != path || result.SourceDigest != wantSourceDigest || result.Status != "authoritative" || result.Replay || !strings.Contains(result.InspectWith, "--goal-id=goal:external-weather --goal-version=1") {
		t.Fatalf("establishment did not expose the authoritative generation and next transition: %+v", result)
	}
	repo, opened, err := openGovernedRepository(ctx, governed)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := repo.Load(ctx, result.GoalID, result.GoalVersion, time.Now().UTC())
	opened.Close()
	if err != nil {
		t.Fatal(err)
	}
	if baseline.Digest != result.GoalDigest || baseline.ImportSourceRef != path || baseline.ImportSourceDigest != wantSourceDigest || baseline.WorkPlan != nil {
		t.Fatalf("durable Goal lineage differs from establishment: %+v", baseline)
	}

	replay := captureStdout(t, func() {
		if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=establish", "--input=" + path}, governed); err != nil {
			t.Fatalf("exact establishment replay: %v", err)
		}
	})
	if !strings.Contains(string(replay), `"replay": true`) {
		t.Fatalf("exact replay was not reported: %s", replay)
	}

	document["refined_outcome"] = "Changed requirements must not overwrite generation 1"
	changed, _ := writeGoalEstablishment(t, dir, "changed.json", document)
	if err := runDynamicInvocation(ctx, []string{"goals-lifecycle", "--operation=establish", "--input=" + changed}, governed); !errors.Is(err, goalstore.ErrBaselineImportConflict) {
		t.Fatalf("changed content under the completed identity must conflict, got %v", err)
	}
}

func TestGoalEstablishmentRejectsFabricatedState(t *testing.T) {
	base := `{"schema_version":"1","goal_id":"goal:guarded","original_intent":"admit an outcome","refined_outcome":"a governed Goal exists","success_criteria":["the Goal is durably inspectable"],"rigor":"direct","recommendation_mode":"review_all"}`
	for name, body := range map[string]string{
		"digest":         strings.TrimSuffix(base, "}") + `,"digest":"sha256:fake"}`,
		"decision":       strings.TrimSuffix(base, "}") + `,"decisions":[{"id":"fabricated","statement":"already decided","status":"resolved"}]}`,
		"work plan":      strings.TrimSuffix(base, "}") + `,"work_plan":{"candidates":[]}}`,
		"unknown":        strings.TrimSuffix(base, "}") + `,"hidden_state":true}`,
		"trailing":       base + `{}`,
		"underspecified": `{"schema_version":"1","goal_id":"goal:guarded","original_intent":"admit an outcome","refined_outcome":"a governed Goal exists","rigor":"direct","recommendation_mode":"review_all"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeGoalEstablishment([]byte(body)); err == nil {
				t.Fatal("fabricated or ambiguous state was accepted")
			}
		})
	}
}
