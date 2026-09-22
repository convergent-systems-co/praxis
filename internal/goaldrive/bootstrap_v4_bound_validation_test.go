package goaldrive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/convergent-systems-co/praxis/internal/eventstore"
	"github.com/convergent-systems-co/praxis/packages/goals"
	"github.com/convergent-systems-co/praxis/pkg/contracts"
)

const boundValidatorScript = `#!/bin/sh
set -eu
if [ "${1:-integrated}" = integrated ]; then exit 0; fi
printf 'PRAXIS-VALIDATION %s\n' "$1"
`

// boundGitFixture is a real repository (bare remote plus checkout) carrying a
// digest-bound validator, a safety-bearing baseline whose plan binds that
// validator's digest, and a controller wired with every safety seam.
type boundGitFixture struct {
	t          *testing.T
	work       string
	remote     string
	ledger     Ledger
	baseline   *goals.GoalBaseline
	candidate  contracts.WorkCandidate
	repository GitRepository
}

func newBoundGitFixture(t *testing.T) *boundGitFixture {
	t.Helper()
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	runGitTest(t, root, "init", "--bare", remote)
	runGitTest(t, root, "init", "--initial-branch=main", work)
	runGitTest(t, work, "config", "user.email", "dogfood@example.invalid")
	runGitTest(t, work, "config", "user.name", "Praxis Dogfood")
	runGitTest(t, work, "remote", "add", "origin", remote)
	writeFile(t, filepath.Join(work, "README"), "base\n")
	runGitTest(t, work, "add", "README")
	runGitTest(t, work, "commit", "-q", "-m", "base")
	declareValidator(t, work, boundValidatorScript)
	sum := sha256.Sum256([]byte(boundValidatorScript))
	binding := protectedBinding()
	binding.ValidationProfileDigest = "sha256:" + hex.EncodeToString(sum[:])
	candidate := contracts.WorkCandidate{ID: "unit", Kind: contracts.WorkCandidateOrdinary, Priority: 1, Sequence: 1, SourceRef: "unit.json", SourceDigest: "sha256:" + strings.Repeat("4", 64), Provenance: contracts.ProvenancePLAN, QualificationPredicates: []string{"candidate/unit/conformance"}}
	return &boundGitFixture{
		t: t, work: work, remote: remote,
		ledger:     Ledger{Store: eventstore.NewMemoryStore(), Actor: contracts.PrincipalRef{ID: "controller", Kind: "controller"}},
		baseline:   &goals.GoalBaseline{ID: "goal", Version: "1", Digest: "sha256:" + strings.Repeat("9", 64), WorkPlan: &contracts.WorkPlan{Safety: binding, Candidates: []contracts.WorkCandidate{candidate}}},
		candidate:  candidate,
		repository: GitRepository{Dir: work, Remote: "origin", Branch: "main"},
	}
}

func (f *boundGitFixture) remoteHead() string {
	return strings.TrimSpace(runGitOutput(f.t, f.work, "ls-remote", "origin", "refs/heads/main"))
}

// turn runs one controller turn whose worker executes the shell command.
func (f *boundGitFixture) turn(id, command string, authority PlanAuthorityVerifier) (TurnRecord, error) {
	f.t.Helper()
	worker := ProviderCLIWorker{ProviderID: "local-subscription-test", Dir: f.work, Command: []string{"/bin/sh", "-c", command}}
	controller := Controller{Ledger: f.ledger, Worker: worker, NoProgressLimit: 1, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: authority}
	return controller.ExecuteTurnWithRepository(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: id, TurnID: id + ":turn:1", GraphID: "g", GraphVersion: "1", Mode: ModeSupervised, GoalBaseline: f.baseline}, f.repository)
}

const commitWithClaim = `printf 'work\n' > unit.txt && git add unit.txt && git commit -q -m 'unit' -m 'Praxis-Unit-Complete: unit'`

// TestBoundValidationQualifiesPublishesAndSurvivesRestart is the affirmative
// composed path: select, worker, integrated validation bound to the exact
// checkpoint and profile, candidate conformance, publication, qualified
// completion, then a fresh controller (restart) that sees the completion.
func TestBoundValidationQualifiesPublishesAndSurvivesRestart(t *testing.T) {
	f := newBoundGitFixture(t)
	before := f.remoteHead()
	record, err := f.turn("inv-a", commitWithClaim, passingPlanAuthority{})
	if err != nil || !record.Progress || !record.CheckpointPublished || !record.UnitCompleted {
		t.Fatalf("bound turn did not qualify and publish: %+v %v", record, err)
	}
	if f.remoteHead() == before {
		t.Fatal("qualified checkpoint was not published")
	}
	completions, err := f.ledger.LoadCompletions(context.Background(), "goal", "1")
	if err != nil || len(completions) != 1 || !completions[0].MechanismTestsPassed || !completions[0].ConformanceQualified || completions[0].ValidationProfileDigest != f.baseline.WorkPlan.Safety.ValidationProfileDigest {
		t.Fatalf("completion lacks integrated/conformance evidence: %+v %v", completions, err)
	}
	if err := VerifyPlanCompletions(f.baseline.WorkPlan, completions); err != nil {
		t.Fatal(err)
	}
	// Restart: a fresh controller over the same durable ledger derives its
	// state from the ledger alone and sees no remaining runnable unit.
	restarted := Controller{Ledger: f.ledger, Worker: &countingWorker{}, SafetyActivation: passingSafetyActivation{}, GoverningAuthority: passingPlanAuthority{}}
	_, _, err = restarted.prepare(context.Background(), TurnRequest{GoalID: "goal", GoalVersion: "1", InvocationID: "inv-b", TurnID: "inv-b:turn:1", Mode: ModeSupervised, GoalBaseline: f.baseline})
	if err == nil {
		t.Fatal("restart selected work although every unit is durably complete")
	}
}

