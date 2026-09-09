# Plan: b1-issue40

Source: `docs/develop/specs/b1-issue40.md` (enhanced spec — plan against its Clarified acceptance
criteria and Assumptions, not the raw issue text embedded at its top).

## Why this is a single task

The entire deliverable is one new `Executor` subclass in one new file plus its test file. `Executor`
is an ABC with six abstract methods (`capabilities`, `health`, `launch`, `status`, `cancel`,
`result`); a class implementing only a subset is not instantiable, so there is no way to split
"discovery/capabilities/health" from "launch/status/cancel/result" into two independently-testable,
committable slices without one of them shipping stub methods that the other later replaces — exactly
the kind of narrative-driven split (and half-finished implementation) the planning rules warn
against. Both halves would also declare the identical two files, so splitting buys no real DAG
concurrency (they would serialize on footprint overlap regardless of how the dependency edge is
drawn). One task, `T1`, covers every acceptance criterion.

## Design decisions pinned by this plan

The enhanced spec leaves several implementation shapes to the planner (Clarified AC 6, 8). Pinned
here so the implementer spends zero time on design and starts writing code and tests immediately.

1. **New module:** `src/praxis_executors/adapters/claude_cli.py`, class `ClaudeCliExecutor(Executor)`.
   - `__init__(self, executor_id: str) -> None` — no other constructor parameters (mirrors
     `SubprocessExecutor`/`FakeCapabilityExecutor`; `executor_id` is caller-supplied, per Explicitly
     out of scope / Assumption 6). Internal state mirrors `SubprocessExecutor`:
     `self._processes: dict[str, subprocess.Popen]`, `self._results: dict[str, ExecutionResult]`,
     `self._cancelled: set[str]`.
   - Module constant `_SPEC_VERSION = "1.0.0"` and `_CLI_NAME = "claude"`.

2. **Credential redaction helper**, module-level in the same file (not a separate module — nothing
   else needs it, so a shared module would be premature):
   ```python
   _CREDENTIAL_PATTERN = re.compile(r"sk-ant-[A-Za-z0-9_-]{10,}")
   _REDACTED = "***REDACTED***"

   def _redact(text: str) -> str:
       return _CREDENTIAL_PATTERN.sub(_REDACTED, text)
   ```
   Apply `_redact(...)` to every piece of subprocess-sourced text (`stdout`, `stderr`, and the
   string form of any caught `OSError`) before it goes into `ExecutionResult.evidence`,
   `ExecutionResult.payload`, or an `ExecutorError` message. This regex matches the Anthropic
   API-key shape (`sk-ant-...`); the credential-safety test (Clarified AC 7) must use a fake string
   of exactly this shape (e.g. `"sk-ant-api03-FAKESECRETFAKESECRETFAKE"`) so the test and the
   implementation agree on what "credential-shaped" means — there is no repo precedent to verify
   this shape against, so pinning it here avoids the implementer inventing one mid-task.

