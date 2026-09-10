# Bundle remediation-2 — Enhanced Spec

## Original content

> # Bundle remediation-2
>
> ## Issues (priority findings from Round 2 post-merge audit)
> - #68 — Ollama: cancel() race silently reports SUCCEEDED instead of CANCELLED
> - #69 — Ollama: malformed /api/generate response permanently corrupts the handle
> - #70 — Ollama: capabilities() raises unhandled exceptions for malformed responses
> - #71 — Claude: auth detection permanently stubbed, adapter unselectable by default
> - #72 — Claude: launch() inherits ambient environment, risking silent metered fallback
>
> Two unrelated files (`ollama.py` x3 tasks, `claude_cli.py` x2 tasks) bundled
> together only because they were found in the same audit round.
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-remediation-2
> branch: develop/remediation-2, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build.
>
> ## Task 1 (issue #68): Ollama cancel() race
>
> `launch()` initializes `self._connections[handle_id] = []` before starting
> the worker thread; `cancel()` pops from that list, but if the worker hasn't
> reached `urlopen()` yet the list is empty and nothing is closed —
> `self._cancelled.add(handle_id)` still runs, but `_run_generate`'s success
> path never checks it, so the request completes and reports `SUCCEEDED`
> despite the cancel. Fix: check `handle_id in self._cancelled` in
> `_run_generate`'s success path too, reporting `CANCELLED` instead of
> `SUCCEEDED` when cancellation was requested. Add a test that calls
> `cancel()` immediately after `launch()`, before the connection would
> normally register (do not wait for `executor._connections.get(...)` to be
> truthy first, unlike the existing `test_cancel_closes_in_flight_request`).
>
> ## Task 2 (issue #69): Ollama malformed /api/generate payload
>
> `_run_generate` parses `payload = json.loads(...)` inside a `try`, but
> *uses* it (`payload.get(...)`) outside the `try/except Exception` block —
> if `/api/generate` returns valid JSON that isn't an object (a list, string,
> number, null), `payload.get(...)` raises `AttributeError` uncaught, the
> worker thread dies without writing to `self._results[handle_id]`, and
> `status()`/`result()` raise `KeyError` forever on that handle. Fix: move
> the payload-shape validation and result-dict construction inside the
> `try`, or explicitly check `isinstance(payload, dict)` before use,
> converting a non-dict payload into a `FAILED` `ExecutionResult` like every
> other malformed-response path in this file. Add a regression test with a
> non-dict `/api/generate` response (e.g. `["not", "a", "dict"]`).
>
> ## Task 3 (issue #70): Ollama capabilities() unhandled exceptions
>
> Three confirmed gaps in `capabilities()`: (1) `/api/show` returning
> non-dict JSON makes `show.get("capabilities")` raise `AttributeError`,
> uncaught by the surrounding `except (..., KeyError, TypeError, ValueError)`
> clause; (2) `/api/tags` returning a non-dict model entry makes
> `"name" not in model` raise `TypeError`, uncaught (the try/except only
> covers the initial fetch, not the per-model loop); (3) `/api/show`
> returning a non-list `capabilities` field makes
> `_classify_kinds(model_name, show_capabilities)` raise `TypeError`
> uncaught (called outside the guarded try). Fix: validate response shapes
> (dict-ness of top level, list-ness of `models`/`capabilities`) before use;
> widen the per-model best-effort `except` clause to include
> `AttributeError`; move `_classify_kinds`'s consumption inside the guarded
> try. Add regression tests for all three malformed shapes.
>
> ## Task 4 (issue #71): Claude auth detection stub
>
> `_detect_authenticated()` unconditionally returns `None`. `claude auth
> status --json` is a safe, non-destructive, sub-second command that reports
> `{"loggedIn": true/false, ...}` with no login dialog — wire
> `_detect_authenticated` to it (with a timeout and the same redaction
> discipline the rest of the adapter uses), parsing `loggedIn`. Add a test
> (mocked subprocess) confirming `health()` returns `AVAILABLE` when
> `loggedIn: true` is reported, and a fallback (`None`/`DEGRADED`, not a
> crash) if the command errors or the output is unparseable.
>
> ## Task 5 (issue #72): Claude launch() environment isolation
>
> `launch()` calls `subprocess.Popen(argv, ...)` with no `env=` argument, so
> the child inherits the full parent environment including
> `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN`/`ANTHROPIC_BASE_URL` if set,
> which could make the wrapped CLI silently authenticate via a metered key
> while this adapter keeps advertising `auth_transport: "subscription_cli"`.
> Fix: pass an explicit `env=` to `Popen` that strips those variables (copy
> `os.environ`, delete the Anthropic-API env keys, pass the result). Add a
> test that sets a fake `ANTHROPIC_API_KEY` in the *parent* process's
> environment (via `monkeypatch.setenv`, restored after the test) and
> confirms the subprocess's environment (assert via the fake/mocked
> `Popen` call's `env` kwarg, not a real credential) does not contain it.
>
> ## Acceptance
>
> - Full test suite (`.venv/bin/python -m pytest`) passes, including all new
>   regression tests.
> - No regression to any currently-passing test.
>
> ## Delivery
>
> Open a PR against `main` referencing #68, #69, #70, #71, #72 with closing
> keywords. Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

