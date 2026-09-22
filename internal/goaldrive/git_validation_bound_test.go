//go:build darwin || linux || freebsd || netbsd || openbsd

package goaldrive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The validator prints its working directory and requires `flag` to be "ok" in
// that directory, so the result reflects exactly the bytes it ran against.
const exportFlagValidator = `#!/bin/sh
set -eu
if [ "${1:-integrated}" = integrated ]; then
  pwd -P
  [ "$(cat flag)" = ok ] || { echo "flag is not ok" >&2; exit 1; }
  if [ -e extra ]; then echo "unexpected extra file" >&2; exit 1; fi
  exit 0
fi
printf 'PRAXIS-VALIDATION %s\n' "$1"
`

type exportFixture struct {
	t       *testing.T
	work    string
	remote  string
	repo    GitRepository
	profile string
	marker  string
}

func newExportFixture(t *testing.T, validator string) *exportFixture {
	t.Helper()
	root := t.TempDir()
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	runGitTest(t, root, "init", "--bare", remote)
	runGitTest(t, root, "init", "--initial-branch=main", work)
	runGitTest(t, work, "config", "user.email", "dogfood@example.invalid")
	runGitTest(t, work, "config", "user.name", "Praxis Dogfood")
	runGitTest(t, work, "remote", "add", "origin", remote)
	writeFile(t, filepath.Join(work, "flag"), "ok\n")
	writeFile(t, filepath.Join(work, "README"), "base\n")
	if err := os.MkdirAll(filepath.Join(work, ".praxis"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".praxis", "validate"), []byte(validator), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, work, "add", ".")
	runGitTest(t, work, "commit", "-q", "-m", "base")
	runGitTest(t, work, "push", "-q", "origin", "main")
	f := &exportFixture{t: t, work: work, remote: remote, repo: GitRepository{Dir: work, Remote: "origin", Branch: "main"}, profile: digestBytes([]byte(validator)), marker: filepath.Join(root, "marker")}
	return f
}

func (f *exportFixture) head() string {
	return strings.TrimSpace(runGitOutput(f.t, f.work, "rev-parse", "HEAD"))
}

func (f *exportFixture) remoteHead() string {
	fields := strings.Fields(runGitOutput(f.t, f.work, "ls-remote", "origin", "refs/heads/main"))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// shell runs a worker command in the worker checkout.
func (f *exportFixture) shell(command string) {
	f.t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = f.work
	if output, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("worker command %q: %v\n%s", command, err, output)
	}
}

func (f *exportFixture) validate() (string, error) {
	return f.repo.RunBoundValidation(context.Background(), f.head(), f.profile, "integrated")
}

// TestGitBoundValidationRunsAnExactExportNotTheWorkerCheckout is the
// affirmative composed path: validate, guard, publish the exact commit, and
// prove the validator ran in a private export, not in the worker checkout.
func TestGitBoundValidationRunsAnExactExportNotTheWorkerCheckout(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	f.shell(`printf 'more\n' > more.txt && git add more.txt && git commit -q -m more`)
	head := f.head()
	output, err := f.validate()
	if err != nil {
		t.Fatalf("a clean legitimate checkpoint was refused: %v\n%s", err, output)
	}
	ran := strings.TrimSpace(strings.Split(output, "\n")[0])
	realWork, _ := filepath.EvalSymlinks(f.work)
	if ran == realWork || strings.HasPrefix(ran, realWork+string(filepath.Separator)) || !strings.Contains(ran, "praxis-bound-validation-") {
		t.Fatalf("the validator ran in %q, not in a private export (worker checkout %q)", ran, realWork)
	}
	if _, statErr := os.Stat(filepath.Dir(ran)); !os.IsNotExist(statErr) {
		t.Fatalf("the export was not removed after validation: %v", statErr)
	}
	if err := f.repo.PublishGuard(context.Background(), head, f.profile); err != nil {
		t.Fatalf("the guard refused the exact qualified checkpoint: %v", err)
	}
	if err := f.repo.PushAndVerify(context.Background(), head); err != nil {
		t.Fatalf("publication of the exact commit failed: %v", err)
	}
	if f.remoteHead() != head {
		t.Fatalf("remote is %s, want the qualified %s", f.remoteHead(), head)
	}
}

