#!/usr/bin/env python3
"""Replay the independent review counterexamples without editing source files.

Usage: python3 run_probes.py /absolute/path/to/praxis
PASS means the asserted counterexample/control was reproduced.
"""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

repo = Path(sys.argv[1]).resolve()
evidence = Path(__file__).resolve().parent
with tempfile.TemporaryDirectory(prefix="praxis-review2-") as temporary:
    work = Path(temporary)
    env = dict(os.environ, GOCACHE=str(work / "go-cache"))
    replacements = {}
    for package in ("cmd/praxis", "pkg/contracts", "internal/goaldrive"):
        name = package.replace("/", "_") + "_review_test.go"
        destination = work / name
        shutil.copyfile(evidence / (name + ".txt"), destination)
        replacements[str(repo / package / "review2_independent_test.go")] = str(destination)
    overlay = work / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replacements}))
    subprocess.run(
        ["go", "test", "-overlay=" + str(overlay), "./cmd/praxis", "./pkg/contracts",
         "./internal/goaldrive", "-run", "^TestReview2", "-count=1", "-v"],
        cwd=repo, env=env, check=True,
    )

    source = work / "image_probe.go"
    shutil.copyfile(evidence / "image_probe.go.txt", source)
    image_overlay = work / "image-overlay.json"
    image_overlay.write_text(json.dumps({"Replace": {
        str(repo / "cmd/inspectauth/main.go"): str(source)
    }}))
    for marker in ("A", "B"):
        subprocess.run(
            ["go", "build", "-overlay=" + str(image_overlay),
             "-ldflags=-X main.Marker=" + marker, "-o", str(work / ("image-" + marker)),
             "./cmd/inspectauth"], cwd=repo, env=env, check=True,
        )
    active = work / "active-image"
    shutil.copy2(work / "image-A", active)
    build_info = subprocess.check_output(["go", "version", "-m", str(active)], text=True, env=env)
    settings = {}
    for line in build_info.splitlines():
        if "vcs." in line and "=" in line:
            key, value = line.strip().split()[-1].split("=", 1)
            settings[key] = value
    digest = "sha256:" + "1" * 64
    manifest = {
        "version": "1", "kernel_version": "praxis-human-interface-bootstrap-v4/1",
        "source_commit": settings["vcs.revision"], "source_tree_digest": digest,
        "build_modified": settings["vcs.modified"] == "true",
        "active_binary_path": str(active),
        "active_binary_digest": "sha256:" + hashlib.sha256((work / "image-B").read_bytes()).hexdigest(),
        "goals_package_id": "p", "goals_package_version": "1",
        "goals_package_content_digest": digest, "goals_package_executable_digest": digest,
        "goals_package_contract_digest": digest, "validation_profile_digest": digest,
        "specification_bundle_digest": digest, "qualification_evidence_digest": digest,
    }
    manifest_path = work / "image-manifest.json"
    manifest_path.write_text(json.dumps(manifest))
    process = subprocess.Popen([str(active), str(manifest_path)], stdin=subprocess.PIPE,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    first = process.stdout.readline()
    replacement = work / "replacement"
    shutil.copy2(work / "image-B", replacement)
    replacement.replace(active)
    output, errors = process.communicate("\n", timeout=20)
    print(first + output + errors, end="")
    assert process.returncode == 0
    assert "RUNNING A" in first and "running=A result=<nil>" in output
