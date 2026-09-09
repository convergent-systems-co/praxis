# Bundle b2-issue43 — Enhanced Spec

## Original content

> # Bundle b2-issue43
>
> ## Issues
> - #43 — Ollama local executor adapter
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b2-issue43
> branch: develop/b2-issue43, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build.
>
> ## Existing architecture (read before writing code)
>
> - `src/praxis_executors/interface.py` — the `Executor` ABC to implement.
> - `src/praxis_executors/adapters/subprocess_executor.py` and `fake.py` —
>   existing adapter patterns (Ollama's local HTTP API may fit better than a
>   subprocess wrapper — use Python's stdlib `urllib`/`http.client`, no new
>   third-party HTTP dependency, unless one is already in `pyproject.toml`).
> - `src/praxis_contracts/schemas/v1/capability.schema.json` — the shape
>   `capabilities()` must return, including `auth_transport`/execution
>   properties from issue #39.
> - `src/praxis_executors/policy.py` — the `auth_transport` vocabulary this
>   adapter must use correctly (`"local"` for Ollama).
>
> ## Task
>
> Add `src/praxis_executors/adapters/ollama.py` implementing the `Executor`
> ABC against a local Ollama service — no external API required, zero
> network calls beyond localhost:
>
> - Discovery: service availability via a local HTTP health check against
>   Ollama's default local API (typically `http://127.0.0.1:11434`;
>   confirm the actual default port/endpoint from Ollama's own documented
>   API, e.g. `GET /api/tags` to list installed models), installed model
>   list, model metadata where available (parameter size, context length if
>   exposed), no assumption that a specific model is installed.
> - `capabilities()`: per-model capability advertisement (`coding`/`reasoning`
>   as applicable based on model name heuristics or Ollama's own model
>   metadata if it exposes a capability hint), `auth_transport: "local"`.
> - Launch/status/cancel/result via Ollama's local HTTP API (e.g. `/api/generate`
>   or `/api/chat`), converting Ollama's response into an `ExecutionResult`.
> - Must gracefully report `ExecutorAvailability.UNAVAILABLE` (not raise) when
>   Ollama isn't running, rather than erroring the whole discovery pass.
>
> ## Acceptance
>
> - Unit tests against a fake local HTTP server (stdlib `http.server` in a
>   background thread, or a mocked `urllib`/`http.client` call) — no real
>   Ollama dependency for the standard test suite.
> - If Ollama is actually running locally with a model installed, a
>   conditional real-inference smoke test is welcome but must be skipped
>   (not failed) otherwise.
> - Full test suite (`.venv/bin/python -m pytest`) passes.
>
> ## Delivery
>
> Open a PR against `main` referencing #43 with a closing keyword (`Closes
> #43`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **Full test suite passes** (unchanged from original) — `.venv/bin/python -m pytest`.

2. **`executor_id` is a constructor parameter, never hardcoded to reveal the vendor.**
   `OllamaExecutor.__init__` must accept `executor_id: str` from its caller, the same way
   `SubprocessExecutor.__init__` (`src/praxis_executors/adapters/subprocess_executor.py:29`) and
   `FakeCapabilityExecutor.__init__` (`src/praxis_executors/adapters/fake.py:33-35`) do — never
   default or hardcode it to a literal containing `"ollama"` or any model name. `docs/ontology.md`
   states as a core rule that `CapabilityAdvertisement.executor_id` "must not encode a vendor or
   model name" (`docs/ontology.md:20-21`); this is the adapter that must not violate it by baking
   `"ollama"` into the id it advertises.

3. **`base_url` is configurable and restricted to loopback addresses.**
   The constructor must accept a `base_url` parameter (default `http://127.0.0.1:11434`, Ollama's
   documented default per the original task) rather than hardcoding it, since the unit tests need
   to point the adapter at a fake server on an OS-assigned ephemeral port (see criterion 8). To keep
   the original task's own stated constraint — "no external API required, zero network calls beyond
   localhost" (spec line: "zero network calls beyond localhost") — enforced rather than aspirational,
   validate at construction time that `base_url`'s host resolves to a loopback address
   (`127.0.0.1`, `::1`, or `localhost`) and raise `ValueError` otherwise. This does not loosen any
   existing guarantee; it makes an already-stated constraint concrete and testable, and it is what
   makes advertising `auth_transport: "local"` (see `AuthTransportPolicy`,
   `src/praxis_executors/policy.py:22-25`, which treats `"local"` as safe-by-default, unlike
   `metered_api`/`api_key`) actually true of this adapter's runtime behavior, not just its label.

