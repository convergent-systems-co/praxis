# Plan: b1-issue41

Source: `docs/develop/specs/b1-issue41.md` (enhanced spec — plan against its Clarified acceptance
criteria and Assumptions, not the raw issue text embedded at its top).

## Why this is a single task

The entire deliverable is one new `Executor` subclass in one new file plus its test file — the same
shape as the just-merged sibling bundle `b1-issue40` (`ClaudeCliExecutor`), whose plan is reused as
the structural template here (see `docs/develop/plans/b1-issue40.md`). `Executor` is an ABC with six
abstract methods (`capabilities`, `health`, `launch`, `status`, `cancel`, `result`); a class
implementing only a subset is not instantiable, so there is no way to split "discovery/capabilities/
health" from "launch/status/cancel/result" into two independently-testable, committable slices
without one of them shipping stub methods the other later replaces — exactly the narrative-driven
split (and half-finished implementation) the planning rules warn against. Both halves would also
declare the identical two files, so splitting buys no real DAG concurrency (they would serialize on
footprint overlap regardless of how the dependency edge is drawn). One task, `T1`, covers every
acceptance criterion, including the two audited-gap remediations (env isolation, broader redaction)
this bundle must not repeat from `claude_cli.py`.

## Design decisions pinned by this plan

The enhanced spec deliberately leaves two categories of decision to the implementer's own session,
not to planning: (a) the exact Codex auth-probe command and any extra OpenAI env vars beyond
`OPENAI_API_KEY` (Clarified AC 6, 8 — genuine investigation work, `codex` is installed in this
worktree's environment per Clarified AC 6 so it is concretely checkable), and (b) the exact CLI argv
shape (Clarified AC 11). Everything else is pinned here so the implementer spends zero time on
structural design and starts writing code and tests immediately.

1. **New module:** `src/praxis_executors/adapters/codex_cli.py`, class `CodexCliExecutor(Executor)`.
   - `__init__(self, executor_id: str) -> None` — no other constructor parameters (mirrors
     `ClaudeCliExecutor`/`SubprocessExecutor`/`FakeCapabilityExecutor`; `executor_id` is
     caller-supplied, per Explicitly out of scope / Assumption 4). Internal state mirrors
     `ClaudeCliExecutor`: `self._processes: dict[str, subprocess.Popen]`,
     `self._results: dict[str, ExecutionResult]`, `self._cancelled: set[str]`.
   - Module constants `_SPEC_VERSION = "1.0.0"` and `_CLI_NAME = "codex"`.
   - Module docstring: one line naming the schema files this dict shape follows (mirror
     `claude_cli.py`'s docstring, substituting nothing else — the schema paths are unchanged).

2. **Credential redaction helper**, module-level in the same file (not shared with `claude_cli.py` —
   the two adapters' credential shapes differ and nothing else needs a shared module):
   ```python
   _CREDENTIAL_PATTERN = re.compile(r"sk-[A-Za-z0-9_-]{20,}")
   _REDACTED = "***REDACTED***"

   def _redact(text: str) -> str:
       return _CREDENTIAL_PATTERN.sub(_REDACTED, text)
   ```
   This covers both the legacy `sk-...` and project-scoped `sk-proj-...` OpenAI key shapes (Clarified
   AC 9 / Assumption 3) without enumerating them separately — both start `sk-` followed by an
   alphanumeric/`_`/`-` run. Apply `_redact(...)` to every piece of subprocess-sourced text (`stdout`,
   `stderr`, and the string form of any caught `OSError`) before it goes into
   `ExecutionResult.evidence`, `ExecutionResult.payload`, or an `ExecutorError` message. Pin the test
   fixtures so the test and implementation agree on what "credential-shaped" means:
   ```python
   FAKE_SECRET_LEGACY = "sk-FAKESECRETFAKESECRETFAKESECRETFAKE12"
   FAKE_SECRET_PROJECT = "sk-proj-FAKESECRETFAKESECRETFAKESECRETFAKE"
   ```
   Both are >20 chars after the `sk-` prefix, so both must be redacted by every redaction test below.

3. **`capabilities()`** — one `Capability` entry, exactly the three named kinds, `auth_transport` on
   that same entry, static regardless of live auth state (Clarified AC 3, 4):
   ```python
   def capabilities(self) -> dict:
       return {
           "spec_version": _SPEC_VERSION,
           "executor_id": self._executor_id,
           "capabilities": [
               {
                   "spec_version": _SPEC_VERSION,
                   "satisfies": [
                       {"kind": "coding"},
                       {"kind": "shell"},
                       {"kind": "filesystem"},
                   ],
                   "auth_transport": "subscription_cli",
               }
           ],
       }
   ```
   `capabilities()` never calls `shutil.which` or any subprocess — same static/live split
   `claude_cli.py` already implements (Assumption 1).

4. **`health()`** — the only place live detection happens, and where the real auth-probe
   investigation (Clarified AC 6) lands:
   ```python
   def health(self) -> ExecutorAvailability:
       cli_path = shutil.which(_CLI_NAME)
       if cli_path is None:
           return ExecutorAvailability.UNAVAILABLE
       self._probe_version(cli_path)
       authenticated = self._detect_authenticated(cli_path)
       if authenticated is False:
           # Deliberately no fallback branch here: an unauthenticated CLI
           # must resolve straight to UNAVAILABLE, never a metered API key.
           return ExecutorAvailability.UNAVAILABLE
       if authenticated is True:
           return ExecutorAvailability.AVAILABLE
       return ExecutorAvailability.DEGRADED
   ```
   - `_probe_version(self, cli_path: str) -> None` runs `subprocess.run([cli_path, "--version"],
     capture_output=True, text=True, timeout=5)` inside `try/except (OSError,
     subprocess.TimeoutExpired): pass` — best-effort, discarded either way (mirrors
     `claude_cli._probe_version`).
   - `_detect_authenticated(self, cli_path: str) -> bool | None` — **this is genuine
     implementer-owned investigation, not pre-resolved by this plan** (Clarified AC 6). Before
     writing this method: run `codex --help` and `codex auth --help` (the real `codex` binary is on
     PATH in this environment per the spec's own confirmation) looking for a safe, side-effect-free,
     sub-second subcommand/flag that reports auth state (e.g. something like `codex auth status` or
     an equivalent). If one exists: call it via `subprocess.run([...], capture_output=True, text=True,
     timeout=5)`, parse its exit code or stdout into `True`/`False`/`None`, and cite the exact command
     and how its output was interpreted in a code comment directly above the method. If, after
     actually checking, no such safe probe exists: implement the method to unconditionally `return
     None`, with a one-line comment naming what was checked (`codex --help`, `codex auth --help`) and
     why neither qualified. Either outcome is acceptable per the spec; silently defaulting to `None`
     without having checked is not.

5. **`launch`/`status`/`cancel`/`result`** — same `Popen`-based lifecycle as `ClaudeCliExecutor`
   (`_process_for`, `_terminal_status`, `uuid.uuid4().hex` handle ids, `ExecutorError` on an unknown
   handle) with two changes: an explicitly constructed, filtered subprocess environment (the audited
   gap this bundle must not repeat, Clarified AC 8), and Codex's own argv shape (Clarified AC 11):
   - `request.parameters` must contain key `"prompt"` (a string); its absence raises
     `ExecutorError("request.parameters is missing required key 'prompt'")` (Clarified AC 10).
   - `request.parameters.get("extra_args", [])` (a `list[str]`, default empty) is appended verbatim
     after the prompt.
   - `launch()` resolves `cli_path = shutil.which(_CLI_NAME)`; if `None`, raise
     `ExecutorError("codex CLI is not available on PATH")` without attempting `Popen`.
   - **Env isolation (Clarified AC 8):** add a module constant
     `_ENV_VARS_TO_STRIP = ("OPENAI_API_KEY",)` and a helper:
     ```python
     def _subprocess_env() -> dict[str, str]:
         env = os.environ.copy()
         for key in _ENV_VARS_TO_STRIP:
             env.pop(key, None)
         return env
     ```
     Before writing `_ENV_VARS_TO_STRIP`, check `codex --help` (same investigation session as the
     auth probe above) for any additional OpenAI env vars it documents (org/project scoping,
     base-URL overrides, etc.); add each one found to the tuple. `OPENAI_API_KEY` must be present
     unconditionally regardless of what else is found (Assumption 2). Pass `env=_subprocess_env()`
     explicitly into every `subprocess.Popen(...)` call in `launch()` — never call `Popen` without an
     explicit `env` kwarg, since inheriting unfiltered is precisely the audited gap.
   - argv: pin `[cli_path, "exec", request.parameters["prompt"], *extra_args]` as the default
     (Codex's documented non-interactive one-shot subcommand is named `exec` in its published CLI
     surface). Since `codex` is installed in this environment, verify this against `codex --help`/
     `codex exec --help` during implementation and correct the literal `"exec"` argument (and cite the
     finding in a comment) if the real CLI documents a different non-interactive invocation — do not
     ship the pinned guess uncorrected if verification contradicts it.
   - `Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, env=_subprocess_env())`
     inside `try/except OSError as exc: raise ExecutorError(f"failed to launch codex CLI:
     {_redact(str(exc))}") from exc`.
   - `status()`/`cancel()`/`_process_for()`/`_terminal_status()` are a direct copy of
     `ClaudeCliExecutor`'s versions, adjusted to this class (Clarified AC 10: `ExecutorError` on
     unknown `handle_id` for `status`, `cancel`, and `result` alike).
   - `result()` mirrors `ClaudeCliExecutor.result()`, redacting before storing:
     ```python
     stdout, stderr = process.communicate()
     returncode = process.returncode
     result = ExecutionResult(
         status=self._terminal_status(handle.handle_id, returncode),
         evidence={"process-exit-status": returncode == 0},
         payload={
             "stdout": _redact(stdout),
             "stderr": _redact(stderr),
             "returncode": returncode,
         },
     )
     ```

## Task

### T1 — `CodexCliExecutor` adapter and its tests (issue #41)

**Files:** `src/praxis_executors/adapters/codex_cli.py` (new), `tests/test_codex_cli.py` (new)

**Depends on:** none

**Interfaces:** `CodexCliExecutor(Executor)` per "Design decisions pinned by this plan" above —
implement exactly those signatures and helper names; do not improvise different ones.

**Steps:**
- [ ] Re-read `src/praxis_executors/interface.py`, `src/praxis_executors/adapters/claude_cli.py`, and
      `src/praxis_contracts/schemas/v1/capability.schema.json` (already read during planning; the
      pinned design above is grounded in them) to confirm nothing has drifted since planning.
- [ ] Investigate the real `codex` binary in this session (`codex --help`, `codex auth --help`, and
      `codex exec --help` if it exists): identify (a) a safe, non-destructive, sub-second auth-status
      probe if one exists, (b) any OpenAI env vars beyond `OPENAI_API_KEY` that need stripping, and
      (c) the correct non-interactive invocation subcommand/flags. Record findings as code comments
      at the point each is used — this is the one piece of this task that cannot be done from the
      plan alone.
- [ ] Create `src/praxis_executors/adapters/codex_cli.py`: module docstring, imports (`os`, `re`,
      `shutil`, `subprocess`, `uuid`, plus the same `praxis_executors.interface` imports
      `claude_cli.py` uses), the `_SPEC_VERSION`/`_CLI_NAME`/`_CREDENTIAL_PATTERN`/`_REDACTED`/
      `_redact`/`_ENV_VARS_TO_STRIP`/`_subprocess_env` module-level pieces, and the
      `CodexCliExecutor` class implementing all six `Executor` methods exactly as pinned above.
- [ ] Create `tests/test_codex_cli.py` (follow `tests/test_claude_cli.py` for import/fixture style —
      a module-level `_executor()` helper constructing
      `CodexCliExecutor(executor_id="executor-codex-cli-1")`). Mock at the boundary:
      `unittest.mock.patch("praxis_executors.adapters.codex_cli.shutil.which", ...)` and
      `"...subprocess.Popen"` / `"...subprocess.run"` — never invoke a real `codex` process in these
      tests except the optional smoke test. Cover, at minimum:
      - [ ] `capabilities()` validates against `capability-advertisement.schema.json` via
            `praxis_contracts.validator.validate_document` + `praxis_contracts.schema_paths.SCHEMA_DIR`
            (mirror `test_claude_cli.py::test_capabilities_advertisement_validates_against_schema`).
      - [ ] `capabilities()`'s single capability entry has `satisfies` covering exactly
            `{"coding", "shell", "filesystem"}` and `auth_transport == "subscription_cli"` on that same
            entry (Clarified AC 3).
      - [ ] Calling `capabilities()` does not call the patched `shutil.which`/`subprocess.Popen`/`run`
            (proves Clarified AC 4 — static, not live-synced).
      - [ ] `health()` returns `UNAVAILABLE` when patched `shutil.which` returns `None` (CLI absent).
      - [ ] `health()` returns `AVAILABLE` when the CLI is present and `_detect_authenticated` is
            mocked to return `True`.
      - [ ] `health()` returns `UNAVAILABLE` when the CLI is present and `_detect_authenticated` is
            mocked to return `False` (the "never fall back" guarantee — no other branch exists).
      - [ ] `health()` returns `DEGRADED` when `_detect_authenticated` is mocked to return `None`
            (Clarified AC 5).
      - [ ] If investigation found a real safe auth probe: a test that patches `subprocess.run` (or
            `subprocess.check_output`, matching whatever was implemented) to simulate both the
            authenticated and unauthenticated probe outputs, and asserts `_detect_authenticated`
            returns `True`/`False` accordingly — proving `health()` can report `AVAILABLE` via that
            mechanism (spec Acceptance bullet 4). If no safe probe was found: a test asserting the
            real, unmocked `_detect_authenticated` returns `None` given only `shutil.which`/
            `subprocess.run` patched (mirrors `test_claude_cli.py`'s
            `test_detect_authenticated_unmocked_returns_none_by_default`), documenting the
            "explicitly documents why none exists" fallback path instead.
      - [ ] `health()` invokes `codex --version` via the patched `subprocess.run` (assert the mock was
            called with an argv ending in `"--version"`).
      - [ ] `launch()` without a `"prompt"` key in `request.parameters` raises `ExecutorError`.
      - [ ] `launch()` when patched `shutil.which` returns `None` raises `ExecutorError` without
            calling the patched `Popen`.
      - [ ] **Env isolation (spec Acceptance bullet 3):** with `OPENAI_API_KEY` (and any other
            identified vars) set in the parent process (`monkeypatch.setenv` or
            `patch.dict(os.environ, ...)`) and a mocked `Popen`, launch a request and assert the `env`
            kwarg the mock was called with does not contain `OPENAI_API_KEY` (nor any other stripped
            key) while still containing an unrelated pre-existing var (proving it's a filtered copy,
            not an empty/replacement dict).
      - [ ] A scripted successful run (mocked `Popen` whose `.poll()` then `.communicate()` simulate a
            zero exit) reaches `ExecutorStatus.SUCCEEDED` with `evidence == {"process-exit-status": True}`.
      - [ ] A scripted nonzero-exit run reaches `ExecutorStatus.FAILED` with
            `evidence == {"process-exit-status": False}`.
      - [ ] `cancel()` on a still-running mocked process calls `.terminate()` and drives the handle to
            `ExecutorStatus.CANCELLED`.
      - [ ] `status()`, `result()`, and `cancel()` each raise `ExecutorError` for an unknown handle
            (Clarified AC 10).
      - [ ] **Credential-safety (Clarified AC 9):** for both `FAKE_SECRET_LEGACY` and
            `FAKE_SECRET_PROJECT` pinned above, mock `Popen.communicate()` to return
            `(f"...{secret}...", f"...{secret}...")`; assert neither secret appears verbatim anywhere
            in `result(handle).evidence` or `result(handle).payload` (`stdout`/`stderr`). Separately,
            mock `Popen` construction to raise `OSError(f"launch failed: {secret}")` for each secret
            and assert the resulting `ExecutorError`'s `str(...)` does not contain it either — mirror
            `test_claude_cli.py::test_result_redacts_credential_shaped_secret_from_evidence_and_payload`
            and `..._launch_failure_redacts_credential_shaped_secret_from_error_message`, which already
            exercise real fields; do not regress behind that bar.
      - [ ] Optional real-CLI smoke test, `pytest.mark.skipif(shutil.which("codex") is None,
            reason="codex CLI not installed")`, doing one non-destructive real call (e.g.
            `CodexCliExecutor(executor_id="executor-codex-cli-smoke").health()` and asserting it
            returns one of the three `ExecutorAvailability` members) — must be skipped, never failed,
            when the CLI is absent. `codex` is installed in this environment per the spec, so this
            branch is concretely exercised here, not just a hypothetical CI path.
- [ ] Run `.venv/bin/python -m pytest tests/test_codex_cli.py -q`.
- [ ] Run the full suite: `.venv/bin/python -m pytest -q` (spec Acceptance bullet 5 — nothing else in
      the repo should be touched by this file addition, so this should be a clean pass).

---

## Coverage check against acceptance criteria

- Discovery (executable availability, version, filesystem/tool capability detection) → T1 (`health()`
  + `_probe_version`, `capabilities()`'s `filesystem`/`shell` kinds).
- Safe-only authentication detection, no direct credential extraction → T1 (`_detect_authenticated`
  investigation step, Clarified AC 6; Explicitly out of scope's credential-extraction boundary).
- `capabilities()` = `coding`/`shell`/`filesystem`, `auth_transport: "subscription_cli"`, schema-valid,
  static/not live-synced → T1 (Clarified AC 1, 2, 3, 4).
- Subprocess-based launch/status/cancel/result, env isolated from metered-API vars from the start →
  T1 (Clarified AC 8, `_subprocess_env`).
- Never falls back to a metered API key when unauthenticated/undeterminable → T1 (`health()`'s
  `authenticated is False` branch has no fallback; Clarified AC 5).
- Unit tests using a fake/stubbed subprocess, no real Codex dependency in the standard suite → T1.
- Optional `skipif`-guarded, non-destructive smoke test → T1.
- Test confirming subprocess env strips identified metered-API vars even when set in the parent → T1
  (env-isolation test).
- Test confirming `health()` can report `AVAILABLE` via the identified safe probe, or documents why
  none exists → T1 (conditional test per investigation outcome).
- Full suite passes → T1 (final step).
- No credential ever logged/printed/included in evidence or error messages → T1 (`_redact` applied at
  every subprocess-sourced text boundary; Clarified AC 9).
- No adapter registration, no `executor_id` hardcoding, no schema/policy/doc vocabulary change, no
  retrofit onto `claude_cli.py`, no unmocked CLI invocation outside the smoke test → not touched by T1
  (Explicitly out of scope).

## Bootstrap

None needed — a single task with no dependencies.