### Task 1 (issue #68) — cancel() race

1. **Exact edit site, confirmed by direct reading**: `src/praxis_executors/adapters/ollama.py`'s `_run_generate` (lines 247-276). The success path currently at lines 269-276 unconditionally builds `ExecutionResult(status=ExecutorStatus.SUCCEEDED, ...)`. Change this one status literal to `ExecutorStatus.CANCELLED if handle_id in self._cancelled else ExecutorStatus.SUCCEEDED` — mirroring the exact pattern already used one block above it in the `except Exception` branch (line 265: `ExecutorStatus.CANCELLED if handle_id in self._cancelled else ExecutorStatus.FAILED`). No lock is needed around this check: `self._cancelled` is only ever added to (never removed from) and a plain `in` membership test on a `set` is a single atomic bytecode-level operation under the GIL, the same assumption the existing exception-branch check already relies on.
2. **New test, precisely scoped**: add to `tests/test_ollama_executor.py` (sibling of `test_cancel_closes_in_flight_request`, lines 302-343). Unlike that test, do **not** poll `executor._connections.get(handle.handle_id)` before calling `cancel()` — call `cancel()` right after `launch()` returns, while the worker thread is still virtually guaranteed not to have reached `urlopen()` yet. Assert the terminal status (via `_wait_for_terminal`, already defined at line 247) is `CANCELLED`, not `SUCCEEDED`. A real (not injected-delay) `/api/generate` canned response is sufficient — the point of this test is the race window before registration, not an in-flight close.

### Task 2 (issue #69) — malformed `/api/generate` payload

1. **Resolved: use the `isinstance(payload, dict)` option, and give it a fixed home relative to Task 1's edit.** Both options the original task offers are behaviorally equivalent for this bug alone, but Task 1 and Task 2 touch the same three lines (the post-`try` success block, `ollama.py:269-276`), so the two fixes must compose into one final block rather than being written independently. Concretely, replace lines 269-276 with:
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
   See **Assumptions made** for why cancellation is checked first (before payload-shape validation), rather than the two checks being independent or payload-shape being checked first.
