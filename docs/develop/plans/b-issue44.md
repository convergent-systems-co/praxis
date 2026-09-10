# Plan — b-issue44: MLX / local Hugging Face executor adapter

Source spec: `docs/develop/specs/b-issue44.md` (enhanced). Base: `origin/main` @ 9366f5b.
Branch: `develop/b-issue44`.

## Shape of the work

One new adapter module (`src/praxis_executors/adapters/mlx.py`), one new test module
(`tests/test_mlx_executor.py`), one doc edit (`docs/executors.md`). No dependency change, no
change to any shipped module, no CLI wiring, no schema change.

Because the spec pins both the adapter and its tests to a single file each, the three method-group
tasks (T2/T3/T4) necessarily share a footprint and serialize behind the bootstrap. That
serialization is real, not accidental: they edit the same two files. The one genuinely independent
branch is the documentation update (T5), which touches only `docs/executors.md` and runs from t=0
alongside everything else.

T1 is deliberately minimal — imports, constructor, the two private exception types, the loopback
helper, the HTTP-JSON helpers, and the shared test fixture — so T2/T3/T4 can start as soon as it
lands.

### Acceptance criteria → tasks

| Criterion | Task(s) |
| --- | --- |
| 1 — HTTP against `mlx_lm.server`, stdlib only, no MLX import | T1, enforced by test in T1 |
| 2 — new files, stdlib only, no new dependency | T1 |
| 3 — `executor_id` is a constructor parameter | T1 |
| 4 — `base_url` configurable, loopback-restricted, no probing in `__init__` | T1 |
| 5 — two timeouts | T1 (constructor), T2/T4 (use sites) |
| 6 — three-state `health()` | T2 |
| 7 — advertisement shape | T3 |
| 8 — execution lifecycle | T4 |
| 9 — the four regression classes | T4 (9.1, 9.2), T3 (9.3), T5+T6 (9.4), T1 (no-arch-branch test) |
| 10 — fake-server tests | T1 (fixture), T2/T3/T4 (cases) |
| 11 — `docs/executors.md` updated + pinned | T5 (doc), T6 (test) |
| 12 — whole suite green | T7 |

---

## T1 — Bootstrap: module scaffold, constructor, HTTP helpers, test fixture

**Depends on:** *(none)*

**Files**
- `src/praxis_executors/adapters/mlx.py` (new)
- `tests/test_mlx_executor.py` (new)

**Interfaces**

```python
_SPEC_VERSION = "1.0.0"

class _MlxUnreachable(Exception): ...
class _MlxHTTPError(Exception): ...

def _is_loopback_host(host: str | None) -> bool: ...
def _http_get_json(base_url: str, path: str, timeout: float) -> object: ...
def _http_post_json(
    base_url: str,
    path: str,
    payload: dict,
    timeout: float,
    on_connection: "collections.abc.Callable[[object], None] | None" = None,
) -> object: ...

class MlxExecutor(Executor):
    def __init__(
        self,
        executor_id: str,
        base_url: str = "http://127.0.0.1:8080",
        timeout: float = 5.0,
        generate_timeout: float = 120.0,
    ) -> None: ...
```

**Steps**
- [ ] Verify `mlx_lm.server`'s documented default host/port against the `mlx-lm` project's own
      current CLI documentation before writing the default down, and cite the source in a comment
      on the `base_url` default. The spec assumes `http://127.0.0.1:8080`; correct it here if the
      documented default differs (it is a one-line change with no fan-out).
- [ ] Create `src/praxis_executors/adapters/mlx.py` with a module docstring stating: local HTTP
      adapter against an already-running `mlx_lm.server`; Apple Silicon is advertised metadata
      only, never a runtime gate; `base_url` is loopback-only; the adapter never imports `mlx`,
      `mlx_lm`, or `transformers` and never starts or supervises a server process.
- [ ] Imports: `from __future__ import annotations`, then stdlib only — `ipaddress`, `json`,
      `threading`, `urllib.error`, `urllib.parse`, `urllib.request`, `uuid` — plus the
      `praxis_executors.interface` names (`Executor`, `ExecutionHandle`, `ExecutionRequest`,
      `ExecutionResult`, `ExecutorAvailability`, `ExecutorError`, `ExecutorStatus`). Nothing else.
- [ ] Define `_MlxUnreachable` and `_MlxHTTPError` with docstrings recording *why* they are
      distinct: `urllib.error.HTTPError` subclasses `urllib.error.URLError`, so it must be caught
      first or "reachable but erroring" collapses into "down" and `health()` loses `DEGRADED`.
