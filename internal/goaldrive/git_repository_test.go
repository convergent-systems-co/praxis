package goaldrive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	praxiscrypto "github.com/convergent-systems-co/praxis/internal/crypto"
	"github.com/convergent-systems-co/praxis/internal/goalstore"
	"github.com/convergent-systems-co/praxis/internal/state"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

type runtimeKeyWrapper struct{ key []byte }

func (w *runtimeKeyWrapper) Capabilities(context.Context, string) (praxiscrypto.Capabilities, error) {
	return praxiscrypto.Capabilities{PQ: true}, nil
}
func (w *runtimeKeyWrapper) Wrap(_ context.Context, keyRef string, profile contracts.CryptoProfile, key []byte) (praxiscrypto.WrappedKey, error) {
	w.key = append([]byte(nil), key...)
	return praxiscrypto.WrappedKey{Ciphertext: append([]byte(nil), key...), SuiteID: "runtime-test", KeyRef: keyRef, KeyVersion: "1", SelectedProfile: profile}, nil
}
func (w *runtimeKeyWrapper) Unwrap(context.Context, praxiscrypto.WrappedKey) ([]byte, error) {
	return append([]byte(nil), w.key...), nil
}

func TestExecuteTurnWithGitRepositoryPersistsAndPublishesOneSelectedUnit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote.git")
	workDir := filepath.Join(root, "work")
	runGitTest(t, root, "init", "--bare", remoteDir)
	runGitTest(t, root, "init", "--initial-branch=main", workDir)
	runGitTest(t, workDir, "config", "user.email", "dogfood@example.invalid")
	runGitTest(t, workDir, "config", "user.name", "Praxis Dogfood")
	runGitTest(t, workDir, "remote", "add", "origin", remoteDir)
	writeFile(t, filepath.Join(workDir, "README"), "base\n")
	runGitTest(t, workDir, "add", "README")
	runGitTest(t, workDir, "commit", "-m", "base")
	runGitTest(t, workDir, "push", "-u", "origin", "main")

	db, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	ledger := Ledger{Store: state.NewSQLiteEventStore(db), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}
	worker := CommandWorker{ProviderID: "local-test-worker", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'bounded\\n' > unit.txt && git add unit.txt && git commit -m bounded >/dev/null && head=$(git rev-parse HEAD) && printf '{\"outcome\":\"COMPLETE\",\"end_head\":\"%s\",\"checkpoint_valid\":true}' \"$head\""}}
	controller := Controller{Ledger: ledger, Worker: worker, NoProgressLimit: 1}
	record, err := controller.ExecuteTurnWithRepository(ctx, TurnRequest{
		GoalID: "dogfood-praxis-issues-96-plus", GoalVersion: "2", InvocationID: "git-integration-1", TurnID: "turn-1",
		GraphID: "praxis.package.goals.default", GraphVersion: "0.2.0", Mode: ModeSupervised, Repository: contracts.RepositorySynced,
		WorkCandidates: []contracts.WorkCandidate{{ID: "bounded-unit", Priority: 1, Sequence: 1, SourceRef: "docs/PLAN/003-post-release-roadmap.md#103", SourceDigest: "sha256:roadmap", Provenance: contracts.ProvenancePLAN}},
	}, GitRepository{Dir: workDir, Remote: "origin", Branch: "main"})
	if err != nil || record.ChildObjective != "bounded-unit" || record.Outcome != OutcomeComplete || !record.Progress || !record.CheckpointPublished {
		db.Close()
		t.Fatalf("production-backed bounded turn mismatch: record=%+v err=%v", record, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resumed := Ledger{Store: state.NewSQLiteEventStore(reopened), Actor: ledger.Actor}
	turns, err := resumed.Load(ctx, record.GoalID, record.GoalVersion)
	if err != nil || len(turns) != 1 || turns[0].ChildObjective != "bounded-unit" || !turns[0].CheckpointPublished {
		t.Fatalf("checkpoint did not survive SQLite restart: turns=%+v err=%v", turns, err)
	}
	snapshot, err := (GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}).Snapshot(ctx)
	if err != nil || snapshot.Relation != contracts.RelationEqual || snapshot.Head != record.EndHead || !snapshot.Clean {
		t.Fatalf("published repository checkpoint mismatch: snapshot=%+v record=%+v err=%v", snapshot, record, err)
	}
}