4. **`health()` uses all three `ExecutorAvailability` values, not just AVAILABLE/UNAVAILABLE.**
   - `UNAVAILABLE`: the local Ollama service is unreachable (connection refused/timeout) —
     the original task's explicit "must gracefully report `ExecutorAvailability.UNAVAILABLE` (not
     raise)" criterion.
   - `DEGRADED`: the service is reachable but `GET /api/tags` reports zero installed models. This
     matters because `capability-advertisement.schema.json`'s `capabilities` array has
     `"minItems": 1` (`capability-advertisement.schema.json:17-19`) — `capabilities()` cannot
     legally return zero capabilities, so a reachable-but-model-less Ollama cannot be `AVAILABLE`.
     `ExecutorAvailability.DEGRADED` (`interface.py:24-29`) exists in the ABC precisely for this
     kind of intermediate state and is otherwise unused by either existing adapter (both are always
     `AVAILABLE`, `subprocess_executor.py:48-51`, `fake.py:48-49`), so this is the first adapter
     that needs it.
   - `AVAILABLE`: reachable with at least one installed model.
   - Note `ExecutorRegistry.advertisements(healthy_only=True)` (the default) already skips calling
     `capabilities()` on any executor whose `health()` isn't `AVAILABLE` (`registry.py:53-63`), so
     the normal selection path never asks a `DEGRADED`/`UNAVAILABLE` `OllamaExecutor` for
     capabilities it can't supply. If `capabilities()` is called directly while the service is
     unreachable, it should raise `ExecutorError`, mirroring `SubprocessExecutor.launch`'s
     OSError→`ExecutorError` conversion (`subprocess_executor.py:64-65`) — callers are expected to
     check `health()` first, exactly as the registry does.

5. **Per-model capability advertisement shape.** `capabilities()` calls `GET /api/tags`, and emits
   one `Capability` entry per installed model:
   - `satisfies`: one or more `{"kind": ..., "parameters": {"model": <ollama model name>}}` entries.
     `kind` must come from the repository's existing "standard capability-kind vocabulary"
     (`docs/executors.md:56-71`: `coding`, `reasoning`, `planning`, `filesystem`, `shell`, `tools`,
     `vision`, `long-context`, `structured-output`, `repository-access`, `web`) — this is exactly
     where the original task's own "coding"/"reasoning" examples come from, not an invented pair.
     The model name goes in `satisfies[].parameters.model` (an open object,
     `capability.schema.json:28-31`) and/or the capability's optional `id` field — never in `kind`
     or `executor_id`, both of which `docs/ontology.md` restricts to vendor/model-neutral values
     (`docs/ontology.md:13-21`); `kind` is additionally pattern-constrained to
     `^[a-z0-9]+(-[a-z0-9]+)*$` (`capability.schema.json:24-26`), which a raw Ollama model name
     (e.g. `llama3:8b`) would not reliably satisfy.
   - Kind classification: prefer Ollama's own model metadata when it exposes a capability hint
     (e.g. a `capabilities` list from `POST /api/show`, when present, mapping entries that already
     match the standard vocabulary — such as `vision` or `tools` — directly); otherwise fall back to
     a name-based heuristic (e.g. models whose name contains `code`/`coder` advertise `coding`; every
     discovered model advertises `reasoning` as the generic baseline). The exact heuristic is an
     implementation detail with no acceptance test tied to a specific classification (the unit tests
     in criterion 8 run against a fake server with fixture data the implementer controls), so the
     planner should leave the precise substring/field logic to implementation rather than treat it as
     a fixed contract.
   - `auth_transport: "local"` on every emitted capability (unchanged from the original task).
   - `context_window`: best-effort only, from `POST /api/show`'s model metadata when present. The
     original task says "if exposed" — omitting it for a model where it isn't cheaply available is
     not an acceptance failure; the per-model extra `/api/show` round trip itself is optional, not
     mandatory (see **Explicitly out of scope**).

