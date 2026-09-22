package bootstrapv4

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// imageLab builds two real probe executables A and B that share VCS identity
// (and, being built from one source with a different -X marker, have the same
// length) but differ in bytes. Each scenario starts a fresh copy of A, mutates
// the executable file behind the running process in some way, then asks the
// running process to verify a manifest.
type imageLab struct {
	t          *testing.T
	dir        string
	a, b       string
	digestA    string
	digestB    string
	revision   string
	modified   bool
	sameLength bool
	counter    int
}

func newImageLab(t *testing.T) *imageLab {
	t.Helper()
	if testing.Short() {
		t.Skip("builds real executables")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	lab := &imageLab{t: t, dir: t.TempDir()}
	lab.a, lab.b = buildProbe(t, lab.dir, "A"), buildProbe(t, lab.dir, "B")
	idBytes, err := exec.Command(lab.a, "identity").Output()
	if err != nil {
		t.Fatal(err)
	}
	var identity struct{ Revision, Modified string }
	if err := json.Unmarshal([]byte(strings.ToLower(string(idBytes))), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.Revision == "" {
		t.Skip("probe build carries no VCS identity (source is not a Git checkout); the identity half of the check cannot be exercised")
	}
	lab.revision = identity.Revision
	lab.modified, _ = strconv.ParseBool(identity.Modified)
	lab.digestA, lab.digestB = fileDigest(t, lab.a), fileDigest(t, lab.b)
	if lab.digestA == lab.digestB {
		t.Fatal("probe builds must differ in bytes")
	}
	infoA, _ := os.Stat(lab.a)
	infoB, _ := os.Stat(lab.b)
	lab.sameLength = infoA.Size() == infoB.Size()
	return lab
}

func (l *imageLab) install(name, source string) string {
	l.t.Helper()
	body, err := os.ReadFile(source)
	if err != nil {
		l.t.Fatal(err)
	}
	target := filepath.Join(l.dir, name)
	if err := os.WriteFile(target, body, 0o755); err != nil {
		l.t.Fatal(err)
	}
	return target
}

func (l *imageLab) manifest(path, digest string) string {
	l.t.Helper()
	l.counter++
	body, _ := json.Marshal(Manifest{ActiveBinaryPath: path, ActiveBinaryDigest: digest, SourceCommit: l.revision, BuildModified: l.modified})
	file := filepath.Join(l.dir, "manifest-"+strconv.Itoa(l.counter)+".json")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		l.t.Fatal(err)
	}
	return file
}

// verdict asks the running process to verify and returns its verdict together
// with the marker of the code that is actually still executing.
func (p *probeProcess) verdict(t *testing.T) (verdict, ran string) {
	t.Helper()
	_, _ = p.stdin.Write([]byte("go\n"))
	verdict = strings.TrimPrefix(p.line(t), "VERIFY ")
	ran = strings.TrimPrefix(p.line(t), "RAN ")
	return verdict, ran
}

func requireRefused(t *testing.T, verdict, ran, wantRunning string) {
	t.Helper()
	if ran != wantRunning {
		t.Fatalf("test precondition: the process should still be executing %s, it reports %s", wantRunning, ran)
	}
	if verdict == "<nil>" {
		t.Fatalf("verification accepted an image other than the one this process is executing (process still runs %s)", ran)
	}
	t.Log(verdict)
}

// TestRunningImageInPlaceOverwriteIsRefused is Astra Review 3 finding N3, kept
// as a permanent regression. Process A is executing; the executable file behind
// it is overwritten IN PLACE (same inode) with B's bytes; A is asked to verify.
// Before the repair a manifest binding B was accepted while A was still the code
// executing, because the descriptor re-read returned B's bytes and the inode
// had not changed. macOS does not refuse writes to a running executable, so the
// overwrite succeeds and the process keeps running A.
func TestRunningImageInPlaceOverwriteIsRefused(t *testing.T) {
	lab := newImageLab(t)
	overwrite := func(t *testing.T, target string, truncate bool) {
		t.Helper()
		flags := os.O_WRONLY
		if truncate {
			flags |= os.O_TRUNC
		}
		file, err := os.OpenFile(target, flags, 0)
		if err != nil {
			if runtime.GOOS != "darwin" {
				t.Skipf("this operating system refuses to open a running executable for writing (%v); the overwrite counterexample cannot be constructed here and the guarantee is the OS's, not this package's", err)
			}
			t.Fatal(err)
		}
		defer file.Close()
		body, err := os.ReadFile(lab.b)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name     string
		truncate bool
	}{{"truncate then write", true}, {"same-length overwrite", false}}
	for _, c := range cases {
		for _, bind := range []string{"B", "A"} {
			t.Run(c.name+"/manifest binds "+bind, func(t *testing.T) {
				if !c.truncate && !lab.sameLength {
					t.Skip("probe builds differ in length")
				}
				target := lab.install("inplace-"+strings.ReplaceAll(c.name, " ", "-")+"-"+bind, lab.a)
				digest := lab.digestB
				if bind == "A" {
					digest = lab.digestA
				}
				p := startProbe(t, target, lab.manifest(target, digest))
				if got := p.line(t); got != "RUNNING A" {
					t.Fatalf("unexpected first line %q", got)
				}
				overwrite(t, target, c.truncate)
				if fileDigest(t, target) != lab.digestB {
					t.Fatal("test precondition: the executable file was not overwritten with B")
				}
				verdict, ran := p.verdict(t)
				requireRefused(t, verdict, ran, "A")
			})
		}
	}
}

// TestRunningImagePathnameMutationMatrix keeps the earlier atomic-replacement
// refusals and adds the other pathname-level mutations, each with a control.
func TestRunningImagePathnameMutationMatrix(t *testing.T) {
	lab := newImageLab(t)
	mutations := map[string]func(t *testing.T, target string){
		"unlink and recreate": func(t *testing.T, target string) {
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, mustRead(t, lab.b), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"atomic rename": func(t *testing.T, target string) {
			replacement := lab.install("rename-source", lab.b)
			if err := os.Rename(replacement, target); err != nil {
				t.Fatal(err)
			}
		},
		"identical bytes on a new inode": func(t *testing.T, target string) {
			replacement := lab.install("same-bytes-source", lab.a)
			if err := os.Rename(replacement, target); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range mutations {
		for _, bind := range []string{"B", "A"} {
			t.Run(name+"/manifest binds "+bind, func(t *testing.T) {
				digest := lab.digestB
				if bind == "A" {
					digest = lab.digestA
				}
				target := lab.install("matrix-"+strings.ReplaceAll(name, " ", "-")+"-"+bind, lab.a)
				p := startProbe(t, target, lab.manifest(target, digest))
				if got := p.line(t); got != "RUNNING A" {
					t.Fatalf("unexpected first line %q", got)
				}
				mutate(t, target)
				verdict, ran := p.verdict(t)
				requireRefused(t, verdict, ran, "A")
			})
		}
	}
	t.Run("symlink retargeted to B", func(t *testing.T) {
		real := lab.install("sym-real", lab.a)
		link := filepath.Join(lab.dir, "sym-link")
		if err := os.Symlink(real, link); err != nil {
			t.Skip("symlinks unavailable: ", err)
		}
		p := startProbe(t, link, lab.manifest(link, lab.digestB))
		if got := p.line(t); got != "RUNNING A" {
			t.Fatalf("unexpected first line %q", got)
		}
		other := lab.install("sym-other", lab.b)
		tmp := link + ".new"
		if err := os.Symlink(other, tmp); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, link); err != nil {
			t.Fatal(err)
		}
		verdict, ran := p.verdict(t)
		requireRefused(t, verdict, ran, "A")
	})

	// Positive controls: the genuine, unmodified image must still verify, however
	// it was launched.
	controls := map[string]func(t *testing.T) (launch, manifestPath string){
		"direct launch": func(t *testing.T) (string, string) {
			target := lab.install("ctl-direct", lab.a)
			return target, target
		},
		"hardlink launch": func(t *testing.T) (string, string) {
			target := lab.install("ctl-hard", lab.a)
			link := filepath.Join(lab.dir, "ctl-hard-link")
			if err := os.Link(target, link); err != nil {
				t.Skip("hard links unavailable: ", err)
			}
			return link, target
		},
		"symlink launch, manifest names the symlink": func(t *testing.T) (string, string) {
			target := lab.install("ctl-sym", lab.a)
			link := filepath.Join(lab.dir, "ctl-sym-link")
			if err := os.Symlink(target, link); err != nil {
				t.Skip("symlinks unavailable: ", err)
			}
			return link, link
		},
		"symlink launch, manifest names the real file": func(t *testing.T) (string, string) {
			target := lab.install("ctl-sym2", lab.a)
			link := filepath.Join(lab.dir, "ctl-sym2-link")
			if err := os.Symlink(target, link); err != nil {
				t.Skip("symlinks unavailable: ", err)
			}
			return link, target
		},
	}
	for name, setup := range controls {
		t.Run("control: "+name, func(t *testing.T) {
			launch, manifestPath := setup(t)
			p := startProbe(t, launch, lab.manifest(manifestPath, lab.digestA))
			if got := p.line(t); got != "RUNNING A" {
				t.Fatalf("unexpected first line %q", got)
			}
			verdict, ran := p.verdict(t)
			if verdict != "<nil>" || ran != "A" {
				t.Fatalf("the genuine running image was refused: %q (running %s)", verdict, ran)
			}
		})
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// TestRunningImageExecToInitWindow covers a replacement that lands after the
// kernel executed the file and before this package captured its descriptor. A
// probe build is given an earlier-initialised package (through a Go build
// overlay, so no repository file is created) that blocks until the test has
// mutated the file. The descriptor this package then opens is the new file.
// On darwin the kernel's exec-time code-directory hash still names A, so
// verification refuses; elsewhere the window is documented and not closed.
func TestRunningImageExecToInitWindow(t *testing.T) {
	lab := newImageLab(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	const module = "github.com/convergent-systems-co/praxis"
	sleeper := filepath.Join(lab.dir, "aaasleep.go")
	if err := os.WriteFile(sleeper, []byte(`package aaasleep

import (
	"bufio"
	"fmt"
	"os"
)

func init() {
	fmt.Println("EARLY")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	importer := filepath.Join(lab.dir, "zz_early.go")
	if err := os.WriteFile(importer, []byte("package main\n\nimport _ \""+module+"/internal/aaasleep\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(lab.dir, "overlay.json")
	overlayBody, _ := json.Marshal(map[string]any{"Replace": map[string]string{
		filepath.Join(root, "internal", "aaasleep", "aaasleep.go"):                              sleeper,
		filepath.Join(root, "internal", "bootstrapv4", "testdata", "imageprobe", "zz_early.go"): importer,
	}})
	if err := os.WriteFile(overlay, overlayBody, 0o600); err != nil {
		t.Fatal(err)
	}
	buildEarly := func(marker string) string {
		out := filepath.Join(lab.dir, "early-"+marker)
		cmd := exec.Command("go", "build", "-overlay="+overlay, "-ldflags", "-X main.marker="+marker, "-o", out, "./testdata/imageprobe")
		if combined, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build early probe %s: %v\n%s", marker, err, combined)
		}
		return out
	}
	ea, eb := buildEarly("A"), buildEarly("B")
	digestA, digestB := fileDigest(t, ea), fileDigest(t, eb)
	if digestA == digestB {
		t.Fatal("early probe builds must differ")
	}
	start := func(t *testing.T, name, digest string) (*probeProcess, string) {
		target := lab.install(name, ea)
		p := startProbe(t, target, lab.manifest(target, digest))
		if got := p.line(t); got != "EARLY" {
			t.Fatalf("early probe did not pause before initialisation: %q", got)
		}
		return p, target
	}
	release := func(t *testing.T, p *probeProcess) (verdict, ran string) {
		_, _ = p.stdin.Write([]byte("\n"))
		if got := p.line(t); got != "RUNNING A" {
			t.Fatalf("unexpected line after release: %q", got)
		}
		return p.verdict(t)
	}
	t.Run("control: no mutation verifies", func(t *testing.T) {
		p, _ := start(t, "early-control", digestA)
		verdict, ran := release(t, p)
		if verdict != "<nil>" || ran != "A" {
			t.Fatalf("unmodified image refused: %q running %s", verdict, ran)
		}
	})
	if runtime.GOOS != "darwin" {
		t.Run("residual", func(t *testing.T) {
			t.Skip("a replacement between exec and package initialisation is not observable on this platform; documented in VerifyProcessImage, not closed here")
		})
		return
	}
	for name, mutate := range map[string]func(t *testing.T, target string){
		"atomic rename before init": func(t *testing.T, target string) {
			replacement := lab.install("early-src", eb)
			if err := os.Rename(replacement, target); err != nil {
				t.Fatal(err)
			}
		},
		"in-place overwrite before init": func(t *testing.T, target string) {
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if _, err := file.Write(mustRead(t, eb)); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name+"/manifest binds B", func(t *testing.T) {
			p, target := start(t, "early-"+strings.ReplaceAll(name, " ", "-"), digestB)
			mutate(t, target)
			if fileDigest(t, target) != digestB {
				t.Fatal("test precondition: the file was not replaced with B")
			}
			verdict, ran := release(t, p)
			requireRefused(t, verdict, ran, "A")
		})
	}
}

var _ = errors.Is