2. **New test, precisely scoped**: add to `tests/test_ollama_executor.py`, mirroring `test_successful_generate_reaches_succeeded` (lines 283-299) but with `responses["/api/generate"] = (200, ["not", "a", "dict"])`. Assert the terminal status is `FAILED` (via `_wait_for_terminal`) and that `executor.result(handle)` does not raise (i.e. the handle resolved instead of hanging in `KeyError` territory forever, the exact regression issue #69 reports).

### Task 3 (issue #70) — capabilities() unhandled exceptions

All three fixes live inside `capabilities()` (`ollama.py:163-214`); confirmed by direct reading, no other function needs to change.

1. **Gap 1 fix (`/api/show` non-dict JSON → `AttributeError`)**: widen the per-model `except` clause at line 186 from `except (_OllamaUnreachable, _OllamaHTTPError, KeyError, TypeError, ValueError):` to also include `AttributeError`. This is purely additive to an already-established best-effort-degrade pattern (confirmed by the existing test `test_capabilities_omits_context_window_when_show_response_is_malformed`, `tests/test_ollama_executor.py:198-209`, which already exercises this except clause for a different malformed-JSON case).
2. **Gap 2 fix (`/api/tags` non-dict model entry → uncaught `TypeError`)**: add a shape check immediately before the existing `if "name" not in model:` check (line 173), of the same fail-fast style already established there:
   ```python
   if not isinstance(model, dict):
       raise ExecutorError(f"ollama /api/tags returned a non-dict model entry: {model!r}")
   ```
   This raises `ExecutorError` (aborting the whole `capabilities()` call), the same as the adjacent, already-tested "missing `name`" case (`test_capabilities_raises_executor_error_for_model_entry_missing_name`, `tests/test_ollama_executor.py:212-220`, whose own docstring already frames this as "a handled `ExecutorError`, not an uncaught `KeyError`" — the same standard extends naturally to a non-dict entry). See **Assumptions made** for why this is fail-fast rather than best-effort-skip, unlike the `/api/show` per-model gaps.
3. **Gap 3 fix (`/api/show` non-list `capabilities` → uncaught `TypeError` in `_classify_kinds`)**: move the `_classify_kinds(model_name, show_capabilities)` call (currently at line 197, outside the guarded try) so its result is computed inside the same try/except block that already wraps the `/api/show` call (lines 180-186), storing the resulting `kinds` list in a variable used later when building `capability["satisfies"]`. On any exception in that block (now including a non-list `capabilities` triggering `_classify_kinds`'s internal `TypeError`), fall back to treating it exactly like an absent `show_capabilities`: `kinds = _classify_kinds(model_name, None)`. See **Assumptions made** for why this fallback (not an empty list, and not omitting the capability's `satisfies` key) is the only schema-consistent choice.
4. **New tests, three, one per gap**: extend `tests/test_ollama_executor.py` with canned `/api/show` and `/api/tags` responses for each of: (a) `/api/show` returning a JSON array instead of an object; (b) `/api/tags`'s `models` containing a non-dict entry (e.g. `"llama3"` as a bare string); (c) `/api/show` returning `{"capabilities": "not-a-list"}`. For (a) and (c), assert `capabilities()` still succeeds and the affected model's capability entry is present (with `context_window` omitted for (a), and `satisfies` containing at least `{"kind": "reasoning", ...}` for (c)); for (b), assert `capabilities()` raises `ExecutorError`, mirroring the existing missing-`name` test's shape.

### Task 4 (issue #71) — Claude auth detection stub

1. **Exact edit site**: `src/praxis_executors/adapters/claude_cli.py`'s `_detect_authenticated` (lines 82-85), currently `return None` unconditionally. Replace with a `subprocess.run([cli_path, "auth", "status", "--json"], capture_output=True, text=True, timeout=5)` call (reusing `timeout=5`, the same value `_probe_version` already uses one method above at line 78, for consistency within this class rather than inventing a new number), parsing `json.loads(result.stdout)["loggedIn"]` inside a `try`/`except` that catches at least `subprocess.TimeoutExpired`, `OSError`, `json.JSONDecodeError`, and `KeyError`/`TypeError` (a present-but-shape-wrong response), returning `None` on any of them — matching the existing `_probe_version`'s own `except (OSError, subprocess.TimeoutExpired): pass` tolerance style one method above it.
2. **`health()` itself needs no change.** Confirmed by direct reading (lines 62-74): it already branches on `_detect_authenticated`'s three possible return values (`True` → `AVAILABLE`, `False` → `UNAVAILABLE`, `None` → `DEGRADED`) — this task is scoped entirely to `_detect_authenticated`'s body.
3. **Existing test needs updating, not just a new test added.** `test_detect_authenticated_unmocked_returns_none_by_default` (`tests/test_claude_cli.py:135-144`) currently patches `shutil.which` and `subprocess.run` (the latter with no `return_value`, i.e. a bare `MagicMock()`) and asserts the *real*, unmocked `_detect_authenticated` returns `None`, with a docstring claiming this "proves the 'no guess' default is actually wired in, not just mockable." Once `_detect_authenticated` is implemented, this same test will still pass (a bare `MagicMock().stdout` fed to `json.loads` raises, which the new `except` clause degrades to `None`) — but *why* it passes changes: it no longer proves "no guess," it now coincidentally exercises the unparseable-output fallback path instead. Rewrite this test's docstring and name (e.g. `test_detect_authenticated_returns_none_when_subprocess_run_output_is_unmocked_and_unparseable`) so it accurately describes what it verifies post-fix, rather than leaving a stale, now-inaccurate rationale in place — this repository has direct precedent for this exact situation (`docs/develop/specs/remediation-1.md`'s Task 3, citing commit `861e3b8`: rewrite a test whose pinned rationale a later fix invalidates, rather than leaving it passing by accident).
4. **New test, precisely scoped**: add a test mocking `subprocess.run` to return a fake completed-process object whose `.stdout` is `'{"loggedIn": true}'`, patching only at the `_detect_authenticated`-call boundary already established by this file's existing patch target (`praxis_executors.adapters.claude_cli.subprocess.run`) — then assert `executor.health()` (not just `_detect_authenticated` in isolation) reaches `ExecutorAvailability.AVAILABLE`, per the original task's own acceptance wording. Add the mirrored `loggedIn: false` → `UNAVAILABLE` case for symmetry with the already-existing `test_health_unavailable_when_cli_present_and_not_authenticated` (lines 113-122), and at least one unparseable-output case (e.g. `.stdout = "not json"`) asserting `DEGRADED`.

