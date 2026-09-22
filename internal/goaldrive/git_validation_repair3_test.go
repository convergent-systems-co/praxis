package goaldrive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// N5 (content layer on its own). A validator running in its export can rewrite
// a tracked file in place, keep the size, restore the mtime, and instruct the
// export's own git to ignore ctime and to trust stat only, after first letting
// the index become older than the file (so git's racy-timestamp rehash no longer
// applies). `git status`, the hint-flag scan and the untracked scan are then all
// blind to the change: only re-hashing the export's CONTENT into a fresh index
// proves it is no longer the checkpoint. This test exists so that removing that
// layer is caught by a regression instead of hiding behind the cheaper layers.
func TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite(t *testing.T) {
	action := `sleep 2; git status >/dev/null 2>&1; git config core.trustctime false; git config core.checkStat minimal; ` +
		`t=$(mktemp); touch -r flag "$t"; printf 'no\n' > flag; touch -r "$t" flag; rm -f "$t"; ` +
		`if [ -n "$(git status --porcelain --untracked-files=all)" ]; then echo BLIND=false; else echo BLIND=true; fi`
	validator := "#!/bin/sh\nset -u\nif [ \"${1:-integrated}\" = integrated ]; then\n  pwd -P\n  " + action + "\n  exit 0\nfi\nprintf 'PRAXIS-VALIDATION %s\\n' \"$1\"\n"
	f := newExportFixture(t, validator)
	output, err := f.validate()
	if !strings.Contains(output, "BLIND=true") {
		t.Skipf("this git does not let the validator blind status here (BLIND not true); output: %q", output)
	}
	if err == nil {
		t.Fatalf("a validator that rewrote its export in a status-blind way qualified the checkpoint:\n%s", output)
	}
}

// N5 (hint layer on its own). A validator that changes NO content but plants an
// index hint (assume-unchanged / skip-worktree) in its export has prepared the
// export to hide a later change from `git status`. Content re-hashing alone
// would not notice a hint with unchanged content; the hint scan must refuse it.
func TestGitBoundValidationRefusesAnIndexHintEvenWithoutAContentChange(t *testing.T) {
	for name, action := range map[string]string{
		"assume-unchanged": `git update-index --assume-unchanged README`,
		"skip-worktree":    `git update-index --skip-worktree README`,
	} {
		t.Run(name, func(t *testing.T) {
			validator := "#!/bin/sh\nset -u\nif [ \"${1:-integrated}\" = integrated ]; then\n  " + action + "\n  exit 0\nfi\nprintf 'PRAXIS-VALIDATION %s\\n' \"$1\"\n"
			f := newExportFixture(t, validator)
			if output, err := f.validate(); err == nil {
				t.Fatalf("a validator that planted an index hint in its export qualified the checkpoint:\n%s", output)
			}
		})
	}
}

// A validator runs code in its export, whose `origin` is the worker checkout it
// was cloned from. It can therefore reach back and move the worker checkout's
// HEAD while it runs. The post-run re-binding of the checkpoint must notice.
func TestGitBoundValidationRefusesAValidatorThatMovesTheWorkerCheckoutDuringTheRun(t *testing.T) {
	action := `git -C "$(git config remote.origin.url)" commit --allow-empty -q -m moved-by-validator`
	validator := "#!/bin/sh\nset -u\nif [ \"${1:-integrated}\" = integrated ]; then\n  " + action + "\n  exit 0\nfi\nprintf 'PRAXIS-VALIDATION %s\\n' \"$1\"\n"
	f := newExportFixture(t, validator)
	before := f.head()
	if output, err := f.validate(); err == nil || !strings.Contains(err.Error(), "validation binding after execution") {
		t.Fatalf("a validator that moved the worker checkout during the run qualified: err=%v\n%s", err, output)
	}
	if f.head() == before {
		t.Skip("the validator could not reach the worker checkout; nothing to prove")
	}
}

// A validator that only commits in its export changes no tree (an empty commit)
// yet is no longer at the qualified checkpoint: the HEAD identity of the export
// is its own check.
func TestGitBoundValidationRefusesAnExportWhoseHeadMoved(t *testing.T) {
	validator := "#!/bin/sh\nset -u\nif [ \"${1:-integrated}\" = integrated ]; then\n  git -c user.email=a@example.invalid -c user.name=a commit --allow-empty -q -m moved\n  exit 0\nfi\nprintf 'PRAXIS-VALIDATION %s\\n' \"$1\"\n"
	f := newExportFixture(t, validator)
	if output, err := f.validate(); err == nil || !strings.Contains(err.Error(), "not at the checkpoint") {
		t.Fatalf("an export whose HEAD moved qualified the checkpoint: err=%v\n%s", err, output)
	}
}

// The checkpoint identity must be a full object id: a ref name or an abbreviation
// can be re-pointed or become ambiguous, an object id cannot.
func TestGitBoundValidationRequiresAFullObjectId(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	head := f.head()
	if err := f.repo.VerifyValidationBinding(context.Background(), head, f.profile); err != nil {
		t.Fatalf("control: the full object id was refused: %v", err)
	}
	for _, id := range []string{"HEAD", "main", head[:12], strings.ToUpper(head)} {
		if err := f.repo.VerifyValidationBinding(context.Background(), id, f.profile); err == nil || !strings.Contains(err.Error(), "not a full object id") {
			t.Fatalf("%q was accepted as a checkpoint identity: %v", id, err)
		}
	}
}

// Publication verifies the remote ref after the push: a remote that accepts the
// push and then ends up pointing elsewhere (here a post-receive hook that resets
// the branch) is not a published checkpoint.
func TestGitBoundValidationPublicationVerifiesTheRemoteRef(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	f.shell(`printf 'a\n' > a.txt && git add a.txt && git commit -q -m a`)
	hook := filepath.Join(f.remote, "hooks", "post-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ngit update-ref refs/heads/main \"$(git rev-parse refs/heads/main~1)\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.PushAndVerify(context.Background(), f.head()); err == nil || !strings.Contains(err.Error(), "remote checkpoint is") {
		t.Fatalf("a push that did not leave the remote at the checkpoint was reported as published: %v", err)
	}
}

// Replace refs in the worker checkout can make an object id name different
// content. The controller's git ignores them, so the qualified identity holds.
func TestGitBoundValidationIgnoresReplaceRefsInTheWorkerCheckout(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	// A side commit whose committed validator is NOT the bound one.
	f.shell(`git checkout -q -b side && printf '# tampered\n' >> .praxis/validate && git commit -q -am tampered && git checkout -q main`)
	tampered := strings.TrimSpace(runGitOutput(t, f.work, "rev-parse", "side"))
	f.shell(`printf 'a\n' > a.txt && git add a.txt && git commit -q -m a`)
	checkpoint := f.head()
	if err := f.repo.VerifyValidationBinding(context.Background(), checkpoint, f.profile); err != nil {
		t.Fatalf("control: the genuine checkpoint did not bind: %v", err)
	}
	// With the replace ref honoured, reading the checkpoint would yield the
	// tampered commit's validator; the controller's git must not honour it.
	f.shell("git replace -f " + checkpoint + " " + tampered)
	if err := f.repo.VerifyValidationBinding(context.Background(), checkpoint, f.profile); err != nil {
		t.Fatalf("a replace ref in the worker checkout changed what the checkpoint id names: %v", err)
	}
	if output, err := f.repo.RunBoundValidation(context.Background(), checkpoint, f.profile, "integrated"); err != nil {
		t.Fatalf("a replace ref in the worker checkout changed what was validated: %v\n%s", err, output)
	}
}