- [ ] Write `_is_loopback_host` locally (do not import from `ollama.py`, do not extract a shared
      module — see spec assumption 8): `None` → `False`; `"localhost"` → `True`; otherwise
      `ipaddress.ip_address(host).is_loopback`, with `ValueError` → `False`.
- [ ] Write `_http_get_json` and `_http_post_json` using `urllib.parse.urljoin` +
      `urllib.request.urlopen`. Catch `urllib.error.HTTPError` → `_MlxHTTPError` **before**
      `urllib.error.URLError` → `_MlxUnreachable`; also map `TimeoutError`/`socket.timeout` and
      `OSError` to `_MlxUnreachable`. Decode with `json.loads(...)` and let the caller decide what
      shapes are acceptable — return whatever JSON value came back (may be a list), do not assume
      a dict here.
- [ ] Give `_http_post_json` an `on_connection` callback invoked with the live response object
      before the body is read, so T4 can register the connection for `cancel()` to close.
- [ ] `MlxExecutor.__init__`: take `executor_id` as a required parameter (never defaulted, never
      containing `"mlx"`, `"lm"`, a repo id, or a model name — `docs/ontology.md:20-21`); split
      `base_url` with `urllib.parse.urlsplit(...).hostname` and raise `ValueError` naming the
      rejected `base_url` and host when it is not loopback; store `_executor_id`, `_base_url`,
      `_timeout`, `_generate_timeout`; initialise `_lock = threading.Lock()`, `_threads: dict`,
      `_results: dict`, `_cancelled: set`, `_connections: dict`. No HTTP call, no `shutil.which`,
      no filesystem access, no `platform`/`sys.platform`/`uname` check anywhere in the module.
- [ ] Add the six ABC methods as bodies that raise `ExecutorError("not implemented")` so the class
      is instantiable and importable; T2/T3/T4 replace them. (`fake.py` shows the ABC surface if a
      signature is unclear.)
- [ ] Create `tests/test_mlx_executor.py` modelled on `tests/test_ollama_executor.py:1-60`: a
      `ThreadingHTTPServer` on `port=0`, a `BaseHTTPRequestHandler` reading from a mutable
      `responses` dict keyed by path to `(status, body)` where a `bytes` body is written verbatim
      (so malformed/non-JSON bodies can be simulated) and a non-`bytes` body is JSON-encoded; a
      per-path `delays` dict that flushes headers, sleeps, then writes the body; a `.serve_forever()`
      background thread; `.shutdown()` + `.server_close()` teardown; and a fixture yielding
      `(server, base_url)` built from the OS-assigned port.
- [ ] Add T1's own tests: `MlxExecutor("e1", base_url=<fixture url>)` constructs; a non-loopback
      `base_url` (e.g. `http://10.0.0.5:8080`) raises `ValueError`; `localhost`, `127.0.0.2`, and
      `http://[::1]:8080` are all accepted; construction against a URL whose port has no listener
      does not raise and issues no request.
- [ ] Add a source-level guard test: read `src/praxis_executors/adapters/mlx.py` and assert it
      contains no `mlx_lm`/`transformers`/`import mlx`/`platform.machine`/`sys.platform`/`uname`
      occurrence, and that its only non-stdlib import is `praxis_executors.interface`.
- [ ] Confirm no `pyproject.toml` edit is needed and make none.
- [ ] `python3 -m pytest tests/test_mlx_executor.py` passes.

---

## T2 — `health()`: three-state availability over `GET /v1/models`

**Depends on:** T1

**Files**
- `src/praxis_executors/adapters/mlx.py`
- `tests/test_mlx_executor.py`

**Interfaces**

```python
class MlxExecutor(Executor):
    def health(self) -> ExecutorAvailability: ...
```

**Steps**
- [ ] Implement `health()` calling `_http_get_json(self._base_url, "/v1/models", self._timeout)`.
- [ ] `_MlxUnreachable` → **return** `ExecutorAvailability.UNAVAILABLE`. Never raise: a discovery
      pass over several adapters must not fail because one local server is down.
- [ ] `_MlxHTTPError` → return `ExecutorAvailability.DEGRADED`, with a comment that the process is
      up but erroring.
- [ ] Any decode/shape problem (`ValueError` — note `json.JSONDecodeError` and `UnicodeDecodeError`
      are both `ValueError` subclasses — plus `TypeError`, `KeyError`, `AttributeError`) → return
      `DEGRADED`, not an exception.
