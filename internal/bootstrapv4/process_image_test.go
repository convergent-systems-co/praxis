package bootstrapv4

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func buildProbe(t *testing.T, dir, marker string) string {
	t.Helper()
	out := filepath.Join(dir, "probe-"+marker)
	cmd := exec.Command("go", "build", "-ldflags", "-X main.marker="+marker, "-o", out, "./testdata/imageprobe")
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build probe %s: %v\n%s", marker, err, combined)
	}
	return out
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type probeProcess struct {
	cmd    *exec.Cmd
	stdin  interface{ Write([]byte) (int, error) }
	stdout *bufio.Scanner
}

func startProbe(t *testing.T, path, manifest string) *probeProcess {
	t.Helper()
	cmd := exec.Command(path, "run", manifest)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &probeProcess{cmd: cmd, stdin: in, stdout: bufio.NewScanner(out)}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return p
}

func (p *probeProcess) line(t *testing.T) string {
	t.Helper()
	if !p.stdout.Scan() {
		t.Fatalf("probe closed its output: %v", p.stdout.Err())
	}
	return p.stdout.Text()
}

// TestRunningImageVerificationSurvivesPathnameReplacement is the B11
// regression. Two builds share VCS revision and modified state but differ in
// bytes. Process A starts, its pathname is atomically replaced with B, and A
// then verifies. Verification must bind to the image A actually loaded:
// a manifest for B (what the pathname now holds) must be refused, and so must a
// manifest for A (its path no longer names the running image).
func TestRunningImageVerificationSurvivesPathnameReplacement(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two executables")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	dir := t.TempDir()
	a, b := buildProbe(t, dir, "A"), buildProbe(t, dir, "B")
	idBytes, err := exec.Command(a, "identity").Output()
	if err != nil {
		t.Fatal(err)
	}
	var identity struct{ Revision, Modified string }
	if err := json.Unmarshal([]byte(strings.ToLower(string(idBytes))), &identity); err != nil {
		t.Fatal(err)
	}
	modified, _ := strconv.ParseBool(identity.Modified)
	manifestFor := func(path, digest string) string {
		body, _ := json.Marshal(Manifest{ActiveBinaryPath: path, ActiveBinaryDigest: digest, SourceCommit: identity.Revision, BuildModified: modified})
		file := filepath.Join(dir, "manifest-"+filepath.Base(path)+"-"+digest[7:15]+".json")
		if err := os.WriteFile(file, body, 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}
	install := func(name, source string) string {
		body, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, name)
		if err := os.WriteFile(target, body, 0o755); err != nil {
			t.Fatal(err)
		}
		return target
	}
	digestA, digestB := fileDigest(t, a), fileDigest(t, b)
	if digestA == digestB {
		t.Fatal("probe builds must differ in bytes")
	}

	if identity.Revision != "" {
		t.Run("positive control: an unreplaced image verifies", func(t *testing.T) {
			target := install("control", a)
			p := startProbe(t, target, manifestFor(target, digestA))
			if got := p.line(t); got != "RUNNING A" {
				t.Fatalf("unexpected first line %q", got)
			}
			_, _ = p.stdin.Write([]byte("go\n"))
			if got := p.line(t); got != "VERIFY <nil>" {
				t.Fatalf("the genuine running image was refused: %q", got)
			}
		})
	}

	for name, digest := range map[string]string{"manifest for the replacement bytes": digestB, "manifest for the running bytes": digestA} {
		t.Run(name, func(t *testing.T) {
			target := install("replaced-"+digest[7:15], a)
			p := startProbe(t, target, manifestFor(target, digest))
			if got := p.line(t); got != "RUNNING A" {
				t.Fatalf("unexpected first line %q", got)
			}
			replacement := install("replacement-"+digest[7:15], b)
			if err := os.Rename(replacement, target); err != nil {
				t.Fatal(err)
			}
			_, _ = p.stdin.Write([]byte("go\n"))
			got := p.line(t)
			if !strings.HasPrefix(got, "VERIFY ") || got == "VERIFY <nil>" {
				t.Fatalf("a process outliving its own replacement verified: %q", got)
			}
			t.Log(got)
		})
	}
}