// N5: validated bytes are the COMMIT's bytes whatever the worker did to its
// working tree, index, hints, excludes or config.
func TestGitBoundValidationBindsToCommitContentNotWorkingTreeState(t *testing.T) {
	// Each manipulation is applied after committing `flag`. `bad` and `ok` are
	// the committed value; the working tree is made to say the opposite.
	manipulations := map[string]func(committed, opposite string) string{
		"assume-unchanged": func(c, o string) string {
			return `printf '` + c + `\n' > flag && git commit -q --allow-empty -am c && printf '` + o + `\n' > flag && git update-index --assume-unchanged flag`
		},
		"skip-worktree": func(c, o string) string {
			return `printf '` + c + `\n' > flag && git commit -q --allow-empty -am c && printf '` + o + `\n' > flag && git update-index --skip-worktree flag`
		},
		"unstaged drift": func(c, o string) string {
			return `printf '` + c + `\n' > flag && git commit -q --allow-empty -am c && printf '` + o + `\n' > flag`
		},
		"staged drift": func(c, o string) string {
			return `printf '` + c + `\n' > flag && git commit -q --allow-empty -am c && printf '` + o + `\n' > flag && git add flag`
		},
		"index and worktree disagree": func(c, o string) string {
			return `printf '` + c + `\n' > flag && git commit -q --allow-empty -am c && printf '` + o + `\n' > flag && git add flag && printf 'third\n' > flag`
		},
		"assume-unchanged plus staged": func(c, o string) string {
			return `printf '` + c + `\n' > flag && git commit -q --allow-empty -am c && printf '` + o + `\n' > flag && git add flag && git update-index --assume-unchanged flag && printf 'third\n' > flag`
		},
	}
	for name, manipulate := range manipulations {
		t.Run(name+"/committed bad", func(t *testing.T) {
			f := newExportFixture(t, exportFlagValidator)
			f.shell(manipulate("bad", "ok"))
			if output, err := f.validate(); err == nil {
				t.Fatalf("a checkpoint whose committed content fails the validator qualified (worktree said ok):\n%s", output)
			}
		})
		t.Run(name+"/committed ok", func(t *testing.T) {
			f := newExportFixture(t, exportFlagValidator)
			f.shell(manipulate("ok", "bad"))
			if output, err := f.validate(); err != nil {
				t.Fatalf("validation depended on working-tree state instead of the committed checkpoint: %v\n%s", err, output)
			}
		})
	}
	t.Run(".git/info/exclude hides an untracked file", func(t *testing.T) {
		f := newExportFixture(t, exportFlagValidator)
		f.shell(`printf 'x\n' > extra && printf 'extra\n' >> .git/info/exclude && printf 'z\n' > z.txt && git add z.txt && git commit -q -m z`)
		// The worker checkout carries an untracked, excluded file `extra` that
		// the validator rejects if it can see it; the export must not have it.
		if output, err := f.validate(); err != nil {
			t.Fatalf("the export was influenced by the worker's untracked/excluded files: %v\n%s", err, output)
		}
	})
}