3. **`capabilities()`** — one `Capability` entry, all four kinds, `auth_transport` on that same
   entry (Clarified AC 1, 3):
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
                       {"kind": "reasoning"},
                       {"kind": "tools"},
                       {"kind": "filesystem"},
                   ],
                   "auth_transport": "subscription_cli",
               }
           ],
       }
   ```
   This is static regardless of live auth state (Clarified AC 4) — `capabilities()` never calls
   `shutil.which` or any subprocess.

4. **`health()`** — the only place live detection happens:
   ```python
   def health(self) -> ExecutorAvailability:
       cli_path = shutil.which(_CLI_NAME)
       if cli_path is None:
           return ExecutorAvailability.UNAVAILABLE
       self._probe_version(cli_path)  # best-effort discovery, Clarified AC 6; ignore failures
       authenticated = self._detect_authenticated(cli_path)
       if authenticated is False:
           return ExecutorAvailability.UNAVAILABLE
       if authenticated is True:
           return ExecutorAvailability.AVAILABLE
       return ExecutorAvailability.DEGRADED  # unknown, Clarified AC 5 / Assumption 4
   ```
   - `_probe_version(self, cli_path: str) -> None` runs `subprocess.run([cli_path, "--version"],
     capture_output=True, text=True, timeout=5)` inside a `try/except (OSError,
     subprocess.TimeoutExpired)`, discards the result on any failure, and does not affect the
     return value either way — it exists purely so a mocked-subprocess test can assert the CLI's
     version-discovery call happens (Clarified AC 6 / original spec's "version (`claude
     --version`)"). Do not parse or store the version string anywhere schema-visible; this is
     unshaped, best-effort metadata per Clarified AC 6 / Assumption 5.
   - `_detect_authenticated(self, cli_path: str) -> bool | None` — **no source in this repository
     names a specific safe, side-effect-free `claude` subcommand that reports auth state.** Per the
     resolve-or-name test (same reasoning as Assumption 5), do not invent one. Implement it to
     always `return None` (unknown), with a one-line comment stating that no verified safe probe
     exists. **Exception:** if `shutil.which("claude")` succeeds in the implementer's own sandbox
     (i.e. the real CLI happens to be installed there), the implementer may run `claude --help`
     (non-destructive, read-only) to check for a genuinely safe, non-mutating auth-status flag; if
     one is found, wire it in and cite the exact command in a code comment. If the CLI isn't
     available to check, or no such flag exists, leave `_detect_authenticated` returning `None`
     unconditionally — never guess (original spec: "If safe detection isn't possible, report
     `unknown`, not a guess").
   - This satisfies "never fall back to a metered API key if the CLI reports unauthenticated": there
     is no fallback branch anywhere in `health()` or elsewhere in this file — add a one-line comment
     next to the `authenticated is False` branch noting the absence is deliberate (mirrors the
     env-var non-bypass test pattern used elsewhere in this codebase for negative claims).

5. **`launch`/`status`/`cancel`/`result`** — same `Popen`-based lifecycle as `SubprocessExecutor`
   (`_process_for`, `_terminal_status`, `uuid.uuid4().hex` handle ids, `ExecutorError` on an unknown
   handle), with the request-to-argv mapping pinned here (Clarified AC 8 leaves this to the planner;
   no existing adapter needs a different mapping yet):
   - `request.parameters` must contain key `"prompt"` (a string); its absence raises
     `ExecutorError("request.parameters is missing required key 'prompt'")` (mirrors
     `SubprocessExecutor`'s `"command"` check).
   - `request.parameters.get("extra_args", [])` (a `list[str]`, default empty) is appended verbatim
     after the prompt — an escape hatch for anything this bundle doesn't need to name explicitly.
   - `launch()` resolves `cli_path = shutil.which(_CLI_NAME)`; if `None`, raise
     `ExecutorError("claude CLI is not available on PATH")` without attempting `Popen`.
   - argv is `[cli_path, "-p", request.parameters["prompt"], *extra_args]`. The `-p`/print-mode flag
     for non-interactive one-shot output is the original spec's own framing (subprocess-based
     execution converting CLI output into a result) and has no repo precedent to contradict it;
     if the implementer's sandbox has the real CLI installed, verify `-p` against `claude --help`
     and cite the finding in a comment — otherwise keep `-p` as the defensible default and cite that
     no local CLI was available to verify against.
   - `Popen(argv, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)` inside a
     `try/except OSError as exc: raise ExecutorError(f"failed to launch claude CLI: {_redact(str(exc))}") from exc`.
   - `status()`/`cancel()`/`_process_for()`/`_terminal_status()` are a direct copy of
     `SubprocessExecutor`'s versions (same file, same private-method names, adjusted to this class).
   - `result()` mirrors `SubprocessExecutor.result()` but redacts before storing:
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

### T1 — `ClaudeCliExecutor` adapter and its tests (issue #40)

**Files:** `src/praxis_executors/adapters/claude_cli.py` (new), `tests/test_claude_cli.py` (new)

**Depends on:** none

**Interfaces:** `ClaudeCliExecutor(Executor)` per "Design decisions pinned by this plan" above —
implement exactly those signatures; do not improvise different names.

**Steps:**
- [ ] Re-read `src/praxis_executors/interface.py`, `src/praxis_executors/adapters/subprocess_executor.py`,
      and `src/praxis_contracts/schemas/v1/capability.schema.json` (already read during planning; the
      pinned design above is grounded in them) to confirm nothing has drifted since planning.
- [ ] Create `src/praxis_executors/adapters/claude_cli.py`: module docstring (follow
      `subprocess_executor.py`'s style — one line naming the schema files this dict shape follows),
      imports (`shutil`, `subprocess`, `uuid`, `re`, plus the same `praxis_executors.interface`
      imports `subprocess_executor.py` uses), the `_SPEC_VERSION`/`_CLI_NAME`/`_CREDENTIAL_PATTERN`/
      `_REDACTED`/`_redact` module-level pieces, and the `ClaudeCliExecutor` class implementing all
      six `Executor` methods exactly as pinned above.
- [ ] Create `tests/test_claude_cli.py` (follow `tests/test_subprocess_executor.py` and
      `tests/test_fake_capability_executor.py` for import/fixture style — a module-level
      `_executor()` helper constructing `ClaudeCliExecutor(executor_id="executor-claude-cli-1")`).
      Mock at the boundary: `unittest.mock.patch("praxis_executors.adapters.claude_cli.shutil.which", ...)`
      and `unittest.mock.patch("praxis_executors.adapters.claude_cli.subprocess.Popen", ...)` /
      `"...subprocess.run"` — never invoke a real `claude` process in these tests. Cover, at minimum:
      - [ ] `capabilities()` validates against `capability-advertisement.schema.json` via
            `praxis_contracts.validator.validate_document` + `praxis_contracts.schema_paths.SCHEMA_DIR`
            (mirror `test_subprocess_executor.py::test_capabilities_advertisement_validates_against_schema`).
      - [ ] `capabilities()`'s single capability entry has `satisfies` covering exactly
            `{"coding", "reasoning", "tools", "filesystem"}` and `auth_transport == "subscription_cli"`
            on that same entry (Clarified AC 1, 3).
      - [ ] Calling `capabilities()` does not call the patched `shutil.which`/`subprocess.Popen`/`run`
            (proves Clarified AC 4 — static, not live-synced).
      - [ ] `health()` returns `UNAVAILABLE` when patched `shutil.which` returns `None` (CLI absent).
      - [ ] `health()` returns `AVAILABLE` when the CLI is present and `_detect_authenticated` is
            monkeypatched/mocked to return `True`.
      - [ ] `health()` returns `UNAVAILABLE` when the CLI is present and `_detect_authenticated` is
            mocked to return `False` — add a comment noting this is the "never fall back" guarantee:
            there is no other branch this could take.
      - [ ] `health()` returns `DEGRADED` when `_detect_authenticated` is mocked to return `None`
            (Clarified AC 5), and — separately, without mocking `_detect_authenticated` — that the
            real, unmocked `_detect_authenticated` returns `None` given only the pinned design (proves
            the "no guess" default is actually wired in, not just mockable).
      - [ ] `health()` invokes `claude --version` via the patched `subprocess.run` (mirrors the
            original spec's named discovery call; assert the mock was called with an argv ending in
            `"--version"`).
      - [ ] `launch()` without a `"prompt"` key in `request.parameters` raises `ExecutorError`.
      - [ ] `launch()` when patched `shutil.which` returns `None` raises `ExecutorError` without
            calling the patched `Popen`.
      - [ ] A scripted successful run (mocked `Popen` whose `.poll()` then `.communicate()` simulate a
            zero exit) reaches `ExecutorStatus.SUCCEEDED` with `evidence == {"process-exit-status": True}`.
      - [ ] A scripted nonzero-exit run reaches `ExecutorStatus.FAILED` with
            `evidence == {"process-exit-status": False}`.
      - [ ] `cancel()` on a still-running mocked process calls `.terminate()` and drives the handle to
            `ExecutorStatus.CANCELLED` (mirror `test_subprocess_executor.py`'s cancel test, adapted to a
            mock instead of a real sleeping subprocess).
      - [ ] `status()`, `result()`, and `cancel()` each raise `ExecutorError` for an unknown handle.
      - [ ] **Credential-safety (Clarified AC 7):** mock `Popen.communicate()` to return
            `(f"...{FAKE_SECRET}...", f"...{FAKE_SECRET}...")` where
            `FAKE_SECRET = "sk-ant-api03-FAKESECRETFAKESECRETFAKE"`; assert `FAKE_SECRET` does not
            appear verbatim anywhere in `result(handle).evidence`, in `result(handle).payload`
            (neither `payload["stdout"]` nor `payload["stderr"]`), and separately that mocking `Popen`
            construction to raise `OSError(f"launch failed: {FAKE_SECRET}")` produces an
            `ExecutorError` whose `str(...)` does not contain `FAKE_SECRET` either.
      - [ ] Optional real-CLI smoke test, `pytest.mark.skipif(shutil.which("claude") is None,
            reason="claude CLI not installed")`, doing one non-destructive real call (e.g.
            `ClaudeCliExecutor(executor_id="executor-claude-cli-smoke").health()` and asserting it
            returns one of the three `ExecutorAvailability` members, or a direct `claude --version`
            subprocess call) — must be skipped, never failed, when the CLI is absent.
- [ ] Run `.venv/bin/python -m pytest tests/test_claude_cli.py -q`.
- [ ] Run the full suite: `.venv/bin/python -m pytest -q` (Acceptance bullet 3 — nothing else in the
      repo should be touched by this file addition, so this should be a clean pass).

---

## Coverage check against acceptance criteria

- Schema-validated `capabilities()` with all four kinds + `auth_transport` → T1 (Clarified AC 1, 3).
- Static `auth_transport`, not live-synced → T1 (Clarified AC 4).
- Unknown auth → `DEGRADED`; confirmed-unauthenticated → `UNAVAILABLE`; never falls back → T1
  (Clarified AC 5, original spec's fallback prohibition).
- Best-effort, unshaped discovery metadata (`--version`) exercised by a mocked-subprocess test → T1
  (Clarified AC 6).
- Credential-shaped string never appears in evidence/payload/error → T1 (Clarified AC 7).
- `Popen`-based lifecycle, `uuid.uuid4().hex` handles, `ExecutorError` on unknown handle → T1
  (Clarified AC 8).
- No real `claude` CLI dependency in the standard suite; optional `skipif`-guarded smoke test; full
  suite passes → T1 (original spec's Acceptance bullets 1-3).
- No adapter registration, no `executor_id` hardcoding, no schema/policy/doc vocabulary change → not
  touched by T1 (Explicitly out of scope).

## Bootstrap

None needed — a single task with no dependencies.
