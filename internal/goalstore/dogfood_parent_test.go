package goalstore

import (
	"context"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

// TestDogfoodParentGoal exercises the supported Goals/session and encrypted
// Goal Baseline interfaces for the post-release issue parent. It deliberately
// does not add a CLI or controller: the absence of that surface is recorded as
// a dogfood finding in the accompanying evidence.
func TestDogfoodParentGoal(t *testing.T) {
	repo, _ := repoFixture(t, crypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	ctx := context.Background()
	goalID := "dogfood-praxis-issues-96-plus"
	session, err := goals.NewSession(goalID, "Process open Praxis GitHub issues #96 and higher through Praxis itself, respecting dependencies, architecture authority, qualification boundaries, and the immutable v2.0.0 release candidate.")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		version string
		outcome string
	}{
		{"1", "captured"},
		{"2", "rigorous"},
		{"3", "ready"},
		{"4", "calibrate"},
		{"5", "review_all"},
	} {
		if err := session.Advance(step.outcome); err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveSession(ctx, session, step.version, time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if session.Stage != goals.StageDecide {
		t.Fatalf("unexpected resumable stage: %s", session.Stage)
	}

	resumed, err := repo.LoadSession(ctx, goalID, "5", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"ready", "ready", "ready", "ready"} {
		if err := resumed.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	baseline := resumed.Baseline
	baseline.RefinedOutcome = "A dependency-aware issue inventory, governed baseline, and Praxis-managed execution record for open issues #96-#103, with post-release boundaries and dogfood findings preserved."
	baseline.Rigor = goals.RigorRigorous
	baseline.RecommendationMode = goals.RecommendationReviewAll
	baseline.Scope = "GitHub issues 96 through 103"
	baseline.Constraints = []string{"qualified source and v2.0.0 release candidate are immutable", "post-release work uses isolated branches/worktrees", "issue dependencies and dogfood findings remain durable"}
	baseline.SuccessCriteria = []string{"all eight open issues are inventoried", "ready work is selected by dependency rather than number", "Praxis execution evidence distinguishes target defects from Praxis dogfood defects"}
	baseline.EvidenceRefs = []string{"github:convergent-systems-co/praxis/issues/96-103", "release-candidate:d87aba192d257dc2df8642ab48bc281f6a33249b", "qualified-source:c5c5e7937b5d1c7562a72d90d761cd630baf7369"}
	baseline.PlanRef = "docs/research/dogfood/praxis-issues-96-plus/issue-inventory-v1.md"
	if err := resumed.SetBaseline(baseline); err != nil {
		t.Fatal(err)
	}
	if err := resumed.Advance("stored"); err != nil {
		t.Fatal(err)
	}
	if resumed.Stage != goals.StageComplete {
		t.Fatalf("parent Goal did not complete: %s", resumed.Stage)
	}
	saved, err := repo.Finalize(ctx, FinalizeRequest{Baseline: resumed.Baseline, Persist: true, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Load(ctx, saved.Baseline.ID, saved.Baseline.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PRAXIS_GOAL_ID=%s PRAXIS_BASELINE=%s/%s digest=%s stage=%s", resumed.ID, loaded.ID, loaded.Version, loaded.Digest, resumed.Stage)
}
