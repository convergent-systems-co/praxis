# Bundle remediation-2 — Implementation Plan

Source: `docs/develop/specs/remediation-2.md` (enhanced spec, authoritative).
Five findings from the Round 2 post-merge audit, spanning two unrelated
adapter files that happen to share nothing but an audit round:
`src/praxis_executors/adapters/ollama.py` (#68, #69, #70) and
`src/praxis_executors/adapters/claude_cli.py` (#71, #72). Each adapter's own
production file plus its own test file is the real footprint bottleneck —
within a file, tasks serialize (correct, not a defect); across the two
files, tasks run concurrently.

T1→T2 is a genuine dependency, not a footprint artifact: both edit the exact
same 8-line block in `ollama.py::_run_generate`'s success path, and the spec
gives the two fixes' composed final text (Task 2, clarified acceptance item
1) built on top of Task 1's cancellation check already being present. T3,
T4, and T5 have no real dependency on anything — they only serialize with
their file-siblings because `ollama.py`/`claude_cli.py` and their test files
are shared, exactly the router-file case: declare the shared file in both
footprints and let the scheduler serialize them.

## Issue #68 / #69 — Ollama `_run_generate` success-path (T1, T2)

### T1 — Fix cancel() race silently reporting SUCCEEDED (#68)

**Files:** `src/praxis_executors/adapters/ollama.py`, `tests/test_ollama_executor.py`

**Interfaces:** `OllamaExecutor._run_generate(self, handle_id: str, model: str, prompt: str) -> None` (body only; signature unchanged)

**Depends on:** (none)

**Steps:**
- [ ] In `_run_generate` (`ollama.py:247-276`), replace the success-path block currently at lines 269-276 (`self._results[handle_id] = ExecutionResult(status=ExecutorStatus.SUCCEEDED, ...)`) so the status is `ExecutorStatus.CANCELLED if handle_id in self._cancelled else ExecutorStatus.SUCCEEDED` — mirroring the exact pattern already used in the `except Exception` branch immediately above it (line 265: `ExecutorStatus.CANCELLED if handle_id in self._cancelled else ExecutorStatus.FAILED`).
- [ ] No lock needed around the `handle_id in self._cancelled` check: `self._cancelled` is only ever added to, never removed from, and a plain `in` test on a `set` is a single atomic bytecode-level op under the GIL — the same assumption the adjacent exception-branch check already relies on. Do not add a `with self._lock:` here.
- [ ] Add a new test in `tests/test_ollama_executor.py`, sibling of `test_cancel_closes_in_flight_request` (lines 302-343). Unlike that test, do **not** poll `executor._connections.get(handle.handle_id)` before calling `cancel()` — call `cancel()` immediately after `launch()` returns, before the worker thread is virtually guaranteed to have reached `urlopen()`. Use a real (not injected-delay) canned `/api/generate` response. Assert the terminal status via `_wait_for_terminal` (already defined at line 247 of the test file) is `CANCELLED`, not `SUCCEEDED`.
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q` and confirm no regression (network-dependent tests using `running_ollama_server` are expected to already be handled by the existing fixture).

---

### T2 — Fix malformed `/api/generate` payload permanently corrupting the handle (#69)

**Files:** `src/praxis_executors/adapters/ollama.py`, `tests/test_ollama_executor.py`

**Interfaces:** `OllamaExecutor._run_generate(self, handle_id: str, model: str, prompt: str) -> None` (body only; signature unchanged)

**Depends on:** T1 (edits the exact same lines T1 just changed; the two fixes compose into one block, not two independent edits)

**Steps:**
- [ ] Starting from T1's already-landed change, replace the (now cancellation-aware) success-path block at `ollama.py:269-276` with:
  ```python
  if handle_id in self._cancelled:
      self._results[handle_id] = ExecutionResult(
          status=ExecutorStatus.CANCELLED,
          payload={"response": None, "model": None, "done": None},
      )
      return
  if not isinstance(payload, dict):
      self._results[handle_id] = ExecutionResult(
          status=ExecutorStatus.FAILED,
          payload={"error": f"/api/generate returned a non-dict payload: {payload!r}"},
      )
      return
  self._results[handle_id] = ExecutionResult(
      status=ExecutorStatus.SUCCEEDED,
      payload={"response": payload.get("response"), "model": payload.get("model"), "done": payload.get("done")},
  )
  ```
- [ ] Cancellation is checked first, before payload-shape validation — a cancelled handle is always reported `CANCELLED` regardless of what the response body looked like (same precedence the `except Exception` branch at line 265 already establishes for the exception path). Do not reorder these two checks.
- [ ] Add a new test in `tests/test_ollama_executor.py`, mirroring `test_successful_generate_reaches_succeeded` (lines 283-299), but with the canned `/api/generate` response set to `(200, ["not", "a", "dict"])`. Assert the terminal status is `FAILED` (via `_wait_for_terminal`) and that `executor.result(handle)` does not raise — this is the exact regression #69 reports (a `KeyError` on `status()`/`result()` forever).
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q` and confirm all tests (including T1's new test) pass.

---

## Issue #70 — Ollama `capabilities()` unhandled exceptions (T3)

### T3 — Validate response shapes in `capabilities()` for all three malformed cases (#70)

**Files:** `src/praxis_executors/adapters/ollama.py`, `tests/test_ollama_executor.py`

**Interfaces:** `OllamaExecutor.capabilities(self) -> dict` (body only; signature unchanged)

**Depends on:** (none) — edits a different function (`capabilities()`, lines 163-214) than T1/T2 (`_run_generate`, lines 247-276); serializes with T1/T2 only because they share `ollama.py`/`tests/test_ollama_executor.py` as files, not because of a real ordering requirement.

**Steps:**
- [ ] **Gap 1** (`/api/show` non-dict JSON → uncaught `AttributeError` from `show.get("capabilities")`): widen the per-model `except` clause at line 186 from `except (_OllamaUnreachable, _OllamaHTTPError, KeyError, TypeError, ValueError):` to also catch `AttributeError`. Purely additive to the existing best-effort-degrade pattern already exercised by `test_capabilities_omits_context_window_when_show_response_is_malformed` (lines 198-209).
- [ ] **Gap 2** (`/api/tags` non-dict model entry → uncaught `TypeError` from `"name" not in model`): immediately before the existing `if "name" not in model:` check (line 173), add:
  ```python
  if not isinstance(model, dict):
      raise ExecutorError(f"ollama /api/tags returned a non-dict model entry: {model!r}")
  ```
  This is fail-fast (aborts the whole `capabilities()` call), matching the adjacent already-tested missing-`name` case (`test_capabilities_raises_executor_error_for_model_entry_missing_name`, lines 212-220) — not best-effort-skip like gaps 1/3, since `/api/tags` is this method's source of model *identity*, not supplementary metadata.
- [ ] **Gap 3** (`/api/show` non-list `capabilities` → uncaught `TypeError` in `_classify_kinds`): move the `_classify_kinds(model_name, show_capabilities)` call (currently line 197, outside the guarded try) inside the same try/except block that already wraps the `/api/show` call (lines 180-186), storing its result in a `kinds` variable used later when building `capability["satisfies"]` (replacing the current inline list comprehension at lines 195-198). On any exception in that block (now including a non-list `capabilities` triggering `_classify_kinds`'s internal `TypeError`), fall back to `kinds = _classify_kinds(model_name, None)` — the same fallback already used for a fully-absent `/api/show` response, and the only schema-consistent choice (`_classify_kinds` guarantees a non-empty result even for `None`; `schemas/v1/capability.schema.json`'s `satisfies` is `"minItems": 1`, so `kinds = []` would produce a schema-invalid advertisement).
- [ ] Add three new tests in `tests/test_ollama_executor.py`, one per gap: (a) `/api/show` returns a JSON array instead of an object — assert `capabilities()` still succeeds and the model's capability entry is present with `context_window` omitted; (b) `/api/tags`'s `models` list contains a non-dict entry (e.g. the bare string `"llama3"`) — assert `capabilities()` raises `ExecutorError`, mirroring the shape of the existing missing-`name` test; (c) `/api/show` returns `{"capabilities": "not-a-list"}` — assert `capabilities()` still succeeds and the model's capability entry's `satisfies` contains at least `{"kind": "reasoning", ...}`.
- [ ] Run `.venv/bin/python -m pytest tests/test_ollama_executor.py -q` and confirm all tests pass.

---

## Issue #71 — Claude auth detection stub (T4)

### T4 — Wire `_detect_authenticated` to `claude auth status --json` (#71)

**Files:** `src/praxis_executors/adapters/claude_cli.py`, `tests/test_claude_cli.py`

**Interfaces:** `ClaudeCliExecutor._detect_authenticated(self, cli_path: str) -> bool | None` (body only; signature unchanged)

**Depends on:** (none)

**Steps:**
- [ ] Replace `_detect_authenticated`'s unconditional `return None` (`claude_cli.py:82-85`) with a `subprocess.run([cli_path, "auth", "status", "--json"], capture_output=True, text=True, timeout=5)` call — `timeout=5` matches `_probe_version`'s existing call one method above (line 78), for consistency within this class.
- [ ] Parse `json.loads(result.stdout)["loggedIn"]` inside a `try`/`except` catching at least `subprocess.TimeoutExpired`, `OSError`, `json.JSONDecodeError`, `KeyError`, and `TypeError`; return `None` on any of them, matching `_probe_version`'s existing tolerance style (`except (OSError, subprocess.TimeoutExpired): pass`, line 79). Requires adding `import json` to this module's imports (not currently imported — confirm by reading the import block, lines 9-12, before editing).
- [ ] `health()` (lines 62-74) needs no change — it already branches correctly on `_detect_authenticated`'s three possible return values (`True` → `AVAILABLE`, `False` → `UNAVAILABLE`, `None` → `DEGRADED`). Do not touch it.
- [ ] Rewrite `test_detect_authenticated_unmocked_returns_none_by_default` (`tests/test_claude_cli.py:135-144`, currently patching `shutil.which` and `subprocess.run` with a bare, return-value-less `MagicMock()`). Post-fix this test still passes (a bare `MagicMock().stdout` fed to `json.loads` raises, degrading to `None`) but for a different reason — it now exercises the unparseable-output fallback path, not "no guess is wired in." Rename it (e.g. `test_detect_authenticated_returns_none_when_subprocess_run_output_is_unmocked_and_unparseable`) and rewrite its docstring so it accurately describes what it verifies post-fix. This repository has direct precedent for rewriting a test whose pinned rationale a later fix invalidates (`docs/develop/specs/remediation-1.md` Task 3, commit `861e3b8`).
- [ ] Add a new test mocking `subprocess.run` (patched at `praxis_executors.adapters.claude_cli.subprocess.run`, the boundary this file's existing tests already use) to return a fake completed-process whose `.stdout` is `'{"loggedIn": true}'`; assert `executor.health()` (not `_detect_authenticated` in isolation) reaches `ExecutorAvailability.AVAILABLE`. Add the mirrored `loggedIn: false` → `UNAVAILABLE` case for symmetry with the existing `test_health_unavailable_when_cli_present_and_not_authenticated` (lines 113-122), and at least one unparseable-output case (e.g. `.stdout = "not json"`) asserting `DEGRADED`.
- [ ] Do not broaden the set of commands or auth signals consulted (e.g. reading `~/.claude` config files, checking `ANTHROPIC_API_KEY` presence) — out of scope per the spec.
- [ ] Run `.venv/bin/python -m pytest tests/test_claude_cli.py -q` and confirm all tests (rewritten and new) pass.

---

## Issue #72 — Claude `launch()` environment isolation (T5)

### T5 — Strip Anthropic API env vars from the launched subprocess's environment (#72)

**Files:** `src/praxis_executors/adapters/claude_cli.py`, `tests/test_claude_cli.py`

**Interfaces:** `ClaudeCliExecutor.launch(self, request: ExecutionRequest) -> ExecutionHandle` (body only; signature unchanged)

**Depends on:** (none) — edits a different method (`launch()`, lines 87-106) than T4 (`_detect_authenticated`, lines 82-85); serializes with T4 only because they share `claude_cli.py`/`tests/test_claude_cli.py` as files, not because of a real ordering requirement.

**Steps:**
- [ ] Add `import os` to `claude_cli.py`'s import block (lines 9-12; not currently imported — confirm by reading the block before editing).
- [ ] In `launch()`, before the `subprocess.Popen(...)` call (lines 96-101), compute:
  ```python
  env = os.environ.copy()
  for key in ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"):
      env.pop(key, None)
  ```
  and pass `env=env` as an additional `Popen` kwarg. Do not strip any other environment variable (e.g. `ANTHROPIC_CUSTOM_HEADERS`, Bedrock/Vertex provider vars) — out of scope per the spec; the threat model is specifically ambient-API-key/base-url metered fallback.
- [ ] Add a new test using `monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-value-for-test")` in the parent test process, patching `subprocess.Popen` (the same boundary every other `launch()` test in this file already patches, e.g. `test_scripted_successful_run_reaches_succeeded_with_true_evidence`, lines 191-206). Assert on `call_args.kwargs["env"]` directly — that the `env` kwarg was passed at all, and that the key is absent from it (or the dict as a whole differs from `os.environ` verbatim) — not merely that a search for the fake value's literal presence turned up empty in something that might not have been checked.
- [ ] Run `.venv/bin/python -m pytest tests/test_claude_cli.py -q` and confirm all tests (including T4's) pass.

---

## Final integration check (not a task — the tech lead's bundle-level verification)

Once all 5 tasks have landed: `.venv/bin/python -m pytest` (full suite) must pass, with no regression to any currently-passing test. No task above should be marked done until its own listed test command passes; this line is the bundle-wide acceptance criterion, not an additional task.

Delivery (PR against `main` referencing #68, #69, #70, #71, #72 with closing keywords; merge policy `never`) is the tech lead's responsibility after all tasks land, not a planned task.
