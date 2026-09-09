# Bundle b2-issue52 — Implementation Plan

Spec: `docs/develop/specs/b2-issue52.md` (enhanced spec — plan against this, not the raw issue).

## Summary

Add a `praxis` console script, relocate `schemas/v1/*.schema.json` into
`src/praxis_contracts/schemas/v1/` as packaged data resolved via
`importlib.resources`, add a clean-environment install check, and bring
`README.md` in line with the new behavior.

A single shared helper, `src/praxis_contracts/schema_paths.py`
(`SCHEMA_DIR`, `schema_path(filename)`), is introduced in the bootstrap task
so every other task — including the 13 test files that independently
compute a schema directory — converges on one resolution mechanism instead
of each re-deriving `Path(__file__).resolve().parent... / "schemas" / "v1"`.
This is why the DAG fans out from one bootstrap task: every consumer needs
that helper and the physically-relocated files to exist, but none of the
consumers need each other.

## Task graph (prose)

```
T1 (bootstrap: pyproject + schema relocation + schema_paths helper)
├── T3 (praxis_runtime schema paths)
│     └── T10 (fix test_repair_findings_b3_issue4.py's SCHEMA_PATH assertion)
├── T4 (praxis_eval schema paths)
├── T5 (praxis_evidence schema paths)
├── T6 (praxis_learning schema paths)
├── T7 (praxis_policy schema paths)
├── T8 (praxis_overlay schema paths)
├── T9 (13 test files: adopt schema_paths instead of their own repo-root computation)
└── T11 (clean-env install check)  [also depends on T2]

T2 (praxis_cli.main console-script target) ──────────────┘ (feeds T11)

T12 (README consistency) — independent, no code dependency
```

Critical path: T1 → T3 → T10 (length 3, ceiling 12).

## T1 — Bootstrap: packaging metadata + schema relocation + shared resolver

**Files:**
- `pyproject.toml`
- `schemas/v1/*.schema.json` (moved away — directory removed)
- `src/praxis_contracts/schemas/v1/*.schema.json` (new location, same 22 filenames)
- `src/praxis_contracts/schema_paths.py` (new)

**Interfaces:**
- `SCHEMA_DIR: pathlib.Path` — resolves to the installed `praxis_contracts/schemas/v1/` directory via `importlib.resources.files("praxis_contracts") / "schemas" / "v1"`.
- `schema_path(filename: str) -> pathlib.Path` — returns `SCHEMA_DIR / filename`.

**Depends on:** (none)

**Steps:**
- [ ] In `pyproject.toml`, add a `[project.scripts]` table: `praxis = "praxis_cli.main:main"`.
- [ ] In `pyproject.toml`, change `[project.optional-dependencies] dev = ["pytest>=7"]` to `dev = ["pytest>=7", "build"]`.
- [ ] In `pyproject.toml`, add `[tool.setuptools.package-data]` with `praxis_contracts = ["schemas/v1/*.schema.json"]`.
- [ ] `git mv schemas/v1 src/praxis_contracts/schemas/v1` — moves all 22 `*.schema.json` files in one commit-friendly operation; the top-level `schemas/` directory disappears once it's empty (git does not track empty directories).
- [ ] Create `src/praxis_contracts/schema_paths.py`:
  ```python
  from __future__ import annotations

  import importlib.resources
  from pathlib import Path

  SCHEMA_DIR: Path = importlib.resources.files("praxis_contracts") / "schemas" / "v1"


  def schema_path(filename: str) -> Path:
      return SCHEMA_DIR / filename
  ```
