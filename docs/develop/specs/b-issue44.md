# Bundle b-issue44 — Enhanced Spec

MLX / local Hugging Face executor adapter.

## Original content

> # Bundle b-issue44: MLX / local Hugging Face executor adapter
>
> ## Issues
> - #44 — MLX / local Hugging Face executor adapter
>
> ## Scope
> Add `src/praxis_executors/adapters/mlx.py` for locally hosted MLX models on compatible
> Apple Silicon, only if it fits the existing `Executor` ABC cleanly (follow the shape of
> the existing Ollama adapter, `src/praxis_executors/adapters/ollama.py`). Do not build a
> model-serving framework inside Praxis — if an existing local OpenAI-compatible server
> (e.g. `mlx_lm.server`) provides the right boundary, adapt to that HTTP surface instead
> of embedding MLX directly.
>
> ## Base
> `origin/main` at commit 9366f5b (praxis CLI scaffold + executors discover/executors/match merged).

## Clarified acceptance criteria

### 1. The conditional in the original scope is resolved: build the adapter, over HTTP

The original scope is conditional ("only if it fits the existing `Executor` ABC cleanly"). It does
fit, and the bundle is to be built. `OllamaExecutor` (`src/praxis_executors/adapters/ollama.py`) is
an existing, shipped proof that a local HTTP model server maps onto the six-method `Executor` ABC
(`src/praxis_executors/interface.py:75-100`) without any ABC change: a background thread turns the
server's blocking request/response cycle into the ABC's
launch-returns-a-handle-immediately/poll-until-terminal contract
(`ollama.py:236-252`, `254-295`, `303-307`). `mlx_lm.server` presents the same kind of local HTTP
surface, so the same structure applies with no new interface work.

The adapter therefore talks HTTP to an already-running local `mlx_lm.server` process. It never
imports `mlx`, `mlx_lm`, or `transformers`; it never loads a model, manages weights, or starts a
server process. That is the original scope's "do not build a model-serving framework inside Praxis"
constraint, made concrete and checkable: `src/praxis_executors/adapters/mlx.py` must import only
stdlib modules plus `praxis_executors.interface`, exactly as `ollama.py:11-29` does.

### 2. New file, stdlib only, no new dependency

Add `src/praxis_executors/adapters/mlx.py` and `tests/test_mlx_executor.py`. Use
`urllib.request`/`urllib.error`/`urllib.parse` from the stdlib for HTTP, matching `ollama.py:16-18`.
Do not add any entry to `pyproject.toml`'s `dependencies` or `optional-dependencies`, which today
name only `jsonschema`, `referencing`, `pytest`, and `build` (`pyproject.toml:9,12`).

### 3. `executor_id` is a constructor parameter, never hardcoded

`MlxExecutor.__init__` must accept `executor_id: str` from its caller, as all five shipped adapters
do (`ollama.py:141-153`). Never default or hardcode it to a literal containing `"mlx"`, `"lm"`, a
Hugging Face repo id, or any model name: `docs/ontology.md:20-21` states as a core ontology rule
that `CapabilityAdvertisement.executor_id` "must not encode a vendor or model name", and
`capability-advertisement.schema.json:13-16` repeats it in the schema's own description.

### 4. `base_url` is configurable and loopback-restricted

The constructor accepts `base_url: str` defaulting to `http://127.0.0.1:8080` (see
**Assumptions made** for why that port) and raises `ValueError` at construction time when the host
is not a loopback address. Reuse `ollama.py:53-70`'s semantics exactly: `localhost` by string
comparison, everything else through `ipaddress.ip_address(host).is_loopback` so the whole
`127.0.0.0/8` range and IPv6 `::1` are covered without reimplementing CIDR logic.

This is what makes `auth_transport: "local"` (criterion 7) true of the adapter's runtime behaviour
rather than merely its label — `AuthTransportPolicy` treats `"local"` as safe-by-default, unlike
`metered_api`/`api_key` (`src/praxis_executors/policy.py:22-25`). It is also what lets the tests
point the adapter at a fake server on an OS-assigned ephemeral port (criterion 10).