- [ ] A well-formed body whose `data` is empty or absent → `DEGRADED`, with a comment citing
      `capability-advertisement.schema.json`'s `"minItems": 1` on `capabilities`: a zero-model
      server has nothing legal to advertise, so it cannot be `AVAILABLE`.
- [ ] At least one entry under `data` → `AVAILABLE`.
- [ ] Tests: no listener → `UNAVAILABLE`; canned 500 on `/v1/models` → `DEGRADED`; canned
      `{"object": "list", "data": []}` → `DEGRADED`; non-JSON body → `DEGRADED`;
      `{"object": "list", "data": [{"id": "mlx-community/X"}]}` → `AVAILABLE`. Assert explicitly
      that the unreachable case returns rather than raises.
- [ ] `python3 -m pytest tests/test_mlx_executor.py` passes.

---

## T3 — `capabilities()`: per-model advertisement + shape-error containment

**Depends on:** T1

**Files**
- `src/praxis_executors/adapters/mlx.py`
- `tests/test_mlx_executor.py`

**Interfaces**

```python
def _classify_kinds(model_id: str) -> list[str]: ...

class MlxExecutor(Executor):
    def capabilities(self) -> dict: ...
```

**Steps**
- [ ] Implement `_classify_kinds`: always include `"reasoning"`; add `"coding"` when the lowercased
      model id contains `code` or `coder`. Comment why the unconditional baseline is load-bearing:
      `capability.schema.json` sets `"minItems": 1` on `satisfies`, so a model classified into zero
      kinds would produce a schema-invalid advertisement. Draw kinds only from the documented
      vocabulary at `docs/executors.md:56-71` — never a model name or repo id (`docs/ontology.md:13-16`).
- [ ] Implement `capabilities()`: `GET /v1/models` via `_http_get_json` with `self._timeout`;
      `_MlxUnreachable` → `raise ExecutorError("... unreachable: ...")`; `_MlxHTTPError` →
      `raise ExecutorError("... returned an error: ...")`. Both `from exc`.
