#!/usr/bin/env python3
"""Bounded, offline CLI route probe against base and this source candidate."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

BASE = "434eeedb98df44350b0bb00b5bb1b9071d3a5486"


def digest(data: bytes) -> str:
    return "sha256:" + hashlib.sha256(data).hexdigest()


def command(args: list[str], cwd: Path, env: dict[str, str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, check=False)


def checked(args: list[str], cwd: Path, env: dict[str, str]) -> str:
    result = command(args, cwd, env)
    if result.returncode:
        raise RuntimeError(f"{' '.join(args)} failed ({result.returncode}):\n{result.stdout[-3000:]}")
    return result.stdout.strip()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--repo", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    repo = args.repo.resolve()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    env = dict(os.environ, GOPROXY="off", GOSUMDB="off")
    source_commit = checked(["git", "rev-parse", "HEAD"], repo, env)
    source_tree = checked(["git", "rev-parse", "HEAD^{tree}"], repo, env)
    base_tree = checked(["git", "rev-parse", f"{BASE}^{{tree}}"], repo, env)
    if source_commit == BASE:
        raise RuntimeError("candidate source is unchanged base")
    with tempfile.TemporaryDirectory(prefix="praxis-oi035-local-cli-") as scratch:
        root = Path(scratch)
        base_worktree = root / "base"
        checked(["git", "worktree", "add", "--detach", str(base_worktree), BASE], repo, env)
        try:
            base_binary = root / "praxis-base"
            candidate_binary = root / "praxis-candidate"
            checked(["go", "build", "-o", str(base_binary), "./cmd/praxis"], base_worktree, env)
            checked(["go", "build", "-o", str(candidate_binary), "./cmd/praxis"], repo, env)
            # The old CLI misroutes local: through GitHub. Force its network
            # attempt to a closed loopback proxy so the negative probe is
            # read-only and cannot contact a remote service.
            base_env = dict(env, HTTPS_PROXY="http://127.0.0.1:1", HTTP_PROXY="http://127.0.0.1:1", ALL_PROXY="http://127.0.0.1:1", NO_PROXY="")
            negatives = {
                "base-negative-install.txt": ([str(base_binary), "install", "local:probe/dynamic@1"], "api.github.com"),
                "base-negative-update.txt": ([str(base_binary), "update", "probe/dynamic", "--to", "local:probe/dynamic@2"], "unknown update option"),
            }
            for name, (cmd, expected) in negatives.items():
                result = command(cmd, base_worktree, base_env)
                (output / name).write_text(result.stdout, encoding="utf-8")
                if result.returncode == 0 or expected not in result.stdout:
                    raise RuntimeError(f"unchanged base did not reject {name}: {result.returncode} {result.stdout}")
            for name, cmd in {
                "candidate-help-install.txt": [str(candidate_binary), "help", "install"],
                "candidate-help-update.txt": [str(candidate_binary), "help", "update"],
            }.items():
                result = command(cmd, repo, env)
                (output / name).write_text(result.stdout, encoding="utf-8")
                if result.returncode or "local:<package-id>@<version>" not in result.stdout:
                    raise RuntimeError(f"candidate help failed: {name}: {result.stdout}")
            focused = command(["go", "test", "./cmd/praxis", "-run", "TestLocalCLI", "-count=1", "-v"], repo, env)
            (output / "candidate-test.log").write_text(focused.stdout, encoding="utf-8")
            if focused.returncode or "--- PASS: TestLocalCLIInstallUpdateAndRemovalScratch" not in focused.stdout:
                raise RuntimeError(f"candidate CLI lifecycle test failed:\n{focused.stdout[-3000:]}")
        finally:
            checked(["git", "worktree", "remove", "--force", str(base_worktree)], repo, env)
    files = {path.name: digest(path.read_bytes()) for path in sorted(output.iterdir()) if path.is_file()}
    receipt = {
        "status": "BOUNDED_SCRATCH_PASS",
        "base_commit": BASE,
        "base_tree": base_tree,
        "source_commit": source_commit,
        "source_tree": source_tree,
        "probe_digest": digest(Path(__file__).read_bytes()),
        "files": files,
        "bounds": ["offline disposable fixtures", "ephemeral signing keys and governed approval rows", "no live signing, publisher enrollment, FAA, Keychain, package deployment, core replacement, or activation"],
    }
    canonical = json.dumps(receipt, sort_keys=True, separators=(",", ":")).encode()
    receipt["receipt_digest"] = digest(canonical)
    (output / "receipt.json").write_text(json.dumps(receipt, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps({"status": receipt["status"], "source_commit": source_commit, "source_tree": source_tree, "receipt_digest": receipt["receipt_digest"]}, sort_keys=True))


if __name__ == "__main__":
    main()