### Task 5 (issue #72) — Claude launch() environment isolation

1. **Exact edit site**: `src/praxis_executors/adapters/claude_cli.py`'s `launch()` (lines 87-106), specifically the `subprocess.Popen(argv, ...)` call at lines 96-101. Add `env=` computed as: `env = os.environ.copy()`, then `for key in ("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"): env.pop(key, None)`, then pass `env=env` to `Popen`. Requires adding `import os` to this module (not currently imported — confirmed by reading the file's import block, lines 9-12).
2. **New test, precisely scoped**: use `monkeypatch.setenv("ANTHROPIC_API_KEY", "fake-value-for-test")` in the parent test process, then patch `subprocess.Popen` (the same boundary every other `launch()` test in this file already patches, e.g. `test_scripted_successful_run_reaches_succeeded_with_true_evidence`, lines 191-206) and assert on the mock's `call_args.kwargs["env"]` — specifically that the key is absent (or the dict as a whole does not equal `os.environ` verbatim), never asserting against the real fake value's literal presence/absence in a way that would pass trivially if `env=` were omitted entirely (i.e. assert the kwarg was passed at all, not just that a search for the fake value came up empty in something that might not have been checked).

## Explicitly out of scope

- **Task 4**: broadening the set of commands or auth signals `_detect_authenticated` consults (e.g. reading `~/.claude` config files directly, checking `ANTHROPIC_API_KEY`'s presence as a proxy for auth state) — the issue names exactly one command (`claude auth status --json`); if that command's real-world output ever needs a second source of truth, that is a new, separately-scoped decision, not silently folded into this task.
- **Task 5**: stripping any Anthropic-related environment variable beyond the three named (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_BASE_URL`) — e.g. `ANTHROPIC_CUSTOM_HEADERS`, or Bedrock/Vertex-provider environment variables (`CLAUDE_CODE_USE_BEDROCK`, AWS/GCP credentials, etc.). The issue's stated threat model is specifically the ambient-API-key/base-url metered-fallback case; widening the denylist is a separate hardening decision this bundle does not make on its own, since it touches a security boundary and has no existing repository convention to anchor a wider list against (fails resolve-or-name condition 4 — see the note in **Assumptions made** about why this was named here, not filled).
- **All tasks**: any change to `praxis_executors.interface` (`ExecutorStatus`, `ExecutionResult`, `Executor` ABC) or any `schemas/v1/*.schema.json` file — every fix in this bundle is internal to `ollama.py`/`claude_cli.py` and their own tests; none requires a new status value, a new evidence/payload shape, or a schema change. `docs/executors.md` needs no update — it makes no claim about `_detect_authenticated`'s internals or these specific bug behaviors that this bundle would make stale (checked directly: it describes `ClaudeCliExecutor` only at the level of `auth_transport: "subscription_cli"` and its role as a `claude`-CLI-driving adapter, both still accurate after this fix).
- **All tasks**: merging the PR (merge policy is `never` for this repository, per the spec's own Delivery section).

## Assumptions made

- **Task 1 + Task 2 compose into one block, with cancellation checked before payload-shape validation.** Both tasks edit the same post-`try` success block in `_run_generate` (`ollama.py:269-276`). Evidence for "cancelled wins": the existing `except Exception` branch immediately above (line 265) already establishes the precedent that a `CANCELLED` outcome takes priority over a `FAILED` one when both conditions could apply to the same handle (that branch reports `CANCELLED` even when the underlying exception was a generic one, as long as `handle_id in self._cancelled`). Extending that same precedence into the non-exception path — check cancellation once, before evaluating whether the payload is a well-shaped dict — keeps a single consistent rule ("a cancelled handle is always reported as `CANCELLED`, regardless of what else happened to the request") rather than two independently-reasoned checks that could disagree. Passes resolve-or-name: in scope (both tasks touch this exact block), a direct sibling precedent exists three lines up in the same method, doesn't change either task's stated acceptance criteria (neither names the interaction, so no criterion is being narrowed), no security/compliance implication, and is trivially reviewable (one `if` block, cited above verbatim).

- **Task 3, gap 2: a non-dict `/api/tags` model entry is fail-fast (`ExecutorError`, aborting `capabilities()`), not best-effort-skip.** The original task's fix list separately calls out "the per-model best-effort `except` clause" (singular, referring to the `/api/show` per-model try at lines 180-186) when discussing gap 1 and gap 3, but does not describe gap 2's fix as "best-effort" — and the existing, already-tested behavior for a malformed `/api/tags` entry (missing `name`, line 173-176) is already fail-fast via `raise ExecutorError`, not skip-and-continue. Treating a non-dict entry the same way is the direct, minimal extension of that existing convention, not a new design choice. This also avoids a subtler inconsistency: `/api/tags`'s model list is this method's primary source of *which models exist at all* (identity), whereas `/api/show` is supplementary metadata for a model whose identity is already established — the fail-fast/best-effort split already drawn in the current code lines up exactly with that identity-vs-metadata distinction. Passes resolve-or-name: in scope, a direct sibling example three lines away in the same loop, doesn't change the stated "add regression tests for all three malformed shapes" criterion (only specifies what the test for gap 2 should assert), no security implication, reviewable.

- **Task 3, gap 3: the fallback when `_classify_kinds` raises is `_classify_kinds(model_name, None)`, not an empty list or a hardcoded literal.** Two reasons converge on this being the only viable choice, not merely a reasonable one: (1) `_classify_kinds`'s own docstring and body (`ollama.py:101-119`) already guarantee a non-empty result for any input, including `None`, because `"reasoning"` is unconditionally appended when absent — so re-invoking it with `None` (the same value used when `/api/show` fails entirely, per the existing per-model `except` clause's `context_window = None` fallback) reuses an already-established "malformed show response degrades this model's optional data, not the model's `satisfies` claim" pattern; (2) `schemas/v1/capability.schema.json`'s `satisfies` field is `"minItems": 1` (line 18) — falling back to a literal `[]` would produce a schema-invalid advertisement and fail `test_capabilities_advertisement_validates_against_schema`-style checks, so it is not a neutral alternative. Passes resolve-or-name: in scope, defensible from both a direct code contract and a schema constraint (not just convention), doesn't shrink the stated acceptance criterion (still "capabilities() doesn't raise, regression test added"), no security implication, reviewable.

- **Task 4: reuse `timeout=5` for the new `subprocess.run` call**, matching `_probe_version`'s existing `subprocess.run([cli_path, "--version"], ..., timeout=5)` one method above (`claude_cli.py:78`) rather than inventing a different value. Evidence: direct sibling call in the same class, same file, for a comparably-scoped "quick CLI probe" purpose; the original task only specifies "with a timeout," not a value. Passes resolve-or-name on all five conditions (in scope, direct sibling default, doesn't change any stated criterion, no security implication in the timeout value itself, reviewable/adjustable later).

- **Task 4: the existing `test_detect_authenticated_unmocked_returns_none_by_default` must be rewritten (name and docstring), not left in place unchanged.** See **Clarified acceptance criteria** Task 4 item 3 for the full reasoning; this repository has direct, named precedent for rewriting a test whose pinned rationale a later fix invalidates rather than leaving it passing by accident (`docs/develop/specs/remediation-1.md`, commit `861e3b8`). Passes resolve-or-name: in scope (same file/task), direct repository precedent cited by commit, doesn't change what the *behavior* being tested is (only the test's own description of it), no security implication, reviewable.

- **Task 4: the exact shape of `claude auth status --json` (that it exists, is safe/non-destructive/sub-second, and reports `{"loggedIn": bool, ...}`) is taken as given from the original issue text and not independently re-verified here.** This enhancement pass attempted to confirm it directly (`claude auth --help` / `claude auth status --help` in this environment) but both required interactive approval unavailable in this headless run, so it could not be executed. This is not escalated as an open question because the task's own required fallback bounds the risk: if the command name, flag, or field name turns out to be wrong, the implementation's own "unparseable/erroring output → `None`/`DEGRADED`, never a crash" acceptance criterion (already stated in the original task and restated in **Clarified acceptance criteria** above) means the adapter degrades to exactly today's behavior (`DEGRADED`) rather than failing loudly or corrupting other behavior — the blast radius of this assumption being wrong is already capped by a criterion the task independently requires.

## Open questions

None. Every gap found during this pass (the Task 1/Task 2 interaction, the two ambiguous-fallback points in Task 3, and Task 4's timeout/test-staleness details) satisfied all five resolve-or-name conditions — each is inside its task's stated scope, anchored to a direct sibling example or an existing schema/code constraint already in this file or its tests (not merely "what seems reasonable"), leaves every originally-stated acceptance criterion's meaning unchanged, carries no security/compliance/public-API/cost/deployment-blast-radius implication of its own (the one item that does touch a security boundary — how wide the Task 5 env-var denylist should be — was named as **Explicitly out of scope** instead of filled), and is independently checkable by whoever reads this next against the file/line citations given.
