package bootstrapv4

import (
	"os"
	"strings"
	"testing"
)

// The manifest digest is its own check: a manifest that binds bytes other than
// this process's image is refused although the file, the inode, the pathname and
// the kernel's code identity are all untouched.
func TestRunningImageManifestDigestMustBeTheImageDigest(t *testing.T) {
	lab := newImageLab(t)
	target := lab.install("digest-only", lab.a)
	control := startProbe(t, target, lab.manifest(target, lab.digestA))
	if got := control.line(t); got != "RUNNING A" {
		t.Fatalf("unexpected first line %q", got)
	}
	if verdict, _ := control.verdict(t); verdict != "<nil>" {
		t.Fatalf("control: the exact manifest was refused: %s", verdict)
	}
	wrong := startProbe(t, target, lab.manifest(target, lab.digestB))
	if got := wrong.line(t); got != "RUNNING A" {
		t.Fatalf("unexpected first line %q", got)
	}
	verdict, ran := wrong.verdict(t)
	requireRefused(t, verdict, ran, "A")
	if !strings.Contains(verdict, "image digest mismatch") {
		t.Fatalf("refused for another reason than the digest: %s", verdict)
	}
}

// Appending to the file behind a running process changes its length, which the
// descriptor reports; the process no longer proves what it was started from.
func TestRunningImageLengthChangeIsRefused(t *testing.T) {
	lab := newImageLab(t)
	target := lab.install("length-change", lab.a)
	p := startProbe(t, target, lab.manifest(target, lab.digestA))
	if got := p.line(t); got != "RUNNING A" {
		t.Fatalf("unexpected first line %q", got)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Skipf("this operating system refuses to write a running executable (%v)", err)
	}
	if _, err := file.Write([]byte("trailing bytes")); err != nil {
		t.Fatal(err)
	}
	file.Close()
	verdict, ran := p.verdict(t)
	requireRefused(t, verdict, ran, "A")
	if !strings.Contains(verdict, "changed size") {
		t.Fatalf("refused for another reason than the length change: %s", verdict)
	}
}