Construction must not probe the environment. No HTTP call, no `shutil.which`, no filesystem check in
`__init__` beyond the `base_url` validation above — `src/praxis_cli/adapters.py:1-6` states this as a
standing property of every adapter the CLI can build, even though this bundle does not wire itself
into that CLI (see **Explicitly out of scope**).

### 5. Two timeouts, not one

A short timeout (default a few seconds) for `health()`/`capabilities()` so an unreachable or hung
server fails fast, and a separate, longer, caller-configurable timeout for generation calls, since
real local inference legitimately takes far longer than a model list. Mirrors
`ollama.py:141-156`'s `timeout=5.0` / `generate_timeout=120.0` pair.

### 6. `health()` uses all three `ExecutorAvailability` values

Probe `GET /v1/models` (criterion 7) and return:

- `UNAVAILABLE` — the server is unreachable (connection refused, DNS failure, timeout). Must
  **return**, never raise: a discovery pass over several adapters cannot be allowed to fail because
  one local server is not running.
- `DEGRADED` — reachable but answering non-2xx, or reachable and reporting zero models.
  Zero models cannot be `AVAILABLE` because `capability-advertisement.schema.json:17-19` sets
  `"minItems": 1` on `capabilities`, so `capabilities()` could not legally answer for such a server.
- `AVAILABLE` — reachable with at least one model.

Distinguishing "unreachable" from "reachable but erroring" requires catching
`urllib.error.HTTPError` **before** the broader `urllib.error.URLError`, since the former is a
subclass of the latter. `ollama.py:43-50` and `90-98` document and implement exactly this ordering;
getting it wrong silently collapses `DEGRADED` into `UNAVAILABLE`.

`capabilities()`, by contrast, raises `ExecutorError` when the server cannot be reached or answers
non-2xx (`ollama.py:164-169`). Callers are expected to check `health()` first, which is what
`ExecutorRegistry.advertisements(healthy_only=True)` already does by default
(`src/praxis_executors/registry.py`, described at `docs/executors.md:227`).

### 7. Capability advertisement shape

`capabilities()` calls `GET /v1/models`, reads each entry's `id` out of the OpenAI-shaped
`{"object": "list", "data": [{"id": ..., ...}, ...]}` body, and emits one capability per model:

```json
{
  "spec_version": "1.0.0",
  "satisfies": [{"kind": "reasoning", "parameters": {"model": "mlx-community/..."}}],
  "auth_transport": "local",
  "platform": "macos"
}
```

- `kind` values come only from the repository's documented standard vocabulary
  (`docs/executors.md:56-71`: `coding`, `reasoning`, `planning`, `filesystem`, `shell`, `tools`,
  `vision`, `long-context`, `structured-output`, `repository-access`, `web`). A raw Hugging Face repo
  id such as `mlx-community/Qwen2.5-7B-Instruct-4bit` would not satisfy
  `capability.schema.json`'s `^[a-z0-9]+(-[a-z0-9]+)*$` pattern on `kind` anyway, and
  `docs/ontology.md:13-16` forbids putting a model name there regardless.
- The model identifier goes in `satisfies[].parameters.model`, an open object
  (`capability.schema.json`, `parameters.additionalProperties: true`). This is the same mechanism
  `docs/executors.md` already documents for carrying adapter-specific data without a schema change.
- Classification: name heuristic only. Every discovered model advertises `reasoning` as the generic
  baseline; a model whose id contains `code` or `coder` additionally advertises `coding`. The exact
  substring logic is an implementation detail, not a pinned contract — no acceptance test asserts a
  particular model id maps to a particular kind set beyond the baseline.
