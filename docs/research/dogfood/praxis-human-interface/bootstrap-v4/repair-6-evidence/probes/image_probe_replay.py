#!/usr/bin/env python3
"""Replay Review #3's process-image A/B drive against the current source, unchanged.

Builds two probe binaries A and B (same VCS identity, different bytes via -X main.marker) from
the reviewer's probe main, with a tiny pre-init sleeper package supplied through `go build -overlay`
(the repository is not modified), runs the reviewer's image-probe-drive.py, then compares the
drive's per-scenario results with Repair 5's replay after normalising digests and temporary paths.

  python3 image_probe_replay.py <output-dir>
"""
import json, os, re, shutil, subprocess, sys, tempfile
ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), *[".."] * 7))
DOGFOOD = os.path.join(ROOT, "docs/research/dogfood/praxis-human-interface")
R3 = os.path.join(DOGFOOD, "reviews/bootstrap-v4-kernel-astra-review-3-evidence/r3-activation")
R5LOG = os.path.join(DOGFOOD, "bootstrap-v4/repair-5-evidence/qualification/image-probe-replay.log")
OUT = os.path.abspath(sys.argv[1]); os.makedirs(OUT, exist_ok=True)
work = tempfile.mkdtemp(prefix="imgprobe-")
main_go = os.path.join(work, "main.go"); shutil.copy(os.path.join(R3, "probe-src-main.go.txt"), main_go)
sleeper = os.path.join(work, "sleep.go")
open(sleeper, "w").write('''package aaasleep

import (
	"os"
	"time"
)

// init runs before internal/bootstrapv4 (packages initialise in dependency order, then by path):
// it announces itself through PROBE_SLEEP_FILE and holds so the drive can replace the image
// between exec and bootstrapv4's own initialisation.
func init() {
	if f := os.Getenv("PROBE_SLEEP_FILE"); f != "" {
		_ = os.WriteFile(f, []byte("x"), 0o644)
		time.Sleep(2 * time.Second)
	}
}
''')
overlay = os.path.join(work, "overlay.json")
json.dump({"Replace": {os.path.join(ROOT, "cmd/zzprobe/main.go"): main_go, os.path.join(ROOT, "internal/aaasleep/sleep.go"): sleeper}}, open(overlay, "w"))
drive_dir = os.path.join(work, "drive"); os.makedirs(drive_dir)
shutil.copy(os.path.join(R3, "image-probe-drive.py"), drive_dir)
for name in ("A", "B"):
    subprocess.run(["go", "build", "-overlay=" + overlay, "-ldflags=-X main.marker=" + name, "-o", os.path.join(drive_dir, "probe-" + name), "./cmd/zzprobe"], cwd=ROOT, check=True)
out = subprocess.run([sys.executable, os.path.join(drive_dir, "image-probe-drive.py")], capture_output=True, text=True, timeout=600).stdout
open(os.path.join(OUT, "image-probe-replay.log"), "w").write(out)

def normalise(text):
    text = re.sub(r"sha256:[0-9a-f]{64}", "sha256:H", text)
    text = re.sub(r"/private/var/folders/\S+?/scn-\w+|/var/folders/\S+?/scn-\w+", "<tmp>", text)
    text = re.sub(r"code-directory hash [0-9a-f]{40}", "code-directory hash <cdhash>", text)  # a property of the freshly built probe binary
    text = re.sub(r"(got sha256:H want sha256:H).*", r"\1", text)
    return [l.strip() for l in text.splitlines() if l.strip()]
new, old = normalise(out), normalise(open(R5LOG).read())
same = new == old
print(f"scenarios={sum(1 for l in new if l.startswith('###'))} normalised transcript {'IDENTICAL to Repair 5 replay' if same else 'DIFFERS'}")
if not same:
    import difflib
    print("\n".join(difflib.unified_diff(old, new, "repair5", "repair6", lineterm="")))
shutil.rmtree(work, ignore_errors=True)
sys.exit(0 if same else 1)