6. **Launch/status/cancel/result execution model.** Ollama's HTTP API is a blocking
   request/response cycle, unlike `subprocess.Popen` (which itself launches asynchronously). To
   preserve the ABC's launch-returns-a-handle-immediately/poll-until-terminal contract that
   `SubprocessExecutor` establishes (`interface.py:86-92`, `subprocess_executor.py:53-68`,
   `76-85`), `launch()` must start the blocking Ollama call on a background thread and return an
   `ExecutionHandle` immediately; `status()` reports `RUNNING` while that thread is alive and the
   terminal status once it completes; `result()` returns the completed `ExecutionResult` (raising
   `ExecutorError` if still running, matching `subprocess_executor.py:98-99`'s convention).
   - Use `POST /api/generate` with `"stream": false` as the required minimum endpoint (the original
     task's "e.g. `/api/generate` or `/api/chat`" is illustrative, not a dual-support mandate — see
     **Explicitly out of scope**). Non-streaming keeps the fake test server and the response parsing
     both a single JSON object, with no NDJSON chunk-parsing needed.
   - `request.parameters` must carry `model` (str, required — which installed model to invoke,
     matching the value advertised in that model's `satisfies[].parameters.model`, criterion 5) and
     `prompt` (str, required), mirroring `SubprocessExecutor`'s single-well-known-key convention for
     `request.parameters["command"]` (`subprocess_executor.py:54-56`). Missing either key raises
     `ExecutorError`, matching `subprocess_executor.py:54-55`'s pattern exactly.
   - `cancel()` closes the in-flight HTTP connection for that handle (the client-side mechanism
     Ollama's API relies on to stop an in-progress generation), then marks the handle `CANCELLED` —
     the HTTP-transport equivalent of `SubprocessExecutor.cancel`'s `process.terminate()`
     (`subprocess_executor.py:87-91`). A cancellation that races a response that has already fully
     arrived may still surface as `SUCCEEDED`; this best-effort behavior is acceptable (see
     **Explicitly out of scope**).
   - HTTP client: use `urllib.request`/`urllib.error` from the stdlib, not `http.client` — this
     matches the one existing precedent for stdlib HTTP calls in this repository,
     `praxis_dashboard`'s test suite (`tests/test_dashboard_server.py:22-24`, `34-39`), keeping a
     single stdlib HTTP idiom in the codebase rather than introducing a second one. (The original
     task offers `urllib`/`http.client` as alternatives; this resolves which one, it doesn't
     contradict the task.)
   - Apply a short timeout (a few seconds) to discovery/health/capabilities calls so an unreachable
     or hung service fails fast rather than blocking `health()`/`capabilities()` indefinitely; a
     longer or caller-configurable timeout applies to `/api/generate` calls, since real inference can
     legitimately take much longer than a health check.

7. **Test suite structure.** Follow `tests/test_dashboard_server.py`'s established pattern
   (`tests/test_dashboard_server.py:1-17`) for the fake local HTTP server: a `ThreadingHTTPServer`
   bound to `port=0` (OS-assigned ephemeral port) driven by a background `.serve_forever()` thread,
   torn down with `.shutdown()`. Point `OllamaExecutor(base_url=...)` (criterion 3) at that server's
   assigned port. Add `tests/test_ollama_executor.py`, mirroring
   `tests/test_subprocess_executor.py`'s structure (one test per lifecycle behavior, plus a
   `test_capabilities_advertisement_validates_against_schema` test in the same shape as
   `test_subprocess_executor.py:95-100`). The conditional real-Ollama smoke test (original task,
   unchanged) is a separate, explicitly skippable test, not part of the fake-server suite.

## Explicitly out of scope

- `POST /api/chat` support. `/api/generate` (criterion 6) is the required minimum; the original
  task's "e.g. `/api/generate` or `/api/chat`" phrasing does not require both, and adding
  multi-turn chat-history handling is a separate increment that can build on this adapter later.
- Any capability execution-property field beyond `auth_transport` — `interactive`, `platform`,
  `availability` (`capability.schema.json:41-57`) are optional and not mentioned by the original
  task; `context_window` is included only best-effort (criterion 5).
- The extra per-model `POST /api/show` round trip for `context_window`/family metadata is optional,
  not mandatory — a model list assembled from `GET /api/tags` alone (name, size, `parameter_size`,
  `quantization_level`) satisfies the original task's discovery requirement; adding the second call
  per model is an implementation choice, not an acceptance criterion.
- Non-loopback / remote Ollama support of any kind (criterion 3) — the original task states "zero
  network calls beyond localhost" as a hard constraint, not a default to be overridden.
- Guaranteed, race-free mid-generation cancellation across all Ollama versions — best-effort via
  closing the client connection (criterion 6) is sufficient; a response that completes before the
  cancel takes effect surfacing as `SUCCEEDED` instead of `CANCELLED` is acceptable.
- Registering an `OllamaExecutor` instance anywhere in application/composition-root code (e.g. a
  default `ExecutorRegistry` setup). No such wiring exists today for `SubprocessExecutor` or
  `FakeCapabilityExecutor` either (confirmed by repo-wide search — both are only constructed inside
  their own test files); per `docs/executors.md`'s "Adding a new executor adapter" section
  (`docs/executors.md:237-257`), constructing and registering an instance is explicitly "the
  caller"'s job, not the adapter bundle's.
- Any change to `src/praxis_executors/interface.py`, `matching.py`, `policy.py`, or any
  `schemas/v1/*.schema.json` file — this bundle adds one new adapter module and its tests only.

## Assumptions made

- **`executor_id` via constructor parameter, never hardcoded** (criterion 2). Evidence:
  `docs/ontology.md:20-21`'s explicit rule, and both existing adapters already follow this pattern
  (`subprocess_executor.py:29`, `fake.py:33-35`). Resolve-or-name test: in scope, defensible default
  from two sibling adapters plus an explicit documented rule, doesn't change any acceptance
  criterion's meaning, no security/compat impact, trivially checkable.

- **`base_url` configurable, defaulting to `http://127.0.0.1:11434`, validated as loopback-only**
  (criterion 3). Evidence: the original task's own "zero network calls beyond localhost" constraint
  (spec Task section) and Ollama's documented default address (spec's own parenthetical). This
  fill *tightens* an already-stated constraint into a testable one rather than introducing a new
  security decision — consistent with the rubric's worked example of resolving "add rate limiting"
  by applying an existing default (a fill that only adds a restriction, never loosens one).

- **`health()` reports `DEGRADED` for a reachable-but-model-less Ollama, `AVAILABLE` otherwise,
  `UNAVAILABLE` when unreachable** (criterion 4). Evidence: `capability-advertisement.schema.json`'s
  `capabilities` array requires `minItems: 1` (`capability-advertisement.schema.json:17-19`), so
  `AVAILABLE` (which the registry uses to decide whether to call `capabilities()` at all,
  `registry.py:53-63`) cannot be reported when there is nothing valid to advertise; `DEGRADED`
  already exists in `ExecutorAvailability` for exactly this kind of intermediate state
  (`interface.py:24-29`) and is otherwise unused by either shipped adapter.

- **Per-model capability shape: `kind` from the existing standard vocabulary, model name in
  `satisfies[].parameters.model`** (criterion 5). Evidence: `docs/executors.md:56-71`'s documented
  vocabulary list (which is where the original task's "coding"/"reasoning" examples come from),
  `docs/ontology.md:13-21`'s rule that `kind`/`executor_id` must stay vendor/model-neutral, and
  `capability.schema.json:28-31`'s already-open `parameters` object (the same mechanism
  `docs/executors.md:162-171` documents being used for the unrelated `cost`/`risk`/`latency` ranking
  hint, i.e. an established convention for carrying adapter-specific data in `parameters` without a
  schema change).

- **`/api/generate` with `"stream": false` as the one required endpoint; background-thread execution
  model for `launch`/`status`/`cancel`/`result`; `urllib.request`/`urllib.error` as the HTTP client**
  (criterion 6). Evidence: the original task's "e.g." phrasing leaves the endpoint choice open;
  non-streaming keeps both the fake server and the response parser single-JSON-object simple. The
  background-thread design is the structural analogue of `SubprocessExecutor`'s already-async
  `subprocess.Popen` + `poll()` pattern (`subprocess_executor.py:53-68`, `76-85`), needed because an
  HTTP POST is otherwise synchronous and would violate `Executor.launch`'s
  return-a-handle-for-later-polling contract (`interface.py:86-92`). `urllib` over `http.client`
  matches the one existing stdlib-HTTP precedent in this repo (`tests/test_dashboard_server.py:22-24`).

- **Fake-server test pattern: `ThreadingHTTPServer` on `port=0` plus a background `.serve_forever()`
  thread** (criterion 7). Evidence: this is the exact, already-used pattern for driving a real HTTP
  server in this repo's tests (`tests/test_dashboard_server.py:1-17`), avoiding inventing a second
  approach when the original task itself suggests "stdlib `http.server` in a background thread" as
  one of its two named options.

## Open questions

None. Every gap found passed the resolve-or-name test against evidence already in the repository
(the `Executor` ABC and its two existing adapters, `docs/ontology.md`/`docs/executors.md`'s
documented vocabulary and conventions, the `capability`/`capability-advertisement` schemas, the
`AuthTransportPolicy` safe-by-default rules, `ExecutorRegistry`'s health-gating behavior, the
existing stdlib-HTTP test precedent in `praxis_dashboard`, and the original spec's own stated
"zero network calls beyond localhost" constraint) or against Ollama's own documented local HTTP API
(default address/port, `/api/tags`, `/api/generate`, `/api/show`) as the original task explicitly
directs.