- `auth_transport: "local"` on every capability.
- `platform: "macos"` on every capability (`capability.schema.json`'s `platform` is an open
  illustrative string, documented at `docs/executors.md:85-87`). This is advertised metadata only.
  It must not gate anything: `matching.match` ranks on satisfied kinds and the cost hint alone
  (`docs/executors.md:104-120`), so `platform` is inert, and no code path may branch on the host's
  actual architecture (see criterion 9).
- `context_window` is omitted entirely. The OpenAI-compatible `/v1/models` surface carries no
  context-length metadata and there is no documented per-model metadata endpoint to fetch it from,
  unlike Ollama's `POST /api/show` (`ollama.py:122-135`). Omitting an optional field is schema-legal
  (`capability.schema.json` requires only `spec_version` and `satisfies`).
- A reachable server reporting zero models makes `capabilities()` raise `ExecutorError`, matching
  `ollama.py:212-215` and the `minItems: 1` constraint above.

### 8. Execution lifecycle

`launch()` validates `request.parameters`, starts a background daemon thread, and returns an
`ExecutionHandle` immediately. `status()` reports `RUNNING` while that thread is alive and the
recorded terminal status afterwards. `result()` raises `ExecutorError` when called while the
execution is still running. `status()`, `cancel()`, and `result()` all raise `ExecutorError` for an
unknown `handle_id`. This is `ollama.py:236-252`, `297-323` applied unchanged in structure.

- **Endpoint**: `POST /v1/chat/completions` with `"stream": false`.
- **Request parameters**: `request.parameters` must carry `model` (str) and `prompt` (str); a missing
  key raises `ExecutorError`, matching `ollama.py:236-241`. The adapter wraps `prompt` into
  `messages: [{"role": "user", "content": prompt}]` so the server applies the model's chat template.
  Keeping the adapter-facing key as `prompt` rather than `messages` means `MlxExecutor` and
  `OllamaExecutor` accept an identical `ExecutionRequest`, so a caller can swap one for the other
  without reshaping its request.
- **Result payload**: on success, `ExecutionResult(status=SUCCEEDED, payload=...)` carrying the
  assistant message text, the model the server reports, and the `finish_reason`. Read them
  defensively with `.get()` at every level — the nested
  `choices[0].message.content` path is three unchecked subscripts deep and a missing or
  differently-shaped `choices` must produce a `FAILED` result, never an exception escaping the
  worker thread (criterion 9).
- **Cancel**: `cancel()` closes the in-flight HTTP connection for that handle and marks the handle
  cancelled, the transport equivalent of `SubprocessExecutor.cancel`'s `process.terminate()`.
  Best-effort is sufficient; see **Explicitly out of scope**.

### 9. The four failure modes that cost the Ollama adapter a repair cycle must not recur

