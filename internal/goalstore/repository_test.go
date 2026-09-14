package goalstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
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
	return praxiscrypto.WrappedKey{Ciphertext: []byte("wrapped"), SuiteID: "test", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (w *wrapper) Unwrap(context.Context, praxiscrypto.WrappedKey) ([]byte, error) {
	return append([]byte(nil), w.key...), nil
}

func repoFixture(t *testing.T, caps praxiscrypto.Capabilities, profile contracts.CryptoProfile) (Repository, *state.Store) {
	t.Helper()
	return repoFixtureAt(t, filepath.Join(t.TempDir(), "praxis.db"), caps, profile)
}

func repoFixtureAt(t *testing.T, path string, caps praxiscrypto.Capabilities, profile contracts.CryptoProfile) (Repository, *state.Store) {
	t.Helper()
	ctx := context.Background()
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := state.New(db)
	return Repository{Store: store, Crypto: praxiscrypto.EnvelopeService{Wrapper: &wrapper{caps: caps}}, KeyRef: "key:goals", Profile: profile, Sensitivity: state.SensitivityConfidential}, store
}

func goalFixture() goals.GoalBaseline {
	return goals.GoalBaseline{ID: "goal-1", Version: "1", OriginalIntent: "Help me publish article two", RefinedOutcome: "Produce a publishable second AI-safety article with reusable research context", Rigor: goals.RigorRigorous, RecommendationMode: goals.RecommendationDelegated, Decisions: []goals.Decision{{ID: "d1", Statement: "reuse prior research baseline", Status: goals.DecisionResolved, Recommendation: "reuse", Rationale: "avoids repeated discovery", Reversible: true, AutoAccepted: true}}}
}

func TestRepositoryEncryptedRoundTrip(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Now().UTC()
	saved, err := repo.Save(context.Background(), goalFixture(), now, nil)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Digest == "" {
		t.Fatal("saved baseline must have digest")
	}
	loaded, err := repo.Load(context.Background(), saved.ID, saved.Version, now)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Digest != saved.Digest || loaded.OriginalIntent != saved.OriginalIntent {
		t.Fatalf("round trip mismatch saved=%+v loaded=%+v", saved, loaded)
	}
}

func TestRepositoryPQRequiredFailsWithoutPQProvider(t *testing.T) {
	repo, store := repoFixture(t, praxiscrypto.Capabilities{Classical: true}, contracts.CryptoPQRequired)
	_, err := repo.Save(context.Background(), goalFixture(), time.Now().UTC(), nil)
	if err == nil {
		t.Fatal("pq-required Goal Baseline must not persist through classical-only wrapper")
	}
	if _, err := store.GetSecureBlob(context.Background(), baselineNamespace, "goal-1", "1", time.Now().UTC()); err != state.ErrSecureBlobNotFound {
		t.Fatalf("failed encryption must not leave persisted record, got %v", err)
	}
}

func TestRepositoryRejectsBaselineDigestMutationBeforePersistence(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	b := goalFixture()
	d, _ := b.ComputeDigest()
	b.Digest = d
	b.RefinedOutcome = "changed after digest"
	if _, err := repo.Save(context.Background(), b, time.Now().UTC(), nil); err == nil {
		t.Fatal("mutated baseline must not persist")
	}
}

func TestRepositoryHonorsRetentionExpiry(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	saved, err := repo.Save(context.Background(), goalFixture(), now, &expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Load(context.Background(), saved.ID, saved.Version, expiry); err != state.ErrSecureBlobExpired {
		t.Fatalf("expected expired baseline, got %v", err)
	}
}

func TestRepositorySessionCheckpointSurvivesRestartAndResumes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "praxis.db")
	ctx := context.Background()
	keyWrapper := &wrapper{caps: praxiscrypto.Capabilities{PQ: true}}
	db, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	store := state.New(db)
	repo := Repository{Store: store, Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	session, err := goals.NewSession("session-1", "Help me make a reliable research plan")
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []string{"captured", "rigorous", "ready", "calibrate", "review_all"} {
		if err := session.Advance(outcome); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.SaveSession(context.Background(), session, "5", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}

	// Close the provider and reopen the same durable database, as a process
	// restart would. The key wrapper is retained only as the test's simulated
	// external key service; plaintext session state is not retained in memory.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedDB, err := state.OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedDB.Close()
	reopened := Repository{Store: state.New(reopenedDB), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	resumed, err := reopened.LoadSession(ctx, session.ID, "5", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Stage != goals.StageDecide || resumed.StageOutcomes[goals.StageCalibrate] != "review_all" {
		t.Fatalf("checkpoint did not preserve resumable responsibility state: %+v", resumed)
	}
	if err := resumed.Advance("ready"); err != nil {
		t.Fatal(err)
	}
}

func TestRepositorySessionCheckpointVersionsAreImmutable(t *testing.T) {
	repo, _ := repoFixture(t, praxiscrypto.Capabilities{PQ: true}, contracts.CryptoPQRequired)
	session, err := goals.NewSession("session-immutable", "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSession(context.Background(), session, "1", time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSession(context.Background(), session, "1", time.Now().UTC(), nil); err == nil {
		t.Fatal("reusing a checkpoint version must not replace the immutable snapshot")
	}
}
