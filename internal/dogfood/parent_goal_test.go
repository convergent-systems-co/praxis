package dogfood

import (
	"context"
	"errors"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type wrapper struct {
	caps praxiscrypto.Capabilities
	key  []byte
}

func (w *wrapper) Capabilities(context.Context, string) (praxiscrypto.Capabilities, error) {
	return w.caps, nil
}
func (w *wrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, key []byte) (praxiscrypto.WrappedKey, error) {
	w.key = append([]byte(nil), key...)
	return praxiscrypto.WrappedKey{Ciphertext: []byte("wrapped"), SuiteID: "dogfood-test", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (w *wrapper) Unwrap(context.Context, praxiscrypto.WrappedKey) ([]byte, error) {
	if len(w.key) == 0 {
		return nil, errors.New("dogfood key is unavailable")
	}
	return append([]byte(nil), w.key...), nil
}

// TestParentGoal exercises the supported Goals/session and encrypted Goal
// Baseline interfaces without changing an attested runtime package digest.
func TestParentGoal(t *testing.T) {
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, t.TempDir()+"/praxis.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := goalstore.Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	goalID := "dogfood-praxis-issues-96-plus"
	session, err := goals.NewSession(goalID, "Process open Praxis GitHub issues #96 and higher through Praxis itself, respecting dependencies, architecture authority, qualification boundaries, and the immutable v2.0.0 release candidate.")
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ version, outcome string }{{"1", "captured"}, {"2", "rigorous"}, {"3", "ready"}, {"4", "calibrate"}, {"5", "review_all"}} {
		if err := session.Advance(step.outcome); err != nil {
			t.Fatal(err)
		}
		if err := repo.SaveSession(ctx, session, step.version, time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
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
	saved, err := repo.Finalize(ctx, goalstore.FinalizeRequest{Baseline: resumed.Baseline, Persist: true, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := repo.Load(ctx, saved.Baseline.ID, saved.Baseline.Version, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("PRAXIS_GOAL_ID=%s PRAXIS_BASELINE=%s/%s digest=%s stage=%s", resumed.ID, loaded.ID, loaded.Version, loaded.Digest, resumed.Stage)
}