- [ ] Run `.venv/bin/python -m pip install -e .[dev]` and confirm it completes with no error. This re-registers the `praxis` entry point and pulls in `build`. The `praxis` command will not actually run yet (T2 hasn't landed) — that's expected.
- [ ] Do not attempt a full `.venv/bin/python -m pytest` run here: 13 test files (T9) and 8 `src/` modules (T3–T8) still reference the old `schemas/v1/` location and will fail to find schema files until their own tasks land. That is expected at this point in the DAG, not a defect in this task.

## T2 — `praxis` CLI entry point

**Files:**
- `src/praxis_cli/__init__.py` (new)
- `src/praxis_cli/main.py` (new)
- `tests/test_praxis_cli.py` (new)

**Interfaces:**
- `praxis_cli.main.main() -> None` — prints the installed `praxis-contracts` distribution version to stdout and returns (a console-script wrapper calling `sys.exit(main())` then exits 0). No subcommands, no argument parsing — printing the version is the entire behavior for this bundle.

**Depends on:** (none — footprint doesn't overlap `pyproject.toml`; the entry-point registration in T1 and this module's existence are independent edits that only need to both be present by the time T11 runs)

**Steps:**
- [ ] Create `src/praxis_cli/__init__.py` (empty).
- [ ] Create `src/praxis_cli/main.py`:
  ```python
  from __future__ import annotations

  import importlib.metadata


  def main() -> None:
      print(importlib.metadata.version("praxis-contracts"))
  ```
  Use `importlib.metadata.version("praxis-contracts")` — not a `pyproject.toml` read — because `pyproject.toml` is not shipped inside a built wheel and this must work from an installed, non-editable install (see T11).
- [ ] Add `tests/test_praxis_cli.py` asserting `main()` writes a version-looking string (e.g. matches `\d+\.\d+\.\d+`) to stdout via `capsys`, and does not raise.
- [ ] Run `.venv/bin/python -m pytest tests/test_praxis_cli.py`.

## T3 — `praxis_runtime` schema path migration

**Files:**
- `src/praxis_runtime/events.py`
- `src/praxis_runtime/state.py`
- `src/praxis_runtime/graph.py`
- `src/praxis_runtime/resources/leases.py`
- `src/praxis_runtime/resources/claims.py`

**Interfaces:** (unchanged) each file keeps its existing `SCHEMA_PATH` module-level constant; only how it's computed changes.

**Depends on:** T1

**Steps:**
- [ ] `events.py:34` — replace `SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "event.schema.json"` with `from praxis_contracts.schema_paths import schema_path` + `SCHEMA_PATH = schema_path("event.schema.json")`. `from pathlib import Path` stays (still used elsewhere in the file).
- [ ] `state.py:21` — same pattern for `"run-state.schema.json"`. `from pathlib import Path` stays (used at `state.py:68`).
- [ ] `graph.py:19` — same pattern for `"graph.schema.json"`. `from pathlib import Path` stays (used at `graph.py:50`).
- [ ] `resources/leases.py:54` — same pattern for `"lease.schema.json"`. `from pathlib import Path` stays (used at `leases.py:119`).
- [ ] `resources/claims.py:28-29` — same pattern for `"resource-claim.schema.json"`. Check whether `Path` is used elsewhere in this file after the edit; remove `from pathlib import Path` if it becomes unused.
- [ ] Run `.venv/bin/python -m pytest tests/ -k "event or state or graph or lease or claim"` to spot-check before the full-suite run at bundle verification time.

## T4 — `praxis_eval` schema path migration

**Files:**
- `src/praxis_eval/types.py`
- `src/praxis_eval/parity.py`

**Depends on:** T1

**Steps:**
- [ ] `types.py:20` — replace `SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"` with `from praxis_contracts.schema_paths import SCHEMA_DIR`. Leave the four `CANDIDATE_CONFIG_SCHEMA_PATH` / `EVALUATION_RECORD_SCHEMA_PATH` / `PROMOTION_POLICY_SCHEMA_PATH` / `PROMOTION_RECORD_SCHEMA_PATH` lines unchanged — they already derive from `SCHEMA_DIR`. Remove `from pathlib import Path` if it becomes unused.
- [ ] `parity.py:16-18` — replace the `PARITY_FIXTURE_SCHEMA_PATH = (Path(__file__)...)` block with `from praxis_contracts.schema_paths import schema_path` + `PARITY_FIXTURE_SCHEMA_PATH = schema_path("parity-fixture.schema.json")`. Remove `from pathlib import Path` if it becomes unused.
- [ ] Run `.venv/bin/python -m pytest tests/test_parity_fixtures.py tests/test_parity_evaluation_records.py -k schema or true` (or just spot-check imports resolve) before bundle-level full-suite verification.

## T5 — `praxis_evidence` schema path migration

**Files:** `src/praxis_evidence/types.py`

**Depends on:** T1

**Steps:**
- [ ] `types.py:20` — replace `SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"` with `from praxis_contracts.schema_paths import SCHEMA_DIR`. Leave `PROOF_RECORD_SCHEMA_PATH = SCHEMA_DIR / "proof-record.schema.json"` unchanged. Remove `from pathlib import Path` if unused afterward.

## T6 — `praxis_learning` schema path migration

**Files:** `src/praxis_learning/types.py`

**Depends on:** T1

**Steps:**
- [ ] `types.py:15` — replace `SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"` with `from praxis_contracts.schema_paths import SCHEMA_DIR`. Leave `OBSERVATION_SCHEMA_PATH` / `HEURISTIC_CANDIDATE_SCHEMA_PATH` unchanged. Remove `from pathlib import Path` if unused afterward.

## T7 — `praxis_policy` schema path migration

**Files:** `src/praxis_policy/profiles.py`

**Depends on:** T1

**Steps:**
- [ ] `profiles.py:20` — replace `_SCHEMA_PATH = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1" / "policy-profile.schema.json"` with `from praxis_contracts.schema_paths import schema_path` + `_SCHEMA_PATH = schema_path("policy-profile.schema.json")`. Remove `from pathlib import Path` (only use in this file per current source).

## T8 — `praxis_overlay` schema path migration

**Files:** `src/praxis_overlay/manifest.py`

**Depends on:** T1

**Steps:**
- [ ] `manifest.py:21-23` — replace the `SCHEMA_PATH = (Path(__file__)...)` block with `from praxis_contracts.schema_paths import schema_path` + `SCHEMA_PATH = schema_path("overlay-manifest.schema.json")`. Remove `from pathlib import Path` (only use in this file per current source).

## T9 — Test files: adopt `schema_paths` instead of an independent repo-root computation

The original spec calls out one bare, genuinely cwd-relative literal
(`tests/test_overlay_trivial_fixture.py:37`) by name. Investigation during
planning found 12 more test files that independently compute
`REPO_ROOT / "schemas" / "v1"` (or the `Path(__file__)`-anchored
equivalent) — these are not cwd-relative today, but they *will* break the
moment `schemas/v1/` physically moves in T1, regardless of cwd, because the
directory they point at will no longer exist. Criterion 1 (full suite
passes) requires all of them to be fixed, not just the one literal named in
the spec's prose.

**Files:**
- `tests/test_malformed_contracts.py`
- `tests/test_repair_findings_b10_issue12.py`
- `tests/test_valid_contracts.py`
- `tests/test_repair_findings.py`
- `tests/test_executor_end_to_end.py`
- `tests/test_authority_boundaries.py`
- `tests/test_core_overlay_boundary.py`
- `tests/test_subprocess_executor.py`
- `tests/test_fake_capability_executor.py`
- `tests/test_retry_budgets.py`
- `tests/test_version_mismatch.py`
- `tests/test_overlay_trivial_fixture.py`
- `tests/test_repair_findings_b5_issue6.py`

**Depends on:** T1 (needs `praxis_contracts.schema_paths` and the physically-relocated files; does not need T3–T8 since none of these files import schema-path constants from those modules)

**Steps:**
- [ ] `test_malformed_contracts.py:16` — replace `SCHEMAS_DIR = Path(__file__).resolve().parent.parent / "schemas" / "v1"` with `from praxis_contracts.schema_paths import SCHEMA_DIR as SCHEMAS_DIR`.
- [ ] `test_repair_findings_b10_issue12.py:38-39` — replace `_REQUIREMENT_SCHEMA_PATH = (Path(__file__)...)` with `from praxis_contracts.schema_paths import schema_path` + `_REQUIREMENT_SCHEMA_PATH = schema_path("requirement.schema.json")`.
- [ ] `test_valid_contracts.py:16` — replace `SCHEMAS_DIR = REPO_ROOT / "schemas" / "v1"` with `from praxis_contracts.schema_paths import SCHEMA_DIR as SCHEMAS_DIR`. Leave `EXAMPLES_DIR = REPO_ROOT / "examples"` untouched.
- [ ] `test_repair_findings.py:23` — same `SCHEMAS_DIR` replacement as above.
- [ ] `test_executor_end_to_end.py:33` — same `SCHEMAS_DIR` replacement.
- [ ] `test_authority_boundaries.py:21` — same `SCHEMAS_DIR` replacement.
- [ ] `test_core_overlay_boundary.py:48` — same `SCHEMAS_DIR` replacement.
- [ ] `test_subprocess_executor.py:24` — same `SCHEMAS_DIR` replacement.
- [ ] `test_fake_capability_executor.py:27` — same `SCHEMAS_DIR` replacement.
- [ ] `test_retry_budgets.py:24` — same `SCHEMAS_DIR` replacement.
- [ ] `test_version_mismatch.py:17` — same `SCHEMAS_DIR` replacement.
- [ ] `test_overlay_trivial_fixture.py:37` — replace the bare `_EVIDENCE_REQUIREMENT_SCHEMA = Path("schemas/v1/evidence-requirement.schema.json")` with `from praxis_contracts.schema_paths import schema_path` + `_EVIDENCE_REQUIREMENT_SCHEMA = schema_path("evidence-requirement.schema.json")`. This is the literal named in the spec.
- [ ] `test_repair_findings_b5_issue6.py:449` — replace the in-function `schema_dir = Path(__file__).resolve().parent.parent / "schemas" / "v1"` with an import of `praxis_contracts.schema_paths.SCHEMA_DIR`, used as `schema_dir` locally.
- [ ] `test_repair_findings_b5_issue6.py:738` — replace the inline `Path(__file__).resolve().parent.parent / "schemas" / "v1" / "proof-record.schema.json"` call argument with `schema_paths.schema_path("proof-record.schema.json")` (or the imported `schema_path` helper).
- [ ] Run `.venv/bin/python -m pytest <each file touched above>` and confirm they pass once T1 has landed.

## T10 — Fix `test_repair_findings_b3_issue4.py`'s hardcoded expected `SCHEMA_PATH`

This file asserts `praxis_runtime.state`'s public `SCHEMA_PATH` constant
equals a specific literal path. That literal must change to match what T3
actually produces, so this task has a real (not just footprint) dependency
on T3, not just T1.

**Files:** `tests/test_repair_findings_b3_issue4.py`

**Depends on:** T1, T3

**Steps:**
- [ ] In `test_state_module_exposes_public_schema_path` (around line 190-193), replace the expected value `REPO_ROOT / "schemas" / "v1" / "run-state.schema.json"` with `schema_paths.schema_path("run-state.schema.json")` (import `from praxis_contracts.schema_paths import schema_path` or reuse an existing import), and assert `state_module.SCHEMA_PATH == schema_path("run-state.schema.json")`.
- [ ] Run `.venv/bin/python -m pytest tests/test_repair_findings_b3_issue4.py` and confirm it passes against T3's already-landed `state.py`.

## T11 — Clean-environment install check

**Files:** `scripts/check_clean_install.py` (new; creates the `scripts/` directory)

**Depends on:** T1, T2

**Steps:**
- [ ] Create `scripts/check_clean_install.py`, runnable via `.venv/bin/python scripts/check_clean_install.py`, that:
  - Builds the package with `python -m build --outdir <tmp>` (via `subprocess.run([sys.executable, "-m", "build", ...], check=True)`) into a `tempfile.TemporaryDirectory()`.
  - Creates a fresh venv with `venv.create(<tmp_venv>, with_pip=True)` inside another `tempfile.TemporaryDirectory()` located outside the repo tree.
  - Installs the built wheel (glob the build output dir for `*.whl`) into that venv via `subprocess.run([<venv_python>, "-m", "pip", "install", <wheel_path>], check=True)` — not `-e .`.
  - Runs, with `cwd` set to a scratch temp directory that is *not* the repo checkout:
    1. `<venv_bin>/praxis` (or `<venv_python> -m praxis_cli.main`) and asserts exit code 0 and non-empty stdout matching a version-like pattern.
    2. A short inline snippet via `<venv_python> -c "..."` that imports `praxis_contracts.validator.validate_document` and `importlib.resources`, resolves `requirement.schema.json` from the *installed* `praxis_contracts` package (not the repo's `schemas/` or `examples/`), builds a minimal valid instance matching that schema's required shape inline (verify the required shape against `src/praxis_contracts/schemas/v1/requirement.schema.json` and `tests/test_valid_contracts.py`'s existing example when writing this — do not read the schema literally into this plan), and confirms `validate_document` does not raise.
  - Cleans up both temporary directories (the `TemporaryDirectory` context managers handle this).
  - Exits non-zero (propagate `subprocess.CalledProcessError` or raise `SystemExit(1)` on an explicit assertion) if any step fails, and exits 0 on success.
- [ ] Run `.venv/bin/python scripts/check_clean_install.py` from the repo checkout and confirm it passes end-to-end.

## T12 — README consistency

**Files:** `README.md`

**Depends on:** (none)

**Steps:**
- [ ] `README.md:298` — remove "there is no `praxis` console script"; state that `pip install` (editable or from a built wheel) provides a `praxis` command, and point to `praxis --version` as the install smoke-check.
- [ ] `README.md:306-308` — remove the "not yet packaged as package data" / "run everything from a repo checkout, not an installed wheel" claims; state that `schemas/v1/*.schema.json` now ships as package data under `praxis_contracts` and is resolved via `importlib.resources` regardless of cwd or install location.
- [ ] `README.md:320` — remove "There is no top-level `praxis` CLI yet"; note that `praxis --version` exists today, while the quickstart below it remains the way to drive a graph as a library (this bundle adds no graph-driving subcommands — that's separate, later work per the spec's explicit scope boundary).
- [ ] Leave the rest of the Installation/Usage sections, including the Python quickstart snippet, unchanged.

## Acceptance-criteria coverage

| Criterion (enhanced spec) | Covered by |
|---|---|
| 1. Full suite passes from repo checkout | All tasks jointly (T1 unblocks, T3–T10 fix every schema-path reference) |
| 2. `praxis` console script exists and works | T1 (entry point) + T2 (target module) |
| 3. Schemas packaged, resolve regardless of cwd/install | T1 (relocation + package-data) + T3–T8 (src consumers) + T9 (test consumers) |
| 4. Clean-environment install check | T11 |
| 5. No remaining cwd-relative `schemas/...` reference | T9 (the literal named in the spec) + T3–T8 (the `SCHEMA_DIR`/`SCHEMA_PATH` constants, which become cwd-independent even though they were already `Path(__file__)`-anchored) |
| 6. README consistency | T12 |
