# Enhanced spec: b-issue42

## Original content

> # Bundle b-issue42: GitHub Copilot executor adapter
>
> ## Issues
> - #42 — GitHub Copilot executor adapter
>
> ## Scope
> First investigate what GitHub Copilot CLI/tooling is actually currently supported and
> authenticatable (this surface has evolved; do not assume a specific tool without
> checking). Then add `src/praxis_executors/adapters/copilot_cli.py` implementing the
> `Executor` ABC against the strongest currently-supported subscription-backed
> integration, following the shape of the existing Claude/Codex CLI adapters
> (`src/praxis_executors/adapters/claude_cli.py`).
>
> ## Base
> `origin/main` at commit 9366f5b (praxis CLI scaffold + executors discover/executors/match merged).

The base commit is confirmed: this worktree's HEAD is `9366f5b`, working tree clean.

## Clarified acceptance criteria

1. **The investigation is a deliverable, not a preamble.** The chosen Copilot
   surface, the evidence for choosing it, and what was rejected must be recorded
   in the new module's docstring/comments, in the same style
   `codex_cli.py` records its own live findings ("Verified against the real codex
   0.153.4: …", "Investigated live: `codex --help` documents no `auth`
   subcommand, but `codex login status` is a safe ~20ms probe…"). A reviewer must
   be able to tell what was verified against a real binary from what was
   inferred.
   The decision rule for "strongest currently-supported subscription-backed
   integration": pick the surface that (a) can be driven non-interactively from a
   script, (b) authenticates off an existing Copilot subscription login, and (c)
   does not require a metered/API-key credential to function. If two surfaces
   qualify, prefer the one that can actually do coding work over one that only
   emits shell-command suggestions.
   Two candidate surfaces exist as *leads to verify live*, not as established
   facts (see Assumption 6 — this enhancement session could not execute anything
   outside the worktree): the standalone agentic `copilot` CLI, and the older
   `gh copilot` extension to the `gh` CLI. Neither name, nor any flag, is
   asserted here. Verify what is installed and what its own `--help` documents
   before writing argv.
2. **Module and class shape.** `src/praxis_executors/adapters/copilot_cli.py`
   defines `CopilotCliExecutor(Executor)` with `__init__(self, executor_id: str)`
   and implements all six ABC members (`capabilities`, `health`, `launch`,
   `status`, `cancel`, `result`) from `src/praxis_executors/interface.py`.
   Construction must not probe the environment — no `shutil.which`, no
   `subprocess` call at `__init__` time (`src/praxis_cli/adapters.py`'s module
   docstring makes this a repo-wide rule; `tests/test_codex_cli.py::test_capabilities_does_not_touch_shutil_or_subprocess`
   pins the equivalent for `capabilities()`).
3. **`capabilities()` returns one Capability entry, statically.** Shape:
   `{"spec_version": "1.0.0", "executor_id": <constructor value>, "capabilities":
   [{"spec_version": "1.0.0", "satisfies": [{"kind": …}, …], "auth_transport":
   "subscription_cli"}]}`. `auth_transport` sits at the Capability level, is
   static advertisement metadata, and is never live-synced to `health()` — the
   split `claude_cli.py` and `codex_cli.py` both implement. Kinds come from the
   existing free-form vocabulary (`^[a-z0-9]+(-[a-z0-9]+)*$` per
   `capability.schema.json`) and must describe what the chosen surface can
   genuinely do; `coding`, `shell`, `filesystem` is the default set if the chosen
   surface is an agentic coding CLI (see Assumption 5), and a narrower honest set
   is required if it is not. A test must assert the advertisement validates
   against `capability-advertisement.schema.json` via
   `praxis_contracts.validator.validate_document` with
   `praxis_contracts.schema_paths.SCHEMA_DIR`, mirroring
   `tests/test_codex_cli.py::test_capabilities_advertisement_validates_against_schema`.
4. **`health()` maps to exactly three outcomes.** Executable absent from `PATH`
   → `UNAVAILABLE`. Confirmed unauthenticated → `UNAVAILABLE`, with no fallback
   branch. Confirmed authenticated *and* an executable that identified itself to
   a version probe → `AVAILABLE`. Anything else (probe unavailable, probe output
   ambiguous, executable never answered `--version`) → `DEGRADED`. This is
   `codex_cli.health()`'s mapping, including its deliberate narrowing that a
   binary which will not identify itself is `DEGRADED` rather than `AVAILABLE`.
   Every probe is bounded (`timeout=5`), never raises out of `health()`, and runs
   under the same filtered environment `launch()` uses
   (`tests/test_codex_cli.py::test_health_probes_use_the_same_filtered_environment_as_launch`).
5. **Auth detection must be safe, read-only, and non-destructive.** A
   sub-second status-style subcommand that only re-reads existing local state is
   acceptable; anything that starts an interactive login, mutates stored
   credentials, or performs a billable network call is not. The adapter must
   never read, parse, or otherwise inspect Copilot's stored token, config file,
   or keychain entry directly. If the CLI's status output has no machine-readable
   mode, distinguish states by text with an explicit unknown branch, exactly as
   `codex_cli._detect_authenticated` does (`return None` for unrecognized
   wording, which maps to `DEGRADED`, not to authenticated).
6. **`launch()` validates its inputs and builds argv defensively.**
   `request.parameters["prompt"]` is required and must be a `str`;
   `request.parameters.get("extra_args", [])` must be a `list` whose every entry
   is a `str`, checked per entry. Each violation raises `ExecutorError`, and no
   message may echo a parameter value (a prompt can carry a credential) — the
   guards `codex_cli.launch()` already implements. Argv must make the prompt
   impossible to reinterpret as options or as a subcommand; the `codex` adapter
   needed an explicit `--` terminator for this, and the equivalent protection for
   whichever invocation form Copilot uses must be verified, not assumed. Two
   tests are required: a prompt beginning with `-`, and a prompt equal to one of
   the CLI's own subcommand names, each reaching the process as a prompt.
   `stdin=subprocess.DEVNULL`, so a child that decides to prompt cannot block
   forever. A failed `Popen` raises `ExecutorError` with a redacted message.
7. **Output must be drained concurrently, not after exit.** The adapter must
   not call `communicate()` only once the process has exited: an agentic CLI
   transcript readily exceeds the OS pipe buffer (64KB on macOS) and the child
   blocks on write, deadlocking the run. Use the per-handle reader-thread pattern
   `codex_cli._OutputPump` implements, with a bounded join and an explicit
   incomplete-read signal rather than an indefinite wait. A test must prove a
   transcript larger than the pipe buffer comes back whole
   (`tests/test_codex_cli.py::test_result_returns_full_output_when_the_run_exceeds_the_os_pipe_buffer`
   is the model).
8. **Lifecycle semantics match the siblings exactly.** `status()` returns
   `RUNNING` while `poll()` is `None`, otherwise `CANCELLED` if the handle was
   cancelled, `SUCCEEDED` on returncode 0, `FAILED` otherwise. `cancel()`
   terminates a still-running process and marks the handle cancelled. `result()`
   raises `ExecutorError` while the execution is still running, caches a settled
   result, and does not cache a result whose transcript read timed out. An
   unknown `handle_id` raises `ExecutorError` from `status()`, `cancel()`, and
   `result()` alike.
9. **Result shape.** `evidence` is `{"process-exit-status": returncode == 0}`.
   `payload` carries `stdout`, `stderr`, `returncode`, and a
   `credentials-redacted` boolean recording whether redaction actually rewrote
   the transcript, plus an `output-read-error` string only when a read failed or
   timed out — the `codex_cli.result()` payload contract. A partial or failed
   read must never be reported as a silent, empty run.
10. **Credential redaction is behavioural, and must stay cheap.** The
    acceptance test is that a fake, credential-shaped secret planted in a run's
    output never survives into `ExecutionResult.payload`, into `evidence`, or
    into any `ExecutorError` message the adapter raises — asserted against fields
    that can genuinely contain the redacted text (the "structurally vacuous test"
    gap named in the sibling bundle's spec). Cover the GitHub credential family
    the chosen surface can actually leak rather than one literal prefix (see
    Assumption 8 for the starting shapes), plus `Authorization: Bearer` values.
    The pattern must remain roughly linear in transcript length: bound the runs
    either side of any keyword alternation, as `codex_cli` had to after an
    unbounded prefix made redaction quadratic (23s on 200KB of base64, 0.01s
    after). If a field-name-based pattern is used, add the same cost guard
    `tests/test_repair_findings_b1_issue41.py` carries.
11. **Environment isolation must be targeted, not blanket.** `launch()` (and
    every `health()` probe) constructs its environment explicitly from a copy of
    `os.environ` with a named list of variables removed. The list must be
    exactly those variables that could route the CLI onto a metered or
    API-key-billed credential instead of the stored subscription login;
    it must **not** blindly strip a variable the subscription login itself
    depends on, which would break auth rather than protect it. Whichever
    variables are stripped, the module records why, with the evidence that
    identified them (the `_ENV_VARS_TO_STRIP` comment in `codex_cli.py` is the
    model). A test asserts each stripped variable is absent from the subprocess
    environment even when set in the parent, and that unrelated variables survive.
12. **No fallback, no silent privilege escalation.** Unauthenticated or
    ambiguous auth state never resolves to a metered API path, to a different
    executor, or to a retry that supplies a credential. The adapter must not add
    a permission-loosening flag (blanket tool auto-approval, sandbox
    disablement, or equivalent) to argv on its own; if the chosen CLI offers one,
    it stays available to the caller through `extra_args` and is not a default
    (see Assumption 7).
13. **Tests.** `tests/test_copilot_cli.py`, mocked at the `shutil.which` /
    `subprocess.Popen` / `subprocess.run` boundary so the standard suite never
    invokes a real Copilot process. Shared doubles, if any, go in a top-level
    `tests/copilot_doubles.py`, not in `tests/conftest.py` — that file's own
    docstring states adapter doubles deliberately do not live there because every
    session pays for the import. One optional smoke test may exercise the real
    CLI, guarded by `@pytest.mark.skipif(shutil.which(<exe>) is None, …)` and
    strictly non-destructive; it must skip, never fail, when the CLI is absent.
14. **The whole suite passes.** Run `python3 -m pytest` from the worktree root.
    There is no `.venv` in this worktree (unlike the Codex bundle's worktree);
    the system interpreter already satisfies the dependencies — Python 3.14.7,
    pytest 9.1.1, jsonschema 4.26.0 — and `pytest.ini_options` in
    `pyproject.toml` already sets `pythonpath = ["src"]`. Verified during this
    enhancement: `python3 -m pytest tests/test_claude_cli.py` → 23 passed.
15. **`docs/executors.md` is updated in the same change.** Three edits, all in
    the "Adding a new executor adapter" section: drop Copilot from the
    "Adding a new backend (e.g. a future Copilot, OpenCode, or MLX/local
    adapter …)" example sentence; drop Copilot from "None of those remaining
    hypothetical adapters (Copilot, OpenCode, MLX) exist yet"; and add
    `CopilotCliExecutor` to the shipped-adapters list with its module path
    (`src/praxis_executors/adapters/copilot_cli.py`) and its `auth_transport`,
    updating "Five concrete adapters ship today" to six. This is not
    housekeeping: the identical omission for Codex became a review finding and a
    repair cycle, now pinned by `tests/test_repair_findings_b1_issue41.py`
    (`test_doc_no_longer_lists_codex_among_hypothetical_adapters`,
    `test_doc_lists_codex_cli_executor_among_concrete_adapters`,
    `test_doc_example_of_future_adapters_no_longer_names_codex`). Those existing
    tests will still pass either way — they are Codex-specific — so nothing but
    this criterion catches the same gap for Copilot.
16. **One escalation trigger, stated up front.** If the investigation in
    criterion 1 finds that no currently-supported Copilot surface can be driven
    non-interactively on a subscription login — or that the only non-interactive
    auth path is an ambient token indistinguishable from a metered credential —
    stop and escalate to the tech lead. Do not advertise
    `auth_transport: "subscription_cli"` for something that is not one, and do
    not quietly ship an `api_key`-transport adapter, which
    `AuthTransportPolicy` denies by default and which contradicts the bundle's
    stated scope.

## Explicitly out of scope

- Registering the adapter anywhere. No `ExecutorRegistry.register` call, no new
  entry in `src/praxis_cli/adapters.py`'s `_ADAPTER_FACTORIES`, no config or
  deployment wiring. `CodexCliExecutor` shipped and is still absent from that
  table; `executor_id` stays a constructor parameter the caller supplies.
- Extending `src/praxis_cli/fields.py`'s `ClaudeCliExecutor`-specific branches
  (`isinstance(executor, ClaudeCliExecutor)`) to cover the new adapter.
- Any change to `capability.schema.json`,
  `capability-advertisement.schema.json`, `src/praxis_executors/policy.py`'s
  `auth_transport` vocabulary, or `docs/ontology.md`. This bundle consumes the
  existing vocabulary; it does not extend it.
- Refactoring `claude_cli.py` or `codex_cli.py` — including extracting a shared
  base class for the lifecycle methods the two already duplicate. `codex_cli.py`
  declined exactly that ("A shared base would have to edit that file, outside
  this bundle's footprint"); the same reasoning applies here, and a third copy
  of those methods is the accepted cost.
- Reading, parsing, or extracting Copilot's stored credential in any form.
- Any real, unmocked Copilot CLI invocation in the standard test suite; the
  optional `skipif`-guarded smoke test is the only real-CLI touchpoint.
- Interactive/streaming session support, MCP or tool-configuration plumbing,
  model selection, and publishing supported models/modes in the advertisement.
  The last is a contract question the Codex adapter already deferred for the
  same reason: a build string or model name is exactly what a vendor-neutral
  Capability document forbids.
- Wiring executor results into `TransitionEngine.apply`. `ExecutionResult.evidence`
  stays a flat claim dict; conversion is `registry.evidence_to_proof_records`'s
  job and dispatch is still a caller's.
- `README.md`. Its single Copilot mention names Copilot as an example of a
  possible executor, which stays true; only `docs/executors.md` makes the
  shipped-versus-hypothetical claim that goes stale.

## Assumptions made

1. **The bundle mirrors `codex_cli.py`, not `claude_cli.py`, wherever the two
   differ.** Evidence: the raw spec points at `claude_cli.py` as the shape to
   follow, but every difference between the two siblings is a place where
   `claude_cli.py` carries an audited gap that the Codex bundle was explicitly
   chartered to not repeat — unfiltered subprocess environment, read-after-exit
   output handling, single-shape redaction, unvalidated `extra_args`. Both files
   are merged in this worktree and the newer one is the repaired shape. Where
   this spec cites a behaviour, it cites the Codex one.
2. **`auth_transport` is `"subscription_cli"`.** Evidence: the bundle scope says
   "subscription-backed integration"; both existing CLI adapters advertise
   `subscription_cli`; and it is safe-by-default under `AuthTransportPolicy`
   (only `metered_api` and `api_key` are denied unless explicitly allowed), so
   this choice does not loosen any existing guard. `oauth_cli` exists in the
   vocabulary but the repo records no semantic distinction between the two
   values, so sibling precedent decides. Criterion 16 governs the case where the
   investigation contradicts this.
3. **Naming follows sibling convention:** `CopilotCliExecutor`,
   `tests/test_copilot_cli.py`, `tests/copilot_doubles.py`, and
   `executor-copilot-cli-1` as the id used in tests. Evidence: `claude_cli.py` /
   `tests/test_claude_cli.py` / `executor-claude-cli-1` and `codex_cli.py` /
   `tests/test_codex_cli.py` / `tests/codex_doubles.py` /
   `executor-codex-cli-1`. (`capability-advertisement.schema.json` says
   `executor_id` "must not encode a vendor or model name"; nothing enforces it
   and both siblings' ids do, so this follows the code, not the description.)
4. **`python3 -m pytest` is the test command.** Evidence: no `.venv` exists in
   this worktree, and the system interpreter runs the suite as-is — verified by
   running `tests/test_claude_cli.py` (23 passed in 0.19s).
5. **Default capability kinds are `coding`, `shell`, `filesystem`.** Evidence:
   `CodexCliExecutor` — the closest analogue, an agentic coding CLI driven as a
   subprocess — advertises exactly those three, unconditionally, in a single
   Capability entry. This is a default to be corrected downward if the surface
   the investigation selects genuinely cannot do one of them, not a runtime
   branch in `capabilities()`.
6. **No live investigation of the Copilot surface was possible in this
   session.** Evidence: this session's sandbox denied every command touching
   anything outside the worktree, including `command -v copilot` and
   `gh issue view 42`. The same limitation applied to the Codex bundle's
   enhancement (see `docs/develop/specs/b1-issue41.md`, clarified item 6), where
   leaving the probe investigation to the implementer worked. Consequently this
   spec asserts no Copilot command name, subcommand, flag, or env var as fact,
   and issue #42's full body could not be read — the bundle `spec.md` quoted
   above is the entire input.
7. **The adapter adds no permission-loosening flag by default.** Evidence:
   `codex_cli.launch()` passes only `exec`, the caller's `extra_args`, `--`, and
   the prompt, adding nothing that widens the CLI's own default sandbox; and
   `extra_args` is the established channel for a caller that wants one. This is
   a tightening default, not a new restriction on the caller.
8. **Redaction's starting credential shapes.** GitHub's token families share
   documented prefixes (`ghp_`, `gho_`, `ghu_`, `ghs_`, `ghr_`, and
   `github_pat_`), and OAuth-style bearer values reach a transcript as opaque
   `Authorization: Bearer` runs or as JWTs. Evidence: `codex_cli` uses exactly
   this reasoning for OpenAI/ChatGPT shapes, and its test module names each fake
   secret shape it covers. These are a starting set for the implementer to
   confirm against what the chosen CLI can actually emit — criterion 10's
   acceptance test is behavioural, so a shape added or dropped after
   investigation does not change what is being verified.

## Open questions

None. The one item that could have escalated — whether the chosen Copilot
surface can honestly claim `subscription_cli` — is not a gap this pass has to
leave open: sibling precedent plus the bundle's own stated scope give a
defensible default (Assumption 2), and the case where the investigation
contradicts it is written down as an explicit stop-and-escalate trigger
(criterion 16) rather than a decision the implementer makes quietly. The
surface investigation itself is implementer-owned work the raw spec already
scoped correctly, and this session could not perform it (Assumption 6).
