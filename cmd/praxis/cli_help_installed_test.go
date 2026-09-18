package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
)

// TestCLIHelpResolvesInstalledEntryPointsFromTheRegistry proves that help for
// an installed package entry point is served from the active invocation
// registry (the same source the dynamic dispatcher resolves from), that root
// help lists the installed aliases, that unknown names still fail, and that
// after package removal the entry point is unknown to help again.
func TestCLIHelpResolvesInstalledEntryPointsFromTheRegistry(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "praxis.db")
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PRAXIS_DB", path)
	var out bytes.Buffer
	if handled, err := dispatchCLIHelp([]string{"goals-lifecycle", "--help"}, &out); !handled || err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("entry point must be unknown before installation: %v %v", handled, err)
	}
	input, err := goals.PackageBuildInput([]byte("fixture-goals-plugin-executable"))
	if err != nil {
		t.Fatal(err)
	}
	built, err := packagecatalog.BuildPackage(input)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 18, 20, 0, 0, 0, time.UTC)
	activated := activateGoalsPackageFixture(t, ctx, db, built.Manifest, built.ArtifactBytes, nil, now)
	db.Close()
	for _, args := range [][]string{{"goals-lifecycle", "--help"}, {"help", "goals-lifecycle"}, {"goal-drive", "-h"}, {"help", "goal-drive"}} {
		out.Reset()
		handled, err := dispatchCLIHelp(args, &out)
		if !handled || err != nil {
			t.Fatalf("%v: installed entry point help must be served: %v %v", args, handled, err)
		}
		text := out.String()
		if !strings.Contains(text, "usage: praxis "+args[len(args)-1]) && !strings.Contains(text, "usage: praxis "+args[0]) {
			t.Fatalf("%v: usage missing: %s", args, text)
		}
		if !strings.Contains(text, "praxis.package.goals@0.1.1") {
			t.Fatalf("%v: package identity missing: %s", args, text)
		}
	}
	out.Reset()
	if _, err := dispatchCLIHelp([]string{"goals-lifecycle", "--help"}, &out); err != nil || !strings.Contains(out.String(), "--operation <enum> (required)") || !strings.Contains(out.String(), "--input <path>") {
		t.Fatalf("goals-lifecycle options must come from the registered contract: %v %s", err, out.String())
	}
	out.Reset()
	if _, err := dispatchCLIHelp([]string{"goal-drive", "--help"}, &out); err != nil || !strings.Contains(out.String(), "--goal-version <string>") || !strings.Contains(out.String(), "--mode <enum>") {
		t.Fatalf("goal-drive options must come from the registered contract: %v %s", err, out.String())
	}
	out.Reset()
	if handled, err := dispatchCLIHelp(nil, &out); !handled || err != nil || !strings.Contains(out.String(), "Installed entry points: goal-drive, goals-lifecycle") {
		t.Fatalf("root help must list installed entry points: %v %v %s", handled, err, out.String())
	}
	out.Reset()
	if handled, err := dispatchCLIHelp([]string{"missing", "--help"}, &out); !handled || err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unknown names must still be refused: %v %v", handled, err)
	}
	out.Reset()
	if handled, err := dispatchCLIHelp([]string{"publisher", "goals-lifecycle", "--help"}, &out); !handled || err == nil {
		t.Fatalf("installed entry points are root commands only: %v %v", handled, err)
	}
	db, err = state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	transitionPackageFixture(t, ctx, db, activated, packagecatalog.TransitionDisable, now.Add(time.Minute))
	db.Close()
	out.Reset()
	if handled, err := dispatchCLIHelp([]string{"goals-lifecycle", "--help"}, &out); !handled || err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("disabled package entry point must be unknown to help: %v %v", handled, err)
	}
	out.Reset()
	if _, err := dispatchCLIHelp(nil, &out); err != nil || !strings.Contains(out.String(), "Installed entry points: none") {
		t.Fatalf("root help after disable: %v %s", err, out.String())
	}
}