`docs/develop/specs/remediation-2.md:8-10` records three post-merge bug issues filed against the
Ollama adapter (#68, #69, #70), and `tests/test_repair_findings_b1_issue41.py` a fourth class
against the Codex adapter. This adapter is structurally identical, so the same four gaps are live
here and each needs a test of its own:

1. **Cancel race (#68).** The worker thread's *success* path must check the cancelled set before
   reporting `SUCCEEDED`, not only its exception path. A `cancel()` that lands after the response has
   fully arrived must still surface as `CANCELLED`, per `ollama.py:276-281`. Test it by calling
   `cancel()` immediately after `launch()` returns, before the worker thread has reached `urlopen()`.
2. **Handle never resolves (#69).** Every path through the worker thread must write exactly one
   `ExecutionResult` for its handle, including on a malformed, non-JSON, or non-dict response body.
   A path that returns without recording a result leaves `status()`/`result()` raising `KeyError`
   forever. `ollama.py:271-295` resolves the handle on all four of its exits. Test with a canned
   `/v1/chat/completions` body that is a JSON array rather than an object, asserting the terminal
   status is `FAILED` and `result()` does not raise.
3. **`capabilities()` leaking non-`ExecutorError` exceptions (#70).** A `/v1/models` response that is
   valid JSON but not the expected shape (a list instead of an object, `data` not a list, an entry
   that is not a dict, an `id` that is not a string) must surface as a handled `ExecutorError`, never
   a bare `KeyError`/`TypeError`/`AttributeError`. Note `json.JSONDecodeError` and
   `UnicodeDecodeError` are both `ValueError` subclasses. `src/praxis_cli/fields.py:26-48` explains
   why this matters downstream: the CLI nets `(ExecutorError, ValueError, AttributeError, TypeError)`
   and logs anything outside an adapter's own vocabulary as *more likely a defect in the adapter than
   an outage in the service*.
4. **Stale documentation (the #41 class).** See criterion 11.

No code path may branch on the host's CPU architecture or OS (`platform.machine()`, `sys.platform`,
`uname`). The adapter's only evidence about Apple Silicon is that a local `mlx_lm.server` answered
on loopback, which is stronger than any host probe and keeps the test suite runnable on any
platform. Apple Silicon appears in the advertisement as metadata (criterion 7) and in the module
docstring, never as a runtime gate.

### 10. Tests: a real fake server, no real MLX dependency

Add `tests/test_mlx_executor.py` following `tests/test_ollama_executor.py:1-60`'s established
pattern: a `ThreadingHTTPServer` bound to `port=0` driven by a background `.serve_forever()` thread
and torn down with `.shutdown()`, with a mutable per-test `responses` dict mapping path to
`(status, body)` so each test swaps in canned `/v1/models` and `/v1/chat/completions` bodies. A
`bytes` body is sent verbatim so a malformed/non-JSON response can be simulated, and a per-path
delay dict lets headers go out before the body so an in-flight `cancel()` has a live connection to
close. Point `MlxExecutor(base_url=...)` at the assigned ephemeral port.

Cover, at minimum: loopback validation (accept and reject), all three `health()` verdicts including
reachable-but-erroring and reachable-but-empty, the advertisement shape, a
`test_capabilities_advertisement_validates_against_schema` test in the same shape as
`tests/test_ollama_executor.py`'s (via `praxis_contracts.validator.validate_document` against
`capability-advertisement.schema.json`), missing `model`/`prompt`, the success path, the four
failure modes in criterion 9, unknown-handle errors, and `result()` while running.

A real-`mlx_lm.server` smoke test is optional and, if added, must `pytest.skip` rather than fail when
no server is reachable — `tests/test_ollama_executor.py` already carries exactly one such skipped
test (1 of its 24).

### 11. `docs/executors.md` must be updated in the same bundle

`docs/executors.md:240-269` ("Adding a new executor adapter") ends with a prose inventory that names
MLX as a hypothetical adapter that does not exist yet and counts five concrete adapters. Landing this
adapter makes both claims false. `tests/test_repair_findings_b1_issue41.py:1-18,69-90` exists
precisely because this was missed once already for `CodexCliExecutor` and had to be repaired after
merge; its assertions are the template for what is required here:

- Remove `MLX` from the hypothetical-adapters list at `docs/executors.md:257` (the existing
  regression test asserts, for Codex, that no `hypothetical adapters (...)` list mentions a shipped
  adapter — the same must hold for MLX).
- Add `MlxExecutor`, its module path `src/praxis_executors/adapters/mlx.py`, its
  `auth_transport: "local"`, and a one-clause description to the concrete-adapters list, and update
  the count from five to six.
- Keep the closing sentence accurate about which adapters are registered with an `ExecutorRegistry`
  by default (`docs/executors.md:267-269`) — none are, and this bundle does not change that.

Add a test to `tests/test_mlx_executor.py` pinning these doc claims, in the shape of
`tests/test_repair_findings_b1_issue41.py:57-90`: pin that MLX is no longer listed as hypothetical
and that `MlxExecutor` plus its module path appear among the concrete adapters. Do **not** pin the
adapter count or the list ordering — the existing test's own comment
(`tests/test_repair_findings_b1_issue41.py:78-81`) explains that pinning the running count makes the
next adapter's bundle fail for an unrelated reason.

### 12. Whole suite green

`python3 -m pytest` from the worktree root passes, with no pre-existing test modified. There is no
`.venv` in this worktree; `pyproject.toml:26` sets `pythonpath = ["src"]`, so the suite runs against
the source tree directly. Verified during this pass: `python3 -m pytest tests/test_ollama_executor.py`
→ 23 passed, 1 skipped.

## Explicitly out of scope

- **Importing or embedding `mlx` / `mlx_lm` / `transformers`, loading weights, downloading models,
  or starting a server process.** The original scope forbids building a model-serving framework
  inside Praxis. Starting, supervising, or auto-launching `mlx_lm.server` is part of that
  prohibition: the adapter assumes the server is already running and reports `UNAVAILABLE` when it
  is not.
- **Wiring `MlxExecutor` into `src/praxis_cli/adapters.py`.** `_ADAPTER_FACTORIES`
  (`src/praxis_cli/adapters.py:31-46`) holds four adapters and deliberately does not include
  `CodexCliExecutor`, which shipped before it — so a new adapter is not expected to add itself.
  `tests/test_cli_adapters.py:26,38-41,54` additionally pins both `len(adapters) == 4` and the exact
  id list, so adding an entry would break a test this bundle has no mandate to change.
  `src/praxis_cli/fields.py:275-278` already degrades an unrecognised adapter class to `"n/a"`
  without raising, so the CLI stays correct with `MlxExecutor` unwired. If CLI exposure is wanted,
  it is a separate increment that also updates `_advertisement_answers_for_health`
  (`fields.py:199-232`, whose docstring anticipates "a fifth adapter with the same property").
- **Registering an instance with an `ExecutorRegistry` anywhere in application code.**
  `docs/executors.md:267-269` states none of the shipped adapters is registered by default and that
  construction plus registration is the caller's job.
- **Any change to `src/praxis_executors/interface.py`, `matching.py`, `policy.py`, `registry.py`, or
  any `schemas/v1/*.schema.json`.** This bundle adds one adapter module plus its tests, and edits
  `docs/executors.md`. Nothing in criteria 1-12 needs a new `ExecutorStatus` value, a new capability
  field, or a schema change.
- **Refactoring `ollama.py`.** Its HTTP and loopback helpers are not to be moved, renamed, or
  rewritten (see **Assumptions made** on duplication).
- **`POST /v1/completions`, streaming (`"stream": true`), token-level streaming of partial output,
  multi-turn chat history, tool/function calling, structured-output/JSON-mode, and embeddings.**
  `/v1/chat/completions` non-streaming is the required minimum. Non-streaming keeps both the fake
  test server and the response parser a single JSON object with no server-sent-event parsing.
- **Vision/multimodal models.** Those run under `mlx-vlm`, a different server surface; no capability
  advertises `vision`.
- **`interactive` and `availability` capability fields** (`capability.schema.json`). Optional,
  unmentioned by the original scope, and `context_window` is omitted per criterion 7.
- **Non-loopback or remote `mlx_lm.server` support of any kind** (criterion 4).
- **Guaranteed, race-free mid-generation cancellation.** Best-effort connection close plus the
  cancelled-set check in criterion 9.1 is sufficient. What must hold is that a cancelled handle is
  never reported as `SUCCEEDED`; what need not hold is that the server actually stops generating.
- **Model-selection policy, cost/latency hints, and benchmark or eval integration.** Advertising
  capabilities is this bundle's job; choosing among them is `matching`/`policy`'s, already shipped.

## Assumptions made

Each fill below passed the resolve-or-name test: inside the original scope, backed by something
already on record rather than by what merely seems reasonable, not changing what any stated
requirement means, no security/retention/public-API/cost/deployment-blast-radius impact, and
correctable by the next reader.

1. **The ABC fits, so the bundle is built, and it is built over HTTP against `mlx_lm.server` rather
   than by embedding MLX** (criterion 1). Evidence: the original scope names `mlx_lm.server` itself
   as the preferred boundary, and `OllamaExecutor` is a shipped, tested instance of the same
   local-HTTP-server-onto-`Executor`-ABC mapping (`ollama.py` in full; 24 tests green). Resolving
   the conditional in favour of building is not an expansion of scope — the alternative reading
   (conclude it does not fit and deliver nothing) is contradicted by a working sibling adapter.

2. **Default `base_url` is `http://127.0.0.1:8080`** (criterion 4). Evidence: `mlx_lm.server`'s
   documented defaults bind loopback on port 8080, and the sibling spec for the Ollama adapter
   (`docs/develop/specs/b2-issue43.md:84-95`) established the pattern of taking the upstream
   project's own documented default address as the constructor default. The implementer should
   confirm the port against `mlx-lm`'s current documented CLI before writing it down, exactly as
   the Ollama spec directed for port 11434 ("confirm the actual default port/endpoint from Ollama's
   own documented API"). The value is a constructor default, not a constant, so correcting it is a
   one-line change with no behavioural fan-out.

3. **`GET /v1/models` is the discovery and health probe, and the three-state `health()` mapping in
   criterion 6.** Evidence: `/v1/models` is the one discovery endpoint the OpenAI-compatible surface
   guarantees, so depending on it avoids depending on `mlx_lm`-specific extras such as `/health`
   whose presence varies by version. The three-state mapping is transplanted from
   `ollama.py:223-234` and forced by `capability-advertisement.schema.json:17-19`'s `minItems: 1`:
   a server with zero models has nothing legal to advertise, so it cannot be `AVAILABLE`, and
   `DEGRADED` exists in `ExecutorAvailability` (`interface.py:24-29`) for exactly this state.

4. **`POST /v1/chat/completions` with `"stream": false`, and the adapter-facing parameter contract
   stays `model` + `prompt`** (criterion 8). Evidence: the original scope says "OpenAI-compatible
   server", of which chat completions is the primary surface, and instruct-tuned models need the
   server-side chat template that `/v1/completions` bypasses. Keeping `prompt` as the adapter's own
   key (wrapped into a single user message internally) matches `ollama.py:236-241`'s two-required-key
   convention, so the two local adapters accept an identical `ExecutionRequest`. This does not
   narrow any stated requirement: the original scope names no endpoint at all.

5. **Kind classification by name heuristic only, with `reasoning` as the unconditional baseline and
   `coding` added on a `code`/`coder` substring** (criterion 7). Evidence: `_classify_kinds`
   (`ollama.py:101-119`) is the established heuristic, and its metadata-preferring branch has no
   analogue here because `/v1/models` exposes no capability hints. The unconditional `reasoning`
   baseline is load-bearing, not cosmetic: `capability.schema.json` sets `"minItems": 1` on
   `satisfies`, so a model classified into zero kinds would produce a schema-invalid advertisement —
   the same reasoning `docs/develop/specs/remediation-2.md:180` records for the Ollama fallback.

6. **`auth_transport: "local"` and `platform: "macos"` on every capability, both inert metadata**
   (criterion 7). Evidence for `"local"`: `policy.py:22-25`'s vocabulary, where `"local"` is the
   safe-by-default transport and the loopback restriction in criterion 4 is what makes the claim
   true. Evidence for `"macos"`: `capability.schema.json`'s `platform` is an open illustrative
   string whose own description gives `'macos'` as the example, documented at
   `docs/executors.md:85-87`; `matching.match` ranks on kinds and the cost hint only
   (`docs/executors.md:104-120`), so adding it changes no selection outcome. The narrower
   Apple-Silicon-versus-Intel distinction is not representable in that single string and is
   deliberately not gated anywhere (criterion 9).

7. **`context_window` omitted rather than guessed** (criterion 7). Evidence: the field is optional
   (`capability.schema.json` requires only `spec_version` and `satisfies`), and there is no metadata
   endpoint to read it from. The Ollama adapter only populates it because `POST /api/show` exposes
   an architecture-prefixed `.context_length` key (`ollama.py:122-135`); inventing a per-model
   constant table here would be exactly the "name a specific model" coupling
   `docs/ontology.md:13-21` rules out.

8. **`mlx.py` is self-contained: the small loopback and HTTP-JSON helpers are written locally rather
   than imported from `ollama.py` or extracted into a shared module.** Evidence: no adapter in
   `src/praxis_executors/adapters/` imports any other — `claude_cli.py` and `codex_cli.py` each carry
   their own copies of closely similar subprocess/version-probe helpers, and every adapter's only
   non-stdlib import is `praxis_executors.interface`. Importing an underscore-prefixed private
   function across adapter modules would be a new pattern, and extracting a shared helper module
   would mean rewriting the shipped, 24-test-covered `ollama.py` inside a bundle scoped to adding one
   adapter. Flagged for the reviewer: this is a deliberate, evidence-backed duplication of roughly
   forty lines, not an oversight. Extracting a shared helper later is a clean, separately reviewable
   refactor; doing it here would widen this bundle's blast radius onto a file it otherwise never
   touches.

9. **No `praxis_cli` wiring** (see **Explicitly out of scope**). Evidence: `CodexCliExecutor` shipped
   and was not added to `_ADAPTER_FACTORIES` (`src/praxis_cli/adapters.py:31-46`);
   `tests/test_cli_adapters.py:26,38-41` pins the set at four specific ids; and
   `src/praxis_cli/fields.py:275-278` is written to degrade an unknown adapter class gracefully
   rather than require registration. Three independent signals agree, so this is the repository's
   convention, not an inference from one data point.

10. **`docs/executors.md` must be updated in this bundle, and the new doc test must not pin the
    adapter count** (criterion 11). Evidence:
    `tests/test_repair_findings_b1_issue41.py:1-18` records that skipping this exact doc update for
    Codex produced a post-merge repair issue, and lines 78-81 of that file explicitly warn against
    pinning the running adapter count or list order. This is the clearest available precedent that
    the doc edit is part of the adapter's definition of done, not follow-up work.

11. **The test command is `python3 -m pytest` from the worktree root** (criterion 12). Evidence:
    earlier bundle specs name `.venv/bin/python -m pytest`, but no `.venv` exists in this worktree
    (checked directly); `pyproject.toml:26` sets `pythonpath = ["src"]` and the system interpreter
    already imports `pytest` and `jsonschema`. Confirmed by running
    `python3 -m pytest tests/test_ollama_executor.py` → 23 passed, 1 skipped.

12. **The class is named `MlxExecutor`.** Evidence: `<Vendor>Executor` is the naming convention
    across all five shipped adapters (`OllamaExecutor`, `ClaudeCliExecutor`, `CodexCliExecutor`,
    `SubprocessExecutor`, `FakeCapabilityExecutor`). A class name is not an `executor_id`, so
    `docs/ontology.md:20-21`'s vendor-neutrality rule does not apply to it — that rule constrains the
    advertised identifier only, which criterion 3 keeps caller-supplied.

## Open questions

None.

One limitation is worth recording rather than escalating: GitHub issue #44's own body could not be
fetched in this environment (`gh` invocation is not permitted here), so this enhancement was built
from the bundle spec text reproduced verbatim above, the repository, and the run's sibling artifacts.
Every criterion above is anchored to a cited file, line range, or committed sibling spec, so a
reviewer with access to #44 can check each one against the original issue text directly. The two
substantive things the issue body might add beyond the bundle spec — a required endpoint and a
stated acceptance list — are both addressed as cited assumptions (4) and criteria 1-12 rather than
left implicit, and the sibling spec for the near-identical Ollama adapter
(`docs/develop/specs/b2-issue43.md`, issue #43, same issue template and same author) supplied the
shape the #44 body would most likely have taken. This passes the resolve-or-name test's
"something already on record gives a defensible default" condition, so it is an assumption to
correct, not a blocker to wait on.
