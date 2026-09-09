# Enhanced spec: b1-issue40

## Original content

> # Bundle b1-issue40
>
> ## Issues
> - #40 — Claude CLI executor adapter
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b1-issue40
> branch: develop/b1-issue40, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build.
>
> ## Existing architecture (read before writing code)
>
> - `src/praxis_executors/interface.py` — the `Executor` ABC to implement
>   (`capabilities`, `health`, `launch`, `status`, `cancel`, `result`).
> - `src/praxis_executors/adapters/subprocess_executor.py` — the existing
>   subprocess-launching pattern to follow.
> - `src/praxis_executors/adapters/fake.py` — a minimal reference adapter.
> - `schemas/v1/capability-advertisement.schema.json` and
>   `src/praxis_contracts/schemas/v1/capability.schema.json` (now at this new
>   location, per issue #52) — the shape `capabilities()` must return,
>   including the `auth_transport`/execution-property fields issue #39 added.
> - `src/praxis_executors/policy.py` — `AuthTransportPolicy` and the
>   `auth_transport` vocabulary (`subscription_cli`, `oauth_cli`, `local`,
>   `metered_api`, `api_key`) this adapter must advertise correctly against.
>
> ## Task
>
> Add `src/praxis_executors/adapters/claude_cli.py` implementing the
> `Executor` ABC against the authenticated Claude Code CLI:
>
> - Discovery: executable availability (e.g. `shutil.which("claude")`),
>   version (`claude --version`), supported execution modes, available
>   models where the CLI exposes them — all via safe, non-destructive CLI
>   introspection, never by reading credential files directly.
> - Authentication detection: safely detectable only (a bounded,
>   side-effect-free CLI call that reports auth state, if one exists) —
>   never extract, copy, or scrape the CLI's stored OAuth token. If safe
>   detection isn't possible, report `unknown`, not a guess.
> - `capabilities()`: a real `CapabilityAdvertisement`-shaped dict advertising
>   `coding`, `reasoning`, `tools`, `filesystem` as applicable, with
>   `auth_transport: "subscription_cli"`.
> - `launch`/`status`/`cancel`/`result`: subprocess-based execution (follow
>   `subprocess_executor.py`'s pattern), converting the CLI's output into an
>   `ExecutionResult` per `interface.py`'s evidence/payload shape.
> - Never fall back to a metered API key if the CLI reports unauthenticated —
>   report `ExecutorAvailability.UNAVAILABLE` instead.
>
> ## Acceptance
>
> - Unit tests using a fake/stubbed subprocess (`unittest.mock` or a fake
>   `subprocess.run`/`Popen`) — no real Claude CLI dependency for the
>   standard test suite.
> - If the real `claude` CLI happens to be installed/authenticated on this
>   machine, a minimal non-destructive smoke test is welcome but must be
>   skipped (not failed) when the CLI isn't present — use
>   `pytest.mark.skipif(shutil.which("claude") is None, ...)` or equivalent.
> - Full test suite (`.venv/bin/python -m pytest`) passes.
> - No credential is ever logged, printed, or included in any evidence/error
>   message this adapter produces.
>
> ## Delivery
>
> Open a PR against `main` referencing #40 with a closing keyword (`Closes
> #40`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **Schema test pattern.** Mirror `tests/test_subprocess_executor.py::test_capabilities_advertisement_validates_against_schema`
   (and `test_fake_capability_executor.py`'s equivalent): assert `capabilities()`'s
   return value validates against `capability-advertisement.schema.json` via
   `praxis_contracts.validator.validate_document` together with
   `praxis_contracts.schema_paths.SCHEMA_DIR`/`schema_path`. This is the concrete,
   already-established form of "a real `CapabilityAdvertisement`-shaped dict."
2. **Corrected schema path (see Assumption 1).** `capability-advertisement.schema.json`
   is not at the original spec's `schemas/v1/capability-advertisement.schema.json` —
   that top-level `schemas/` directory no longer exists. Both it and
   `capability.schema.json` now live under `src/praxis_contracts/schemas/v1/`,
   reached via `praxis_contracts.schema_paths`.
3. **`auth_transport` placement.** It is a field on the `Capability` object
   (sibling to `satisfies`), not per-`kind`. One `Capability` entry with
   `satisfies: [{"kind": "coding"}, {"kind": "reasoning"}, {"kind": "tools"}, {"kind": "filesystem"}]`
   and a single `auth_transport: "subscription_cli"` on that same entry satisfies
   "advertising `coding`, `reasoning`, `tools`, `filesystem` ... with
   `auth_transport: "subscription_cli"`" (see Assumption 2).
4. **`auth_transport` is static, not live-synced.** `capabilities()` always
   advertises `auth_transport: "subscription_cli"` regardless of the
   currently-detected auth state; only `health()`'s `ExecutorAvailability`
   varies with live detection (see Assumption 3).
5. **"Unknown" auth state maps to `ExecutorAvailability.DEGRADED`.** Distinct
   from the confirmed-unauthenticated case, which the original spec already
   pins to `UNAVAILABLE` (see Assumption 4).
6. **Discovery of "supported execution modes" / "available models" is
   best-effort, unshaped metadata** — not required in any specific field and
   not covered by any Acceptance-section test. Only executable presence
   (`shutil.which("claude")`) and version (`claude --version`) are concretely
   named by the original spec and must be exercised by a mocked-subprocess
   test (see Assumption 5).
7. **Credential-safety test, concretely.** At least one test must simulate a
   subprocess whose stdout/stderr contains a fake, credential-shaped string
   and assert that string never appears verbatim in the resulting
   `ExecutionResult.evidence`, `ExecutionResult.payload`, or in the message of
   any `ExecutorError` the adapter raises on that path — this is the testable
   form of "no credential is ever logged, printed, or included in any
   evidence/error message."
8. **`launch`/`status`/`cancel`/`result` lifecycle shape.** Follow
   `subprocess_executor.py`'s `Popen`-based lifecycle (`poll()`,
   `communicate()`, a `uuid.uuid4().hex` handle id, `ExecutorError` on an
   unknown handle), as the original spec already directs. The exact mapping
   from `ExecutionRequest.promise`/`.parameters` to the `claude` CLI's actual
   argv is an implementation decision left to the planner — no existing
   adapter needs to translate a `Promise` into a different backend's CLI
   syntax yet, and the original spec does not name one.

## Explicitly out of scope

- Registering the adapter with any `ExecutorRegistry` instance, choosing a
  production `executor_id`, or wiring it into any config/deployment path.
  The Task names only adding the adapter file; `executor_id` remains a
  constructor parameter the caller supplies, exactly like
  `SubprocessExecutor`/`FakeCapabilityExecutor` (see Assumption 6).
- Reading, parsing, or otherwise inspecting the CLI's stored OAuth token,
  config file, or keychain entry directly, by any means — explicitly
  forbidden by the original spec ("never extract, copy, or scrape") and
  restated here as a boundary, not just a task instruction.
- Any change to `capability.schema.json`, `capability-advertisement.schema.json`,
  `docs/executors.md`'s vocabulary, or `src/praxis_executors/policy.py`'s
  `AuthTransportPolicy` — this bundle consumes the existing vocabulary/policy
  from issues #38/#39; it does not extend either.
- Falling back to `metered_api`/`api_key` auth, or to any other executor,
  when the CLI is unauthenticated or its auth state is undeterminable — the
  original spec's "report `UNAVAILABLE` instead" already forbids this;
  restated so it isn't reintroduced later as a "helpful" retry path.
- Any real, unmocked invocation of the `claude` CLI in the standard test
  suite — the only real-CLI touchpoint allowed is the optional smoke test,
  `skipif`-guarded and non-destructive, per the original Acceptance section.

## Assumptions made

1. **Schema path correction.** Evidence: `find` over this worktree shows no
   top-level `schemas/` directory; `capability-advertisement.schema.json`
   exists only at `src/praxis_contracts/schemas/v1/capability-advertisement.schema.json`.
   Git history (`c324cbb feat(b2-issue52): Repoint 13 test files'
   independently-computed schema d[ata]`, `df0185f`/`49e29e2`/etc. "Migrate
   ... schema path[s] to schema_paths helper") confirms this file moved in
   issue #52's schema relocation, the same relocation the original spec
   already notes for `capability.schema.json` but didn't apply to this
   sibling path.
2. **`auth_transport` is a `Capability`-level field.**
   Evidence: `src/praxis_contracts/schemas/v1/capability.schema.json`
   declares `auth_transport` as a top-level property alongside `satisfies`,
   not inside `satisfies[]`'s item schema (which only allows `kind`/
   `parameters`). One `Capability` entry can carry multiple `satisfies`
   kinds under a single `auth_transport`.
3. **`auth_transport`/execution-property fields are static advertisement
   metadata, not live-synced to `health()`.** Evidence:
   `capability.schema.json`'s own `availability` field description states
   this pattern explicitly — "Distinct from and not synchronized with
   `Executor.health()`'s live `ExecutorAvailability` signal" — and
   `docs/executors.md`'s "Capability execution-property fields" section
   repeats it. The same executor-advertises-statically /
   health-reports-live split applies to `auth_transport`: it describes how
   the executor *would* authenticate, not whether it currently can.
4. **"Unknown" auth state → `ExecutorAvailability.DEGRADED`.** Evidence:
   `src/praxis_executors/interface.py`'s `ExecutorAvailability` has exactly
   three members (`AVAILABLE`, `DEGRADED`, `UNAVAILABLE`) — there is no
   fourth "unknown" value to report verbatim, so the original spec's
   "report `unknown`" must map onto one of the three. `DEGRADED` is the only
   one that doesn't assert a confirmed fact in either direction. This
   doesn't loosen the "never fall back on unauthenticated" guarantee:
   `docs/executors.md`'s "Health and availability" section states
   `ExecutorRegistry.advertisements(healthy_only=True)` (the default)
   already excludes both `DEGRADED` and `UNAVAILABLE` identically, so the
   distinction only matters for `healthy_only=False` diagnostics, not for
   default selection behavior.
5. **Execution-mode/model discovery is best-effort.** Evidence: no file,
   doc, or test in this repository names a specific `claude` CLI flag or
   output shape for "supported execution modes" or "available models" (only
   `shutil.which`/`claude --version` are named concretely, by the spec
   itself), and none of the original spec's four Acceptance bullets tests
   for either. Inventing a specific undocumented flag would fail the
   resolve-or-name test's "defensible default already on record" condition;
   treating it as optional, unshaped metadata does not.
6. **`executor_id` is a constructor parameter, not hardcoded.** Evidence:
   both existing adapters (`SubprocessExecutor.__init__(self, executor_id:
   str, ...)`, `FakeCapabilityExecutor.__init__(self, executor_id: str,
   ...)`) and their tests (`executor_id="executor-subprocess-1"`,
   `executor_id="executor-fake-1"`) take it as a caller-supplied string, not
   an internal constant. This also avoids any tension with
   `capability-advertisement.schema.json`'s "`executor_id` ... must not
   encode a vendor or model name" note: the value is the registering
   caller's choice, made outside this bundle (see Explicitly out of scope).

## Open questions

None — every gap found passed the resolve-or-name test given the evidence above.