- [ ] Contain every shape defect as `ExecutorError`, never a bare `KeyError`/`TypeError`/
      `AttributeError` (regression class #70, spec criterion 9.3): body not a dict; `data` missing
      or not a list; an entry that is not a dict; a missing `id`; an `id` that is not a `str`.
      Guard with `isinstance` checks rather than a bare `try`, and wrap the decode itself so
      `ValueError` from malformed JSON also becomes `ExecutorError`. Cite
      `src/praxis_cli/fields.py:26-48` in a comment for why leaking a non-`ExecutorError` reads
      downstream as an adapter defect rather than a service outage.
- [ ] Emit one capability per model: `{"spec_version": _SPEC_VERSION, "satisfies": [{"kind": k,
      "parameters": {"model": <model id>}} for k in kinds], "auth_transport": "local",
      "platform": "macos"}`. Omit `context_window` entirely — `/v1/models` carries no
      context-length metadata and there is no documented per-model endpoint to fetch it from.
- [ ] Return `{"spec_version": _SPEC_VERSION, "executor_id": self._executor_id,
      "capabilities": [...]}`.
- [ ] Zero models on a reachable server → `raise ExecutorError`, matching `minItems: 1`.
- [ ] Tests: happy-path advertisement shape (`auth_transport == "local"`, `platform == "macos"`,
      `parameters.model` carries the full repo id, `executor_id` is the constructor value and
      contains no vendor/model string, no `context_window` key); a `coder` model additionally
      advertises `coding` while a plain model advertises `reasoning` only as its baseline (assert
      `"reasoning" in kinds`, do not pin an exhaustive kind set for a specific model id);
      unreachable → `ExecutorError`; non-2xx → `ExecutorError`; zero models → `ExecutorError`.
- [ ] Tests for 9.3, each asserting `ExecutorError` (use `pytest.raises(ExecutorError)`, which
      would not catch a stray `TypeError`): body is a JSON list; `data` is a string; an entry is a
      string; an entry has no `id`; `id` is an int; the body is non-JSON bytes.
- [ ] Add `test_capabilities_advertisement_validates_against_schema` in the same shape as the one
      in `tests/test_ollama_executor.py`: validate the returned dict with
      `praxis_contracts.validator.validate_document` against
      `schemas/v1/capability-advertisement.schema.json`.
- [ ] `python3 -m pytest tests/test_mlx_executor.py` passes.

---

## T4 — Execution lifecycle: `launch`/`status`/`cancel`/`result`

**Depends on:** T1

**Files**
- `src/praxis_executors/adapters/mlx.py`
- `tests/test_mlx_executor.py`

**Interfaces**

```python
class MlxExecutor(Executor):
    def launch(self, request: ExecutionRequest) -> ExecutionHandle: ...
    def status(self, handle: ExecutionHandle) -> ExecutorStatus: ...
    def cancel(self, handle: ExecutionHandle) -> None: ...
    def result(self, handle: ExecutionHandle) -> ExecutionResult: ...
    def _run_generation(self, handle_id: str, model: str, prompt: str) -> None: ...
```

**Steps**
- [ ] `launch()`: raise `ExecutorError` when `request.parameters` is missing `model` or `prompt`
      (each named individually in its message, as `ollama.py:236-241` does) and when either value
      is not a `str`. Mint a `handle_id` with `uuid.uuid4()`, register the thread under the lock,
      start a `threading.Thread(..., daemon=True)`, and return the `ExecutionHandle` immediately —
      no blocking work on the caller's thread.
- [ ] `_run_generation()`: `POST /v1/chat/completions` with body
      `{"model": model, "messages": [{"role": "user", "content": prompt}], "stream": false}` and
      `self._generate_timeout`. Register the live connection through `on_connection` so `cancel()`
      can close it.
- [ ] **Every** exit from the worker records exactly one `ExecutionResult` for its handle
      (regression class #69, spec criterion 9.2). Structure the body so the record-and-store step
      is unconditional — a `try/except/finally` or a single terminal write at the end — and confirm
      by inspection that no `return` or raised exception can leave `_results` without an entry.
      A missing entry makes `status()`/`result()` raise `KeyError` forever.
- [ ] Parse the response defensively with `.get()` at every level: body must be a dict, `choices` a
      non-empty list, `choices[0]` a dict, `message` a dict, `content` a str. Any deviation →
      `ExecutionResult(status=ExecutorStatus.FAILED, ...)` with a diagnostic payload, never an
      exception escaping the thread.
- [ ] Success payload carries the assistant message text, the model the server reports (falling
      back to the requested model), and `finish_reason` read defensively.
- [ ] **Cancel race (regression class #68, spec criterion 9.1):** before writing `SUCCEEDED`,
      re-check membership in `self._cancelled` under the lock and write
      `ExecutorStatus.CANCELLED` instead. This check belongs on the *success* path, not only on the
      exception path — a `cancel()` that lands after the response fully arrived must still surface
      as `CANCELLED`.
- [ ] `cancel()`: raise `ExecutorError` for an unknown `handle_id`; otherwise add to `_cancelled`
      and best-effort `.close()` every registered connection for that handle, swallowing any
      exception from the close. Do not attempt to make the server stop generating.
- [ ] `status()`: `ExecutorError` for an unknown handle; `RUNNING` while the thread is alive;
      otherwise the recorded terminal status. `result()`: `ExecutorError` for an unknown handle and
      `ExecutorError` when the execution is still running; otherwise the recorded
      `ExecutionResult`.
- [ ] Tests: success path end-to-end with a canned chat-completions body (join the worker via a
      poll-until-terminal loop with a bounded deadline, as `tests/test_ollama_executor.py` does);
      missing `model` → `ExecutorError`; missing `prompt` → `ExecutorError`; non-str `model` →
      `ExecutorError`; `status`/`cancel`/`result` on a fabricated unknown handle → `ExecutorError`;
      `result()` while running → `ExecutorError`.
- [ ] Test 9.1: use the per-path delay so headers go out first, call `cancel()` immediately after
      `launch()` returns, and assert the terminal status is `CANCELLED` and never `SUCCEEDED`.
- [ ] Test 9.2: canned `/v1/chat/completions` body that is a JSON array, and a second that is
      non-JSON bytes; assert terminal status is `FAILED`, that `result()` returns without raising,
      and that `status()` does not raise `KeyError`.
- [ ] Test that a 500 from `/v1/chat/completions` and an unreachable server each resolve the handle
      to `FAILED` rather than leaving it unresolved.
- [ ] `python3 -m pytest tests/test_mlx_executor.py` passes.

---

## T5 — `docs/executors.md`: retire MLX as hypothetical, list the concrete adapter

**Depends on:** *(none — runs in parallel with the whole code branch)*

**Files**
- `docs/executors.md`

**Steps**
- [ ] In the section intro (`docs/executors.md:241-242`), drop MLX from the "e.g. a future
      Copilot, OpenCode, or MLX/local adapter" list — it is no longer future.
- [ ] At `docs/executors.md:257`, change the hypothetical-adapters sentence to name only Copilot
      and OpenCode. The existing regression test
      (`tests/test_repair_findings_b1_issue41.py:69-90`) asserts no `hypothetical adapters (...)`
      list mentions a shipped adapter; leaving MLX there would fail it once T6's MLX analogue
      lands, and it is factually wrong regardless.
- [ ] Update the count "Five concrete adapters ship today" → "Six", and append `MlxExecutor`
      (`src/praxis_executors/adapters/mlx.py`) to the inventory with a one-clause description: a
      local-HTTP-backed executor against an already-running local `mlx_lm.server`
      (`auth_transport: "local"`).
- [ ] Update the closing sentence from "None of the five is registered with an `ExecutorRegistry`
      by default" to "None of the six ...". Keep the claim itself intact — this bundle registers
      nothing.
- [ ] Change nothing else in the file; in particular do not touch the capability-kind vocabulary at
      `docs/executors.md:56-71` or the matching description at `104-120`.
- [ ] `python3 -m pytest tests/test_repair_findings_b1_issue41.py` still passes.

---

## T6 — Pin the doc claims from `tests/test_mlx_executor.py`

**Depends on:** T1, T5

**Files**
- `tests/test_mlx_executor.py`

**Steps**
- [ ] Following the shape of `tests/test_repair_findings_b1_issue41.py:57-90`, add a test that
      reads `docs/executors.md` and asserts MLX no longer appears in any `hypothetical adapters
      (...)` list.
- [ ] Add a test asserting both `MlxExecutor` and `src/praxis_executors/adapters/mlx.py` appear in
      the concrete-adapters inventory.
- [ ] Do **not** pin the adapter count, the list ordering, or exact surrounding prose. Repeat the
      reason in a comment, citing `tests/test_repair_findings_b1_issue41.py:78-81`: pinning the
      running count makes the next adapter's bundle fail for an unrelated reason.
- [ ] Resolve the doc path relative to the test file (`pathlib.Path(__file__).resolve().parents[1]`)
      so the test does not depend on the pytest invocation directory.
- [ ] `python3 -m pytest tests/test_mlx_executor.py` passes.

---

## T7 — Integration pass: whole suite green, no shipped file changed

**Depends on:** T2, T3, T4, T6

**Files**
- `src/praxis_executors/adapters/mlx.py`
- `tests/test_mlx_executor.py`
- `docs/executors.md`

**Steps**
- [ ] Remove any remaining `NotImplementedError`/placeholder body left over from T1's scaffold and
      confirm all six ABC methods are real.
- [ ] Re-read the finished `mlx.py` for the four regression classes as a single pass: cancelled-set
      check on the success path; every worker exit resolves its handle; `capabilities()` raises only
      `ExecutorError`; docs updated.
- [ ] Confirm no code path branches on host architecture or OS, and that `mlx`, `mlx_lm`, and
      `transformers` appear nowhere outside prose.
- [ ] Run `python3 -m pytest` from the worktree root and confirm the whole suite passes.
- [ ] Run `git diff --stat origin/main` and confirm the changed set is exactly
      `src/praxis_executors/adapters/mlx.py`, `tests/test_mlx_executor.py`, `docs/executors.md`,
      plus this bundle's spec/plan documents. No `pyproject.toml`, no `src/praxis_cli/**`, no
      `schemas/**`, no `src/praxis_executors/{interface,matching,policy,registry}.py`, and no
      pre-existing test modified.
- [ ] Optionally add a single real-`mlx_lm.server` smoke test that `pytest.skip`s when nothing
      answers on the default `base_url`, mirroring the one skipped test in
      `tests/test_ollama_executor.py`. It must skip, never fail, when no server is reachable.

---

## Dependency DAG

```
T1 ──┬─► T2 ─┐
     ├─► T3 ─┤
     ├─► T4 ─┼─► T7
     └─► T6 ─┘
T5 ──┴───────┘  (T6 also depends on T5)
```

Critical path: `T1 → T2/T3/T4/T6 → T7` — 3 tasks long. T5 is off the critical path entirely.

**Expected serialization.** T2, T3, T4, T6, and T7 all declare `tests/test_mlx_executor.py`, and
T2/T3/T4/T7 also share `src/praxis_executors/adapters/mlx.py`, so they run one at a time. That is
correct and unavoidable: the spec pins the adapter to one module and its tests to one test module.
No footprint has been narrowed to fake concurrency. T5 shares nothing with any other task and runs
immediately, in parallel with T1.