// The worker checkout's config, fsmonitor and hooks are never executed by
// validation or publication.
func TestGitBoundValidationNeverExecutesWorkerCheckoutHooksOrFsmonitor(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	for _, hook := range []string{"post-checkout", "pre-push", "reference-transaction", "post-commit", "post-merge"} {
		script := "#!/bin/sh\necho " + hook + " >> " + f.marker + "\nexit 0\n"
		if err := os.WriteFile(filepath.Join(f.work, ".git", "hooks", hook), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	monitor := filepath.Join(filepath.Dir(f.marker), "fsmonitor")
	if err := os.WriteFile(monitor, []byte("#!/bin/sh\necho fsmonitor >> "+f.marker+"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, f.work, "-c", "core.hooksPath=/dev/null", "config", "core.fsmonitor", monitor)
	f.shell(`printf 'more\n' > more.txt && git -c core.hooksPath=/dev/null -c core.fsmonitor=false add more.txt && git -c core.hooksPath=/dev/null -c core.fsmonitor=false commit -q -m more`)
	// Anything recorded from here on was executed by the controller's own git.
	_ = os.Remove(f.marker)
	head := f.head()
	if output, err := f.validate(); err != nil {
		t.Fatalf("validation failed: %v\n%s", err, output)
	}
	if _, err := f.repo.Snapshot(context.Background()); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := f.repo.PublishGuard(context.Background(), head, f.profile); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.PushAndVerify(context.Background(), head); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(f.marker); err == nil {
		t.Fatalf("worker-controlled code was executed with the controller's authority:\n%s", body)
	}
}

// Checkpoint and tree substitution after qualification, and non-exact ids.
func TestGitBoundValidationRefusesCheckpointAndTreeSubstitution(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	f.shell(`printf 'a\n' > a.txt && git add a.txt && git commit -q -m a`)
	qualified := f.head()
	if output, err := f.validate(); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	before := f.remoteHead()

	// Tree substitution: the worker amends the qualified commit after validation.
	f.shell(`printf 'evil\n' > a.txt && git add a.txt && git commit -q --amend -m a`)
	if f.head() == qualified {
		t.Fatal("test setup: amend did not change HEAD")
	}
	if err := f.repo.PublishGuard(context.Background(), qualified, f.profile); err == nil {
		t.Fatal("the guard accepted a checkpoint that is no longer the checkout's HEAD")
	}
	if err := f.repo.PushAndVerify(context.Background(), qualified); err == nil {
		t.Fatal("publication accepted a checkpoint that is not the checkout's HEAD")
	}
	if f.remoteHead() != before {
		t.Fatal("a substituted commit was published")
	}
	// Checkpoint substitution: pushing HEAD's successor under the qualified id
	// is impossible because the exact id is what is pushed.
	if err := f.repo.PushAndVerify(context.Background(), f.head()); err != nil {
		t.Fatalf("the (separately qualified) current HEAD could not be published: %v", err)
	}
	if f.remoteHead() != f.head() {
		t.Fatal("remote does not equal the exact commit that was pushed")
	}
	// Non-exact identities are refused outright.
	for _, id := range []string{"", "HEAD", "main", qualified[:12], strings.Repeat("0", 40), strings.ToUpper(qualified)} {
		if _, err := f.repo.RunBoundValidation(context.Background(), id, f.profile, "integrated"); err == nil {
			t.Fatalf("checkpoint %q was accepted", id)
		}
	}
	// A recorded tree that no longer matches is refused.
	head := f.head()
	qualifiedTrees.Store(f.repo.Dir+"\x00"+head, strings.Repeat("a", 40))
	t.Cleanup(func() { qualifiedTrees.Delete(f.repo.Dir + "\x00" + head) })
	if err := f.repo.VerifyValidationBinding(context.Background(), head, f.profile); err == nil {
		t.Fatal("a checkpoint whose tree differs from the qualified tree was accepted")
	}
}

// The validator identity is the one COMMITTED at the checkpoint.
func TestGitBoundValidationRefusesValidatorThatDiffersFromTheProfile(t *testing.T) {
	t.Run("swapped in the worktree only", func(t *testing.T) {
		f := newExportFixture(t, exportFlagValidator)
		// The committed validator fails; the worktree copy always passes and is
		// hidden from status. The profile digest names the committed validator.
		f.shell(`printf 'bad\n' > flag && git commit -q -am bad && printf '#!/bin/sh\nexit 0\n' > .praxis/validate && chmod +x .praxis/validate && git update-index --assume-unchanged .praxis/validate`)
		if output, err := f.validate(); err == nil {
			t.Fatalf("the worktree-only validator qualified the checkpoint:\n%s", output)
		}
	})
	t.Run("committed validator differs from the profile", func(t *testing.T) {
		f := newExportFixture(t, exportFlagValidator)
		f.shell(`printf '#!/bin/sh\nexit 0\n' > .praxis/validate && git commit -q -am swap`)
		if output, err := f.validate(); err == nil {
			t.Fatalf("a validator that is not the bound profile qualified:\n%s", output)
		}
	})
	t.Run("committed validator lost its executable bit", func(t *testing.T) {
		f := newExportFixture(t, exportFlagValidator)
		f.shell(`chmod -x .praxis/validate && git update-index --chmod=-x .praxis/validate && git commit -q -m chmod`)
		if output, err := f.validate(); err == nil {
			t.Fatalf("a non-executable committed validator qualified:\n%s", output)
		}
	})
}

// A validator that changes its own export - including by hiding the change
// with index hints inside the export - cannot qualify the checkpoint, and the
// export is removed even when the validator made it unwritable.
func TestGitBoundValidationDetectsExportTamperingAndAlwaysCleansUp(t *testing.T) {
	cases := map[string]string{
		"modify tracked file":                 `printf tampered > flag`,
		"modify and assume-unchanged":         `printf tampered > flag; git update-index --assume-unchanged flag`,
		"modify and skip-worktree":            `printf tampered > flag; git update-index --skip-worktree flag`,
		"modify, hide, and exclude untracked": `printf tampered > flag; git update-index --assume-unchanged flag; printf junk > extra2; printf 'extra2\n' >> .git/info/exclude`,
		"new untracked file":                  `printf junk > extra2`,
		"delete tracked file":                 `rm README`,
		"delete and skip-worktree":            `rm README; git update-index --skip-worktree README`,
		"unwritable directory left behind":    `mkdir sub && printf x > sub/f && chmod 000 sub && printf tampered > flag`,
		"stat-cache spoofed in-place rewrite": `t=$(mktemp); touch -r flag "$t"; git config core.trustctime false; git config core.checkStat minimal; printf 'no\n' > flag; touch -r "$t" flag; rm -f "$t"`,
		"excluded untracked file":             `printf junk > extra2; printf 'extra2\n' >> .git/info/exclude`,
	}
	for name, action := range cases {
		t.Run(name, func(t *testing.T) {
			validator := "#!/bin/sh\nset -u\nif [ \"${1:-integrated}\" = integrated ]; then\n  pwd -P\n  " + action + "\n  exit 0\nfi\nprintf 'PRAXIS-VALIDATION %s\\n' \"$1\"\n"
			f := newExportFixture(t, validator)
			output, err := f.validate()
			if err == nil {
				t.Fatalf("a validator that modified its export qualified the checkpoint:\n%s", output)
			}
			ran := strings.TrimSpace(strings.Split(output, "\n")[0])
			if ran == "" {
				t.Fatalf("no export path was reported: %q", output)
			}
			if _, statErr := os.Stat(filepath.Dir(ran)); !os.IsNotExist(statErr) {
				t.Fatalf("the export was not removed: %v", statErr)
			}
		})
	}
}

// The export is removed when the validator times out.
func TestGitBoundValidationCleansUpAfterTimeout(t *testing.T) {
	previous := validationTimeout
	validationTimeout = 500 * time.Millisecond
	t.Cleanup(func() { validationTimeout = previous })
	validator := "#!/bin/sh\n# timeout-cleanup-marker\nif [ \"${1:-integrated}\" = integrated ]; then sleep 30; fi\nexit 0\n"
	f := newExportFixture(t, validator)
	if _, err := f.validate(); err == nil {
		t.Fatal("a validator that timed out qualified")
	}
	leftovers, _ := filepath.Glob(filepath.Join(os.TempDir(), "praxis-bound-validation-*", "checkout"))
	for _, path := range leftovers {
		if body, err := os.ReadFile(filepath.Join(path, ".praxis", "validate")); err == nil && string(body) == validator {
			t.Fatalf("export %s was left behind after a timeout", path)
		}
	}
}

// With a detached HEAD there is no branch ref to compare; the exact-HEAD check
// alone must refuse a checkpoint that is no longer what the checkout is at.
func TestGitBoundValidationRefusesSubstitutionOnADetachedHead(t *testing.T) {
	f := newExportFixture(t, exportFlagValidator)
	f.repo.AllowDetached = true
	runGitTest(t, f.work, "checkout", "-q", "--detach")
	f.shell(`printf 'a\n' > a.txt && git add a.txt && git commit -q -m a`)
	qualified := f.head()
	if output, err := f.validate(); err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	f.shell(`printf 'b\n' > b.txt && git add b.txt && git commit -q -m b`)
	if err := f.repo.PublishGuard(context.Background(), qualified, f.profile); err == nil {
		t.Fatal("the guard accepted a qualified checkpoint that is not the detached HEAD")
	}
	if err := f.repo.PushAndVerify(context.Background(), qualified); err == nil {
		t.Fatal("publication accepted a checkpoint that is not HEAD")
	}
}