func TestRuntimeRecoversExactBaselineAndExecutesOneBoundedUnit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote.git")
	workDir := filepath.Join(root, "work")
	runGitTest(t, root, "init", "--bare", remoteDir)
	runGitTest(t, root, "init", "--initial-branch=main", workDir)
	runGitTest(t, workDir, "config", "user.email", "dogfood@example.invalid")
	runGitTest(t, workDir, "config", "user.name", "Praxis Dogfood")
	runGitTest(t, workDir, "remote", "add", "origin", remoteDir)
	writeFile(t, filepath.Join(workDir, "README"), "base\n")
	runGitTest(t, workDir, "add", "README")
	runGitTest(t, workDir, "commit", "-m", "base")
	runGitTest(t, workDir, "push", "-u", "origin", "main")

	db, err := state.OpenSQLite(ctx, filepath.Join(root, "praxis.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	keyWrapper := &runtimeKeyWrapper{}
	repo := goalstore.Repository{Store: state.New(db), Crypto: praxiscrypto.EnvelopeService{Wrapper: keyWrapper}, KeyRef: "key:goals", Profile: contracts.CryptoPQRequired, Sensitivity: state.SensitivityConfidential}
	baseline := goals.GoalBaseline{ID: "goal-runtime", Version: "1", OriginalIntent: "run one bounded unit", RefinedOutcome: "persist one checkpoint", Rigor: goals.RigorStructured, RecommendationMode: goals.RecommendationReviewAll, WorkPlan: &contracts.WorkPlan{BaselineDigest: "source-baseline", AuthorityRef: "authority:test", AuthorityDigest: "sha256:authority", AcceptanceRef: "acceptance:test", AcceptanceDigest: "sha256:acceptance", AcceptedBy: contracts.PrincipalRef{ID: "operator", Kind: "human"}, ProposalDigest: "sha256:proposal", Candidates: []contracts.WorkCandidate{{ID: "runtime-unit", Priority: 1, Sequence: 1, SourceRef: "test:accepted-workplan", SourceDigest: "sha256:source", Provenance: contracts.ProvenancePLAN, Requirements: []contracts.RequirementRef{{ID: "requirement-1", SourceRef: "goal:runtime", SourceDigest: "sha256:requirement"}}}}}}
	if _, err := repo.Save(ctx, baseline, time.Now().UTC(), nil); err != nil {
		t.Fatal(err)
	}
	dbLedger := state.NewSQLiteEventStore(db)
	providers := NewRegistry()
	if err := providers.Register("local-command", CommandWorker{ProviderID: "local-command", Dir: workDir, Command: []string{"/bin/sh", "-c", "printf 'runtime\\n' > runtime.txt && git add runtime.txt && git commit -m runtime >/dev/null && head=$(git rev-parse HEAD) && printf '{\"outcome\":\"COMPLETE\",\"end_head\":\"%s\",\"checkpoint_valid\":true}' \"$head\""}}); err != nil {
		t.Fatal(err)
	}
	runtime := Runtime{Controller: Controller{Ledger: Ledger{Store: dbLedger, Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}}, Providers: providers, NoProgressLimit: 1}, Baselines: repo, Repository: GitRepository{Dir: workDir, Remote: "origin", Branch: "main"}, GraphID: "praxis.package.goals.default", GraphVersion: "0.2.0"}
	record, err := runtime.Execute(ctx, InvocationRequest{Input: contracts.GoalInput{Kind: contracts.GoalInputID, GoalID: baseline.ID}, GoalVersion: baseline.Version, Mode: ModeSupervised, InvocationID: "runtime-invocation-1", ProviderID: "local-command"})
	if err != nil || record.ChildObjective != "runtime-unit" || record.Outcome != OutcomeComplete || !record.Progress || !record.CheckpointPublished {
		t.Fatalf("runtime did not execute one durable bounded unit: record=%+v err=%v", record, err)
	}
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