// Path E: validator or profile drift after admission must fail closed before
// publication and completion.
func TestValidationDriftAfterAdmissionFailsClosedBeforePublication(t *testing.T) {
	cases := map[string]string{
		"validator deleted":     `git rm -q .praxis/validate && git commit -q -m 'drop validator' -m 'Praxis-Unit-Complete: unit'`,
		"executable bit lost":   `chmod -x .praxis/validate && git add .praxis/validate && git commit -q -m 'chmod' -m 'Praxis-Unit-Complete: unit'`,
		"profile bytes changed": `printf '# drift\n' >> .praxis/validate && git add .praxis/validate && git commit -q -m 'drift' -m 'Praxis-Unit-Complete: unit'`,
	}
	for name, command := range cases {
		t.Run(name, func(t *testing.T) {
			f := newBoundGitFixture(t)
			before := f.remoteHead()
			record, err := f.turn("inv-e", command, passingPlanAuthority{})
			if err == nil || record.CheckpointPublished || record.UnitCompleted {
				t.Fatalf("drift was accepted: %+v err=%v", record, err)
			}
			if f.remoteHead() != before {
				t.Fatal("a checkpoint whose validation drifted was published")
			}
			if completions, _ := f.ledger.LoadCompletions(context.Background(), "goal", "1"); len(completions) != 0 {
				t.Fatalf("a completion was recorded after validation drift: %+v", completions)
			}
		})
	}
}

// A validator that dirties the tree while it runs cannot qualify: the result
// would be attributed to state that no longer equals the checkpoint.
func TestValidationThatMutatesTheCheckoutDuringExecutionDoesNotQualify(t *testing.T) {
	f := newBoundGitFixture(t)
	mutating := strings.Replace(boundValidatorScript, "if [ \"${1:-integrated}\" = integrated ]; then exit 0; fi", "if [ \"${1:-integrated}\" = integrated ]; then printf x > mutated.txt; exit 0; fi", 1)
	sum := sha256.Sum256([]byte(mutating))
	f.baseline.WorkPlan.Safety.ValidationProfileDigest = "sha256:" + hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(f.work, ".praxis", "validate"), []byte(mutating), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, f.work, "commit", "-q", "-am", "mutating validator")
	runGitTest(t, f.work, "push", "-q", "origin", "main")
	before := f.remoteHead()
	// No completion claim: the integrated run alone must be bound, because no
	// later conformance run exists to notice the mutation.
	record, err := f.turn("inv-m", `printf 'w\n' > w.txt && git add w.txt && git commit -q -m w`, passingPlanAuthority{})
	if err == nil || record.CheckpointPublished || f.remoteHead() != before {
		t.Fatalf("mutation during validation was accepted: %+v %v", record, err)
	}
}

// A candidate whose conformance predicate fails must not be published: the
// integrated pass alone does not qualify a unit.
func TestFailedCandidateConformanceIsDecidedBeforePublication(t *testing.T) {
	f := newBoundGitFixture(t)
	f.baseline.WorkPlan.Candidates[0].QualificationPredicates = []string{"candidate/unit/unhandled"}
	failing := strings.Replace(boundValidatorScript, `printf 'PRAXIS-VALIDATION %s\n' "$1"`, `echo unhandled >&2; exit 64`, 1)
	sum := sha256.Sum256([]byte(failing))
	f.baseline.WorkPlan.Safety.ValidationProfileDigest = "sha256:" + hex.EncodeToString(sum[:])
	if err := os.WriteFile(filepath.Join(f.work, ".praxis", "validate"), []byte(failing), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, f.work, "commit", "-q", "-am", "failing conformance")
	runGitTest(t, f.work, "push", "-q", "origin", "main")
	before := f.remoteHead()
	record, err := f.turn("inv-c", commitWithClaim, passingPlanAuthority{})
	if err == nil || record.CheckpointPublished || f.remoteHead() != before {
		t.Fatalf("a checkpoint failing conformance was published: %+v %v", record, err)
	}
}

// Path C at the controller boundary: revoked governing authority refuses every
// future turn, including a continuous subsequent one, and records nothing.
type revocableAuthority struct {
	passingPlanAuthority
	revoked bool
}

func (a *revocableAuthority) VerifyGoverningAuthority(context.Context, goals.GoalBaseline, time.Time) error {
	if a.revoked {
		return errors.New("acceptance decision is revoked")
	}
	return nil
}

func TestRevokedGoverningAuthorityRefusesTheNextTurn(t *testing.T) {
	f := newBoundGitFixture(t)
	authority := &revocableAuthority{}
	if _, err := f.turn("inv-r1", `printf 'w\n' > w.txt && git add w.txt && git commit -q -m w`, authority); err != nil {
		t.Fatal(err)
	}
	authority.revoked = true
	head := f.remoteHead()
	record, err := f.turn("inv-r2", `printf 'x\n' > x.txt && git add x.txt && git commit -q -m x`, authority)
	if !errors.Is(err, ErrPlanAuthority) || record.Progress || f.remoteHead() != head {
		t.Fatalf("a revoked plan still governed execution: %+v %v", record, err)
	}
}

func TestSafetyPlanWithoutGoverningAuthorityVerifierFailsClosed(t *testing.T) {
	f := newBoundGitFixture(t)
	if _, err := f.turn("inv-n", commitWithClaim, nil); !errors.Is(err, ErrPlanAuthority) {
		t.Fatalf("missing verifier was tolerated: %v", err)
	}
}
