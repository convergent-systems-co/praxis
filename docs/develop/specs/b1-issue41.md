# Enhanced spec: b1-issue41

## Original content

> # Bundle b1-issue41
>
> ## Issues
> - #41 — Codex CLI executor adapter
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b1-issue41
> branch: develop/b1-issue41, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build.
>
> ## Existing architecture (read before writing code)
>
> - `src/praxis_executors/adapters/claude_cli.py` — the just-merged sibling
>   adapter this one should mirror in structure (subprocess-based CLI
>   wrapper, `auth_transport: "subscription_cli"`), but **learn from its
>   audited gaps, don't repeat them**:
>   - Its auth-detection was left stubbed when a safe probe actually existed
>     — for Codex, actually investigate whether `codex` has an equivalent
>     safe, non-destructive, sub-second auth-status command (check
>     `codex --help`, `codex auth --help` or similar) before deciding
>     detection isn't possible. If one exists, wire it in from the start.
>   - Its `launch()` didn't isolate the subprocess environment from
>     ambient `ANTHROPIC_API_KEY`-style variables, risking a silent metered
>     fallback. For Codex, identify OpenAI's equivalent env vars
>     (`OPENAI_API_KEY` and any others `codex --help` documents) and strip
>     them from the subprocess's environment in `launch()` from the start.
>   - Its credential-redaction regex only matched one credential shape and
>     one of its tests was structurally vacuous (asserted against a field
>     that could never contain the redacted text). Make sure this adapter's
>     redaction actually covers the credential/token shape Codex's CLI uses,
>     and that tests exercise fields that can actually contain the redacted
>     text.
> - `src/praxis_executors/interface.py` — the `Executor` ABC.
> - `src/praxis_contracts/schemas/v1/capability.schema.json` — the shape
>   `capabilities()` must return.
> - `src/praxis_executors/policy.py` — the `auth_transport` vocabulary.
>
> ## Task
>
> Add `src/praxis_executors/adapters/codex_cli.py` implementing the
> `Executor` ABC against the authenticated OpenAI Codex CLI:
>
> - Discovery: executable availability, version, supported models/modes
>   where exposed, filesystem/tool capability detection.
> - Authentication detection: safely detectable only (see the note above —
>   actually check for a real safe probe first), never extracting
>   credentials directly.
> - `capabilities()`: `coding`, `shell`, `filesystem` as applicable,
>   `auth_transport: "subscription_cli"`.
> - Subprocess-based launch/status/cancel/result, with the subprocess
>   environment isolated from metered-API env vars from the start (see
>   above).
> - Never fall back to a metered API key if the CLI reports unauthenticated.
>
> ## Acceptance
>
> - Unit tests using a fake/stubbed subprocess — no real Codex CLI
>   dependency for the standard test suite.
> - If the real `codex` CLI happens to be installed/authenticated on this
>   machine, a minimal non-destructive smoke test is welcome but must be
>   skipped (not failed) otherwise.
> - A test confirming the subprocess environment strips the identified
>   metered-API env vars even when they're set in the parent process.
> - A test confirming `health()` can report `AVAILABLE` via whatever safe
>   auth-detection mechanism was found (or explicitly documents why none
>   exists, if that's genuinely the case after investigation).
> - Full test suite (`.venv/bin/python -m pytest`) passes.
> - No credential is ever logged, printed, or included in any evidence/error
>   message this adapter produces.
>
> ## Delivery
>
> Open a PR against `main` referencing #41 with a closing keyword (`Closes
> #41`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **Schema/interface references are already accurate.** Unlike b1-issue40's
   enhancement (which had to correct a stale schema path), every path this
   spec cites — `src/praxis_executors/adapters/claude_cli.py`,
   `src/praxis_executors/interface.py`,
   `src/praxis_contracts/schemas/v1/capability.schema.json`,
   `src/praxis_executors/policy.py` — exists exactly as named in this
   worktree; confirmed by direct read. No correction needed.
2. **Schema test pattern.** Mirror `tests/test_claude_cli.py::test_capabilities_advertisement_validates_against_schema`:
   assert `capabilities()`'s return value validates against
   `capability-advertisement.schema.json` via
   `praxis_contracts.validator.validate_document` with
   `praxis_contracts.schema_paths.SCHEMA_DIR`.
3. **`capabilities()` shape: one `Capability` entry, all three kinds, unconditionally.**
   "`coding`, `shell`, `filesystem` as applicable" resolves to: a single
   `Capability` object whose `satisfies` list contains all three
   (`{"kind": "coding"}`, `{"kind": "shell"}`, `{"kind": "filesystem"}`) and
   whose single `auth_transport` is `"subscription_cli"` — the same
   one-entry, `auth_transport`-is-Capability-level-not-per-kind structure
   `claude_cli.py` already uses (see Assumption 1). Drop "as applicable" as
   a live condition: it is not re-evaluated at runtime against the
   installed CLI's actual feature set; if investigation genuinely finds
   Codex cannot do one of the three, that's a planner-level scope
   correction, not a runtime branch in `capabilities()`.
4. **`auth_transport` is static advertisement metadata, not live-synced to
   `health()`.** `capabilities()` always advertises
   `auth_transport: "subscription_cli"`; only `health()`'s
   `ExecutorAvailability` varies with live detection (see Assumption 1) —
   same split `claude_cli.py` already implements and tests.
5. **Auth-state → `ExecutorAvailability` mapping.** Confirmed authenticated
   → `AVAILABLE`; confirmed unauthenticated → `UNAVAILABLE` (the "never
   fall back" guarantee, already explicit in the original spec); state
   undeterminable (no safe probe available, or the probe's result is
   ambiguous) → `DEGRADED` — the same three-way mapping `claude_cli.py`
   already implements and tests (`tests/test_claude_cli.py`'s
   `test_health_available_when_cli_present_and_authenticated`,
   `..._unavailable_when_cli_present_and_not_authenticated`,
   `..._degraded_when_authentication_unknown`), now with in-repo test
   evidence rather than only convention (see Assumption 1).
6. **Auth-probe investigation is implementer-owned work, correctly scoped
   already — not pre-resolved here.** The original spec already names the
   concrete commands to check (`codex --help`, `codex auth --help` "or
   similar") and gives the fallback ("explicitly documents why none
   exists"), so this isn't spec ambiguity to fill. For the record: `codex`
   is installed in this environment (`which codex` →
   `/opt/homebrew/bin/codex`, resolving to
   `/opt/homebrew/Caskroom/codex/0.153.4/bin/codex`), but this session's
   sandbox blocked every attempt to run or inspect it (`codex --help`,
   `codex --version`, `file <path>`, `brew list --versions codex` were all
   denied as outside this session's allowed working directories), so no
   command name is asserted here — the implementer's own session must run
   the real investigation against the actual installed binary.
7. **Smoke-test guard.** Mirror `claude_cli`'s
   `@pytest.mark.skipif(shutil.which("claude") is None, ...)` pattern,
   substituting `"codex"`. `codex` happening to be installed on this
   machine (previous item) makes the smoke test's present-branch concretely
   exercisable here, not just a hypothetical CI path.
8. **Env-var isolation, concretely.** `launch()` must construct the
   subprocess's `env` explicitly (`os.environ` copied, then the identified
   keys popped) rather than inheriting the parent environment unfiltered —
   `claude_cli.py`'s `launch()` passes no `env` to `subprocess.Popen` at all
   (inherits everything unfiltered), which is precisely the audited gap
   this bundle must not repeat. At minimum, `OPENAI_API_KEY` must be
   stripped (see Assumption 2); any further OpenAI env vars `codex --help`
   documents (org/project scoping, base-URL overrides, etc.) are the same
   implementer-owned investigation as item 6, not pre-enumerated here.
9. **Credential redaction, concretely.** The redaction pattern must not
   be a single literal-prefix regex the way `claude_cli._CREDENTIAL_PATTERN`
   (`sk-ant-[A-Za-z0-9_-]{10,}`) only matches one shape — it must cover the
   OpenAI API key family generally (legacy `sk-...` and project-scoped
   `sk-proj-...` both start `sk-` followed by an alphanumeric/`_`/`-` run;
   see Assumption 3), and every redaction test must assert against a field
   that can actually hold the redacted text (`ExecutionResult.payload`'s
   `stdout`/`stderr`, and the message of an `ExecutorError` raised from a
   launch failure) — mirroring `test_result_redacts_credential_shaped_secret_from_evidence_and_payload`
   and `test_launch_failure_redacts_credential_shaped_secret_from_error_message`
   in `tests/test_claude_cli.py`, which already exercise real fields (the
   "vacuous test" gap the original spec warns about is not present in
   either of those two, current tests — the gap to avoid is regressing
   behind that bar in the new adapter's tests, not copying a bug forward).
10. **`launch()` required parameter and unknown-handle behavior.** Mirror
    `claude_cli.launch()`'s `if "prompt" not in request.parameters: raise
    ExecutorError(...)` guard, and `_process_for`'s `ExecutorError` on an
    unknown `handle_id` (exercised identically by `status`, `cancel`, and
    `result`) — both already implicit in "mirror in structure" but not
    spelled out as their own Acceptance bullets in the original spec.
11. **Exact CLI argv construction is a planner/implementer decision, not
    named here.** The original spec does not name Codex's non-interactive
    invocation syntax, and no existing adapter needs to translate a
    `Promise`/`ExecutionRequest` into Codex's specific CLI syntax yet.
    Whether the `prompt` parameter maps to `codex exec <prompt>` or another
    form is left to whoever implements this, consistent with `claude_cli`'s
    own precedent (`launch()` maps `request.parameters["prompt"]` plus an
    optional `extra_args` list onto argv) and subject to the same
    build-time investigation as items 6 and 8.

## Explicitly out of scope

- Registering the adapter with any `ExecutorRegistry` instance, choosing a
  production `executor_id`, or wiring it into any config/deployment path —
  `executor_id` remains a constructor parameter the caller supplies, exactly
  like `ClaudeCliExecutor`/`SubprocessExecutor`/`FakeCapabilityExecutor`
  (see Assumption 4).
- Reading, parsing, or otherwise inspecting Codex's stored OAuth token,
  API-key config file, or keychain entry directly — the original spec's
  "never extracting credentials directly" restated as an explicit boundary,
  not just a task instruction.
- Any change to `capability.schema.json`, `capability-advertisement.schema.json`,
  `docs/executors.md`'s vocabulary, or `src/praxis_executors/policy.py`'s
  `AuthTransportPolicy` — this bundle consumes the existing vocabulary, it
  does not extend it.
- Retrofitting either audited gap (env isolation, redaction coverage) onto
  the already-merged `claude_cli.py` itself. This bundle's job is to not
  repeat those gaps in the new adapter; fixing them in the existing one is
  a separate, un-requested change to a different file.
- Falling back to `metered_api`/`api_key` auth, or to any other executor,
  when the CLI is unauthenticated or its auth state is undeterminable — the
  original spec's "never fall back" already forbids this; restated so it
  isn't reintroduced later as a "helpful" retry path.
- Any real, unmocked invocation of the `codex` CLI in the standard test
  suite — the only real-CLI touchpoint allowed is the optional smoke test,
  `skipif`-guarded and non-destructive, per the original Acceptance section.
- Enumerating every OpenAI-side env var beyond `OPENAI_API_KEY` up front —
  covered by the same build-time investigation as auth-probe discovery
  (item 6/8 above), not something this enhancement pre-decides.

## Assumptions made

1. **`capabilities()` structure: one `Capability`, `auth_transport` at the
   `Capability` level, static (not live-synced to `health()`), and
   undeterminable auth state maps to `DEGRADED`.** Evidence:
   `src/praxis_contracts/schemas/v1/capability.schema.json` declares
   `auth_transport` as a top-level property alongside `satisfies` (not
   nested per-kind); `docs/executors.md`'s "Capability execution-property
   fields" section states execution-property fields (including
   `auth_transport`) are "distinct from and not synchronized with"
   `health()`'s live signal; and `src/praxis_executors/adapters/claude_cli.py`
   (merged, in this worktree) already implements exactly this shape —
   `capabilities()` always returns `auth_transport: "subscription_cli"`
   regardless of auth state, and `health()` maps confirmed-authenticated /
   confirmed-unauthenticated / unknown to `AVAILABLE` / `UNAVAILABLE` /
   `DEGRADED` respectively, with passing tests for each
   (`tests/test_claude_cli.py`). This is now a demonstrated in-repo sibling
   convention, not just an inferred one.
2. **`OPENAI_API_KEY` is the one certain env var to strip in `launch()`.**
   Evidence: the original spec names it explicitly ("identify OpenAI's
   equivalent env vars (`OPENAI_API_KEY` and any others `codex --help`
   documents)") and it is OpenAI's well-established, publicly documented
   SDK/CLI env var for metered API-key auth — the direct structural
   equivalent of `ANTHROPIC_API_KEY`, the variable the audited gap names
   for the Claude sibling. Any additional org/project-scoping or base-URL
   env vars Codex's CLI documents are explicitly left to build-time
   investigation (see Clarified acceptance criteria item 8), not asserted
   here, since I could not execute `codex --help` in this session to
   confirm the full list (see item 6).
3. **Redaction must match the OpenAI API key family generally, not one
   literal prefix.** Evidence: both publicly-documented OpenAI key shapes
   (legacy `sk-...`, project-scoped `sk-proj-...`) share the `sk-` prefix
   followed by an alphanumeric/`_`/`-` run, so a single pattern such as
   `sk-[A-Za-z0-9_-]{20,}` covers both without needing to enumerate every
   shape by name — directly addressing the original spec's stated gap
   ("its credential-redaction regex only matched one credential shape").
   The exact minimum-length threshold is an implementation detail that
   doesn't change what the acceptance test verifies (a fake, credential-shaped
   secret must not survive into `payload`/`evidence`/error messages).
4. **`executor_id` is a constructor parameter, not hardcoded.** Evidence:
   every existing adapter (`ClaudeCliExecutor`, `SubprocessExecutor`,
   `FakeCapabilityExecutor`) takes `executor_id: str` at construction, and
   their tests supply distinct string ids (e.g.
   `executor_id="executor-claude-cli-1"`). This also keeps the adapter
   consistent with `capability-advertisement.schema.json`'s note that
   `executor_id` "must not encode a vendor or model name" — the value
   remains the registering caller's choice, made outside this bundle (see
   Explicitly out of scope).

## Open questions

None. The one item that looked like it might need escalation — the exact
Codex auth-status probe command — turned out not to be spec ambiguity: the
original spec already names the commands to check, already supplies a
documented fallback if no safe probe exists, and already makes both
outcomes independently testable. It's implementer-owned investigation
work the spec correctly scoped already, not a gap for this pass to fill or
escalate (see Clarified acceptance criteria item 6 for what was actually
attempted and why it couldn't be completed in this session).
