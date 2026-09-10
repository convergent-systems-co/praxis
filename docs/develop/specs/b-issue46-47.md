# Bundle b-issue46-47 — Enhanced Spec

## Original content

> # Bundle b-issue46-47: praxis doctor + praxis run --executor CLI wiring
>
> ## Issues
> - #46 — `praxis doctor`
> - #47 — `praxis run --executor <id|auto>` runner + per-node/per-graph executor requirements
>
> ## Scope
> Both extend the existing CLI scaffold (merged in PR #78) and share the same CLI module
> footprint, so they are bundled together.
>
> ### #46 `praxis doctor`
> Validate: Python/runtime prerequisites, Praxis configuration validity, graph/overlay
> schema validity (reuse `praxis_contracts.validator`), executor discovery results per
> adapter (installed/authenticated status), policy state (confirms `metered_api`/
> `api_keys` deny settings are active).
>
> ### #47 `praxis run <overlay-or-graph> --executor <id>` / `--executor auto`
> Wire together the existing `TransitionEngine`, `ExecutorRegistry`, and
> `praxis_executors.matching.match`:
> - `--executor <id>`: explicit selection always wins over auto-matching, but is still
>   subject to policy — a policy-violating explicit choice is refused, not silently
>   overridden.
> - `--executor auto`: uses existing matching logic.
>
> ## Base
> `origin/main` at commit 9366f5b (praxis CLI scaffold + executors discover/executors/match merged).

## Clarified acceptance criteria

### A. Shared CLI plumbing (both issues)

1. **`main()`'s dispatch gate must grow from one recognized subcommand to three, and
   this is the single highest-risk edit in the bundle.** `src/praxis_cli/main.py:42-44`
   today reads:
   ```python
   if not argv or argv[0] != "executors":
       _print_version()
       return 0
   ```
   Everything whose first token is not the literal `"executors"` prints the package
   version and exits 0 — including `--version`, `--help`, and an unknown subcommand
   (the known limitation recorded in the comment at `main.py:36-41`). Change the gate
   to recognize exactly `{"executors", "doctor", "run"}` and keep the legacy
   print-version-and-exit-0 fallback for everything else, unchanged. Do not convert the
   fallback into an argparse error: `tests/test_praxis_cli.py` calls `main()` bare under
   pytest, where `sys.argv[1:]` is pytest's own arguments, and that test must keep
   passing (the full rationale is `docs/develop/specs/b2-issue45.md` criterion 3).
   Narrowing the fallback so `praxis bogus` reports an error is still out of scope
   (`main.py:38-41` says it wants its own issue).

2. **The executors-only argument checks must not run for `doctor`/`run`.**
   `main.py:46-52` builds one parser and then reads `args.json` and
   `args.executors_command` unconditionally. Both attributes exist only on the
   `executors` namespace; reaching them for a `doctor` or `run` invocation raises
   `AttributeError`. Dispatch per top-level token — build (or select) the parser for the
   token that was actually given, and keep the `--json`-is-executors-only guard on the
   `executors` branch only.

3. **The lazy-import property is load-bearing and is covered by an existing test.**
   `main.py:55-57` imports the adapter and command modules inside `main()`, after the
   version fallback, and `tests/test_praxis_cli_executors.py:205-240` runs a subprocess
   probe asserting that the version path leaves `praxis_cli.discover_cmd`,
   `praxis_cli.status_cmd`, `praxis_cli.match_cmd`, `praxis_cli.fields`,
   `praxis_executors.adapters.claude_cli` and `praxis_executors.adapters.ollama` unimported.
   New modules (`praxis_cli.doctor_cmd`, `praxis_cli.run_cmd`, and anything they pull in
   — `praxis_runtime.transitions`, `praxis_executors.registry`) must be imported inside
   `main()` on their own branch, never at module scope. Extend that probe's `watched`
   list with the new modules rather than leaving the new imports unguarded.

4. **New code lives in new `praxis_cli` submodules, plus tests.** Follow the existing
   split: one module per subcommand (`discover_cmd.py`, `status_cmd.py`, `match_cmd.py`)
   with a `run_<name>(...) -> int` entry point returning the exit code (see
   `discover_cmd.py:62-64`). Add `src/praxis_cli/doctor_cmd.py` and
   `src/praxis_cli/run_cmd.py` the same way. Reuse `praxis_cli.adapters.build_adapters()`
   (`adapters.py:49-54`) as the one construction path for adapters — do not build a
   second adapter list.

5. **Argument parsing stays on stdlib `argparse`.** `pyproject.toml`'s dependencies are
   `["jsonschema>=4.18", "referencing>=0.28.4"]`; no CLI framework is available and none
   is to be added.

### B. #46 — `praxis doctor`

6. **Command shape.** `praxis doctor [--graph PATH]... [--overlay-manifest PATH]...`.
   Both document flags are repeatable and both are optional. No `--json` flag (see
   **Explicitly out of scope**).

7. **Output shape and exit code.** Print one block per check, in the order the issue
   lists them, using the same two-level shape `discover` already prints
   (`discover_cmd.py:54-60`): a `<check name>:` header line, then indented
   `  <field>: <value>` lines. Every check ends with an explicit verdict of `ok`, `warn`,
   or `fail`. `praxis doctor` returns **0 when no check is `fail`** (warnings do not
   affect the exit code) and **1 when at least one check is `fail`**. No check may raise
   out of the command: a check that cannot complete reports `fail` with the reason in its
   own block, and the remaining checks still run.

8. **Check 1 — Python/runtime prerequisites.** Concretely:
   - `sys.version_info >= (3, 10)`, the floor `pyproject.toml`'s `requires-python = ">=3.10"`
     declares. Below it: `fail`.
   - `praxis-contracts` is installed and its version is readable via
     `importlib.metadata.version("praxis-contracts")` — the same call `main.py:8-9`
     already makes. Not installed: `fail`.
   - Both runtime dependencies are importable and at or above their declared minimums
     (`jsonschema>=4.18`, `referencing>=0.28.4`), read through
     `importlib.metadata.version`. Missing or below minimum: `fail`.

9. **Check 2 — Praxis configuration validity, which today means the installed package's
   schema data, not a user configuration file.** **Praxis has no configuration file,
   configuration module, or configuration environment variable anywhere in this
   repository**: there is no `[tool.praxis]` table in `pyproject.toml`, no config module
   under `src/`, and the only `PRAXIS_*` name in the tree (`PRAXIS_STUB_ADAPTERS`) is read
   inside a test probe script (`tests/test_praxis_cli_executors.py:215`), never by
   shipped source. So this check reports, concretely:
   - that `praxis_contracts.schema_paths.SCHEMA_DIR` resolves to an existing directory
     (this is `importlib.resources`-based, so it is exactly the "did the package data
     install correctly" question — `schema_paths.py:6`, and `pyproject.toml`'s
     `[tool.setuptools.package-data]` entry that ships `schemas/v1/*.schema.json`);
   - that every `*.schema.json` in that directory loads as JSON via
     `praxis_contracts.validator.load_schema` and passes
     `jsonschema.Draft202012Validator.check_schema` — a schema file that is itself
     malformed makes every downstream contract check meaningless, so it is a `fail`;
   - a stated line that **no user configuration file exists to validate** — Praxis reads
     none today. This is a named gap reported in the output, exactly like the
     `version: unknown` gap `executors discover` already reports and README.md:409
     documents. **Do not invent a configuration file name, format, or search path.**

10. **Check 3 — graph/overlay schema validity.** For each `--graph PATH`, load it with
    `praxis_runtime.graph.load_graph` (`graph.py:48-80`), which validates against
    `graph.schema.json` through `praxis_contracts.validator.validate_document` *and* adds
    the structural invariants the schema cannot express (edges reference existing nodes,
    entry node exists, every node reachable, terminal nodes exist). Report the file path,
    node/edge counts, and `ok`, or the `GraphValidationError` message and `fail`. For each
    `--overlay-manifest PATH`, load the JSON and pass it to
    `praxis_overlay.manifest.load_manifest` (`manifest.py:65`), which adds the
    namespace-prefix cross-field check the schema documents as its own job
    (`overlay-manifest.schema.json`'s `namespacedString` `$comment`). Report `overlay_id`,
    `namespace` and `ok`, or the error and `fail`. **With neither flag supplied, this check
    reports `skipped (no document supplied)` and does not affect the exit code** — doctor
    has no document-discovery mechanism and must not acquire one here (see
    **Explicitly out of scope**).

11. **Check 4 — executor discovery per adapter, reusing `discover`'s existing rows
    verbatim.** Call `discover_cmd.build_discover_rows(adapters)` (`discover_cmd.py:27-51`)
    over `adapters.build_adapters()` and print its `installed` / `version` /
    `authenticated` / `auth_transport` / `capabilities` cells. Do not re-derive any of
    those fields: `praxis_cli.fields` already owns that derivation and already degrades a
    single unreadable adapter to a cell rather than failing the command
    (`fields.py:20-47`).
    - **An absent or degraded adapter is `warn`, never `fail`.** README.md:409 states as a
      shipped property that all four executors commands work on a machine with no `claude`
      binary and no Ollama service; a doctor that exits nonzero on such a machine would
      contradict it and would report `fail` on nearly every developer machine.
    - This check must remain non-destructive and must never trigger an interactive login
      prompt — the same guarantee README.md:393 already makes for the `executors`
      commands, preserved by reusing their code path rather than adding a new probe.
    - `fail` for this check is reserved for `build_adapters()` itself raising, i.e. no
      adapter could be constructed at all.

12. **Check 5 — policy state: the two unsafe-by-default auth transports are denied.**
    `praxis_executors.policy.py:25` defines
    `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS = frozenset({"metered_api", "api_key"})`. Note the
    exact spelling: the transport value is **`api_key`, singular** — the issue's `api_keys`
    is not a value any schema or module uses (`policy.py:22-25`,
    `capability.schema.json`). Report the check against the real values.
    - Assert through the public surface only: construct `AuthTransportPolicy()` (the
      no-argument default `ExecutorRegistry.select` itself installs, `registry.py:72-75`)
      and call `is_eligible("probe-executor", advertisement)` with a synthetic
      single-capability advertisement for each of `metered_api`, `api_key` and `local`.
      Expect `False`, `False`, `True`. Do not call `_capability_is_eligible` — it is
      private.
    - Any of those three probes returning the wrong verdict is `fail`: it means the
      deny-by-default posture the issue asks doctor to confirm is not active.
    - Additionally report, as an informational line, whether any advertisement actually
      discovered in check 4 carries a denied transport (such an executor exists but will
      be excluded from every match). Present-but-excluded is `warn`, not `fail`.

### C. #47 — `praxis run`

13. **Command shape.**
    `praxis run <target> [--executor <id>|auto] [--capability KIND]... --run-dir DIR [--run-id ID]`.
    `--executor` defaults to `auto` when omitted. `--run-dir` is **required** — a command
    that writes a durable run checkpoint and event log must never pick a destination
    implicitly.

14. **`<target>` resolution: a graph document path, else a shipped overlay id.** If
    `<target>` is an existing filesystem path, load it with
    `praxis_runtime.graph.load_graph` — the same graph-document entry point
    `python -m praxis_dashboard --graph` already takes (README.md:383-389,
    `examples/sample-graph.json` is a runnable sample). Otherwise treat it as an overlay
    id resolved through a small explicit mapping in the CLI module, mirroring
    `adapters.py:31-46`'s explicit-factory-dict precedent:
    `trivial` → `overlays.trivial.overlay.build_trivial_graph`, `development` →
    `overlays.development.graph.build_development_graph`. The overlays this repository
    ships are Python builders, not files on disk, so there is nothing to load from a path
    for them. An unrecognized target that is neither an existing path nor a known overlay
    id: print a message naming the known overlay ids, return nonzero, and **write nothing
    into `--run-dir`**.

15. **Per-node executor requirement is `node.metadata["requirement"]`.** This is an
    established convention in this repository, not a new one:
    `requirement.schema.json`'s own title description is "The shape a graph node uses to
    declare what capabilities it needs", and `src/overlays/development/graph.py:68-82`
    already writes a `Requirement`-shaped dict into exactly `node.metadata["requirement"]`
    for every dispatched node. That file's docstring (`graph.py:23-26`) records that the
    entry is **declarative metadata only — "no core module currently reads or enforces
    it"**. Making `praxis run` its first real consumer is the substance of this issue.
    - There is nowhere else it could live: `graph.schema.json` sets
      `additionalProperties: false` on the node object and permits only `id`, `kind` and
      an open `metadata` object. Changing that schema is out of scope.
    - Validate each node's requirement with
      `validate_document(requirement, schema_path("requirement.schema.json"))` **before**
      any executor is selected or launched. An invalid requirement fails that node closed
      with the `ContractValidationError` reason and launches nothing.

16. **Per-graph executor requirement comes from the CLI, because a graph document has
    nowhere to put one.** `graph.schema.json` is closed at the top level
    (`additionalProperties: false`, no graph-level `metadata`), and
    `praxis_runtime.graph.Graph` (`graph.py:39-45`) carries no metadata field. So the
    graph-level requirement is supplied as repeatable `--capability KIND` flags,
    synthesized into a `Requirement` document exactly the way `executors match` already
    synthesizes one (`match_cmd.py`, per `docs/develop/specs/b2-issue45.md` criterion 7):
    one `{"promise": {"spec_version": "1.0.0", "kind": KIND}, "constraint": "required"}`
    entry per flag, wrapped in `{"spec_version": "1.0.0", "requirements": [...]}`.
    Precedence, stated explicitly:
    1. a node's own `metadata["requirement"]` wins for that node;
    2. otherwise the `--capability`-derived graph-level requirement applies;
    3. a node with neither dispatches no executor at all. It is transitioned
       `start` → `complete` with no evidence, which the engine's evidence gate still
       judges — a node that declares an `evidence_requirement` and receives no evidence
       fails closed on its own, exactly as it does today. This is what the development
       overlay's placeholder nodes (`metadata={}`, `graph.py:31-37`) and both trivial
       overlay nodes look like.

17. **`--executor auto` uses the registry's existing selection path, not a
    reimplementation.** Register every adapter from `build_adapters()` into an
    `ExecutorRegistry` and call `registry.select(requirement)` (`registry.py:65-76`), which
    installs `policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)`
    itself when no `is_eligible` is passed. This guarantees `run`'s eligibility decisions
    are identical to the registry's own. If `MatchResult.selected is None`, that node fails
    closed: print the `unsatisfied` entries (including the `policy_excluded` distinction),
    apply the `fail` transition for the node, stop the run, and return nonzero. State and
    events already written stay on disk and stay valid.

18. **`--executor <id>` is explicit selection: it bypasses matching, but not policy.**
    - The id must be one of `build_adapters()`'s keys (`adapters.py:31-46` —
      `executor-subprocess-1`, `executor-fake-1`, `executor-claude-cli-1`,
      `executor-ollama-1`). Unknown id: message naming the known ids, nonzero exit,
      nothing written, nothing launched.
    - **Policy is checked before anything launches.** Read the adapter's advertisement via
      `executor.capabilities()` and call
      `AuthTransportPolicy().is_eligible(executor_id, advertisement)` — the same default
      object `registry.select` installs. `False` means refuse: print a message naming the
      executor and its denied `auth_transport`, return nonzero, launch nothing. This is
      the issue's "a policy-violating explicit choice is refused, not silently
      overridden", made concrete.
    - **The explicit executor must also satisfy the node's requirement, or the node is
      refused.** See **Assumptions made** — this is the reading most likely to be revisited
      by a reviewer, and it is called out there with its evidence rather than buried here.
    - Because `ExecutorRegistry.execute*` always re-selects (`registry.py:130`), the
      explicit path cannot go through it. Drive the chosen adapter's
      `launch`/`status`/`result` directly and convert its evidence with the public
      `registry.evidence_to_proof_records(...)` (`registry.py:150-180`) — do not duplicate
      that conversion, and do not call any `_`-prefixed registry helper.

19. **Dispatch and transition wiring per node.** For a node with a requirement and a
    resolved executor:
    - `engine.apply(node_id, "start")` first (`transitions.py:83-97` makes `start` the only
      legal transition out of `PENDING`).
    - Build `ExecutionRequest(promise=<the first `required` promise in the node's
      requirement>, parameters=node.metadata.get("parameters", {}))`
      (`interface.py:32-37`).
    - Auto path: `registry.execute_with_proof_records(requirement, request, run_id=...,
      graph_version=graph.spec_version, node_id=node_id)` (`registry.py:91-120`) — it
      already converts the flat evidence dict into the `list[dict]` of proof-record
      documents `TransitionEngine.apply(..., evidence=...)` requires, using the
      `executor_id` its own `select()` chose.
    - Then `engine.apply(node_id, "complete", evidence=records)` on
      `ExecutorStatus.SUCCEEDED`, or `engine.apply(node_id, "fail")` on `FAILED` /
      `CANCELLED` and on any `ExecutorError` raised by `launch`/`status`/`result`.
      An `ExecutorError` must never escape as a traceback: print the reason, fail the
      node, return nonzero.
    - `registry.py:10-17` states plainly that "no executor-to-runtime orchestrator module
      exists in this codebase today" and that this dispatch step is the caller's
      responsibility. This command is that caller; it is the first one.

20. **Driving loop: mirror `praxis_runtime.testing.fake_executor.FakeExecutor.run_to_completion`.**
    That method (`fake_executor.py:26-54`) is this repository's one existing
    drive-a-run-to-completion loop: read `engine.current_state()`, take every cursor not in
    a terminal status, apply `start` where `engine.legal_next(node_id)` offers it, then
    apply the node's outcome, bounded by a `max_steps` guard that raises rather than
    spinning forever. `run_cmd` walks cursors the same way — through
    `legal_next`/`apply` only, never touching `RunStateStore`/`EventLog` directly — with
    the scripted outcome replaced by the executor dispatch of criterion 19, and the same
    `max_steps` bound. A run that hits the bound stops with a nonzero exit and a message,
    not an unhandled exception.

21. **Run directory layout matches the documented quickstart, so the dashboard can read
    it.** `RunStateStore(run_dir / "run-state.json")` and `EventLog(run_dir / "events")`,
    exactly as README.md:340-341 constructs them and exactly what
    `python -m praxis_dashboard --run-dir` expects (README.md:389). `--run-id` defaults to
    a fresh `uuid4().hex`.

22. **A `--run-dir` that already contains `run-state.json` is refused.** Print a message,
    return nonzero, write nothing. Resuming an existing run is real behavior with its own
    semantics (matching `--run-id` against the loaded state, replaying cursors) and is out
    of scope here — refusing is the fail-closed choice this repository takes everywhere
    else, and it cannot corrupt an existing run.

23. **Tests must never launch a real executor.** Every shipped adapter's `launch()`
    spawns real work or demands parameters a graph cannot supply:
    `ClaudeCliExecutor.launch` requires `parameters["prompt"]` and `Popen`s the real
    `claude` binary (`claude_cli.py:102-125`); `SubprocessExecutor.launch` requires
    `parameters["command"]` and `Popen`s it; `FakeCapabilityExecutor.launch` requires a
    `request_key` present in its construction-time `script`, and `adapters.py:41-43`
    constructs it with `script={}`, so it raises `ExecutorError` for every request.
    Acceptance tests therefore drive `run_cmd`'s entry point with an injected adapter
    mapping (a `FakeCapabilityExecutor` built with a real `script`, or an equivalent
    stub), never through `build_adapters()`'s real four —
    `tests/test_praxis_cli_executors.py:213-221`'s `PRAXIS_STUB_ADAPTERS` stub is the
    existing precedent for keeping a CLI test hermetic. Give `run_cmd`'s entry point an
    adapter-mapping parameter for exactly this reason, the way
    `discover_cmd.run_discover(built)` already takes its adapters as an argument
    (`main.py:59-65`).

### D. Acceptance (both issues)

24. Unit tests, all hermetic (no `claude` binary, no Ollama service, no real subprocess):
    - `doctor`: each of the five checks passing; at least one check failing and driving the
      exit code to 1; an absent adapter producing `warn` and exit 0; a malformed `--graph`
      document producing `fail`; the check-3 `skipped` path with no document flags.
    - `run`: `--executor auto` selecting and completing a two-node fixture graph;
      `--executor <id>` explicit selection winning over what auto-matching would have
      picked; an explicit choice whose transport is denied by `AuthTransportPolicy` being
      refused with a nonzero exit and no launch; a node whose requirement nothing satisfies
      failing closed with the `unsatisfied` reasons; an unknown target and an unknown
      executor id both refused before anything is written.
    - `main()` dispatch: `praxis doctor` and `praxis run` reach their commands;
      `praxis --version`, `praxis` bare, and `praxis bogus` still print the version and
      exit 0; the import probe still shows the version path importing no executor,
      runtime or command module.
25. The full suite passes: `.venv/bin/python -m pytest` (the environment convention this
    run's sibling bundles use; `pyproject.toml` sets `testpaths = ["tests"]` and
    `pythonpath = ["src"]`).
26. **README.md is updated**, because this bundle falsifies three statements currently in
    it: line 320 ("this bundle adds no graph-driving subcommands to the CLI — that's
    separate, later work"), line 415 ("`praxis` with ... anything other than `executors`
    as its first argument, prints the package version and exits 0"), and the "Usage"
    section's claim that driving a graph to completion is library-only. Add `praxis doctor`
    and `praxis run` to the CLI section alongside the existing `executors` commands.
27. Open a PR against `main` referencing both issues with closing keywords
    (`Closes #46`, `Closes #47`). Do not merge — merge policy is `never` for this
    repository.

## Explicitly out of scope

- **Any change to `graph.schema.json`, `requirement.schema.json`, or any other
  `schemas/v1/*.schema.json`.** Both a per-node and a per-graph requirement are expressible
  today (node `metadata`, and the CLI respectively, per criteria 15-16); widening a closed
  contract schema is a separate change with its own compatibility blast radius.
- **Any change to `Executor` (`interface.py`), `matching.py`, `policy.py`, `registry.py`,
  `transitions.py`, `graph.py`, or any adapter.** This bundle is CLI-only:
  `src/praxis_cli/*` plus tests plus README. Every behavior above is composed from those
  modules' existing public surfaces.
- **A configuration file for Praxis** — no name, no format, no search path, no
  `[tool.praxis]` table. Doctor reports the absence (criterion 9); it does not invent the
  thing it would validate.
- **Document discovery for `praxis doctor`.** It validates the graphs and manifests it is
  handed and skips the check otherwise (criterion 10). It does not scan the working
  directory, the installed overlays, or `examples/`.
- **`--json` output for `doctor` or `run`.** `--json` remains bound to the bare
  `praxis executors` status table (README.md:409, `main.py:49-52`).
- **Resuming or replaying an existing run.** A populated `--run-dir` is refused
  (criterion 22).
- **Concurrent or parallel node dispatch.** The loop of criterion 20 is sequential, like
  the `FakeExecutor` loop it mirrors. Fan-out edges still create independent cursors; they
  are executed one at a time.
- **Node execution parameters beyond `node.metadata["parameters"]`.** No prompt templating,
  no per-node command construction, no parameter derivation from `node.kind`.
- **Narrowing `main()`'s unknown-subcommand fallback so `praxis bogus` errors.**
  `main.py:36-41` explicitly defers that to its own issue; this bundle only adds two more
  recognized tokens.
- **`--prefer` / `--prohibit` capability flags.** `run --capability` synthesizes `required`
  constraints only, matching `executors match`'s existing surface.
- **Reporting real adapter versions.** `executors discover`'s `version: unknown` gap
  (README.md:409) stays as it is; `doctor` reuses those rows rather than fixing them.

## Assumptions made

1. **An explicit `--executor <id>` that does not satisfy the node's declared requirement is
   refused, not used with a warning** (criterion 18). Evidence: the issue says explicit
   selection "always wins over auto-matching" and names policy as the constraint that still
   applies, which leaves requirement satisfaction genuinely ambiguous. Every comparable
   decision point in this repository fails closed rather than proceeding on a warning —
   `validator.validate_document` ("Fail-closed", `validator.py:69`),
   `TransitionEngine.apply` ("fail-closed, no partial write", `transitions.py:5-7`),
   `PolicyProfile` lookup ("every lookup fails closed ... rather than silently falling back",
   `profiles.py:6-8`), and `ExecutorRegistry._execute_selected`, which raises
   `RegistryError` rather than running something unselected (`registry.py:131-134`).
   Resolve-or-name: in scope, backed by a repository-wide convention rather than by taste,
   does not weaken the stated criterion (it makes explicit selection win the *ranking*, which
   is what auto-matching does, while still honoring what the graph declared), no
   security/compat/cost impact — the more permissive reading would be the one with
   blast radius, since it would let an operator dispatch work to an executor that never
   advertised the capability — and it is trivially reversible if a reviewer prefers
   warn-and-proceed. **This is the assumption in this spec most likely to be corrected;
   the surrounding structure does not depend on which way it goes.**

2. **Per-node requirements are read from `node.metadata["requirement"]`, and the per-graph
   requirement is supplied by repeatable `--capability` flags** (criteria 15-16). Evidence:
   `requirement.schema.json`'s description names the graph node as the declarer;
   `src/overlays/development/graph.py:68-82` already writes that exact key, and its docstring
   (`graph.py:23-26`) records that nothing reads it yet; `graph.schema.json` closes the node
   object and the top-level object to additional properties, so metadata is the only place a
   node requirement can live and there is no place at all for a graph-level one; the
   `--capability` synthesis reuses `executors match`'s already-shipped flag shape.
   Resolve-or-name: in scope, defensible from three independent on-record sources, changes no
   stated criterion, touches no schema or public interface, and is checkable by reading the
   overlay graph.

3. **`praxis doctor`'s "Praxis configuration validity" check means the installed schema
   package data, plus an explicit statement that no user configuration file exists**
   (criterion 9). Evidence: exhaustive search of the tree found no config module, no
   `[tool.praxis]` table, and no `PRAXIS_*` variable read by shipped source; the one
   configuration-shaped thing that can actually be invalid on a user's machine is the
   `importlib.resources`-resolved `schemas/v1/` package data (`schema_paths.py:6`,
   `pyproject.toml`'s `package-data` entry). Naming an unfillable gap in the output is the
   precedent this CLI already set with `version: unknown` (README.md:409). Resolve-or-name:
   in scope, has an on-record precedent for how to report a gap, does not weaken the check
   (it still fails on genuinely broken installs), and inventing a config format instead
   would have failed conditions 2 and 4 outright.

4. **A missing or degraded adapter is a doctor `warn`, and doctor exits 0 on a machine with
   no `claude` binary and no Ollama service** (criterion 11). Evidence: README.md:409 states
   as shipped behavior that the executors commands all work on exactly such a machine, and
   `praxis_cli.fields` is built around degrading one row rather than the command
   (`fields.py:30-47`). Resolve-or-name: in scope, backed by documented existing behavior,
   does not drop the check (the row is still reported in full), no security impact, and the
   verdict mapping is a one-line change if a reviewer wants adapters treated as required.

5. **`praxis run` refuses a `--run-dir` that already holds a `run-state.json`, and requires
   `--run-dir` explicitly** (criteria 13, 22). Evidence: `RunStateStore.save` overwrites
   atomically (`state.py:81-90`) and `EventLog` appends to whatever is in the directory
   (`events.py:56-75`), so silently reusing a populated directory would interleave two runs'
   events under one log; the repository's fail-closed convention (assumption 1's citations)
   says refuse rather than guess. Resolve-or-name: in scope, defensible from the storage
   modules' own behavior, does not change any stated criterion, prevents rather than creates
   a data-loss path, and is straightforwardly relaxed later by whoever implements resume.

6. **Overlay targets resolve through a small explicit id→builder mapping in the CLI module**
   (criterion 14). Evidence: `src/overlays/{trivial,development}` expose graph builders in
   Python and ship no graph document, and `praxis_cli/adapters.py:31-46` is this CLI's
   existing "explicit dict of factories, not a plugin-discovery system" precedent, adopted
   for the same reason in `docs/develop/specs/b2-issue45.md`. Resolve-or-name: in scope,
   sibling example on record, adds no dependency or discovery mechanism, and is replaceable
   by real overlay discovery later without changing the command's surface.

7. **Node execution parameters come from `node.metadata.get("parameters", {})`**
   (criterion 19). Evidence: `node.metadata` is the established open extension point for
   everything the engine and policy layers read off a node — `evidence_requirement`
   (`trivial/overlay.py:72-79`), `policy_requirement` (`profiles.py:5-6`),
   `resource_claims` (`transitions.py:31-35`), `requirement` (`development/graph.py:68`) —
   and `graph.schema.json` marks `metadata` `additionalProperties: true`, so the convention
   extends without a schema change. Resolve-or-name: in scope, follows four on-record uses of
   the same extension point, changes no stated criterion, and is additive (a graph that omits
   it gets `{}` and behaves exactly as before). Note the consequence made explicit in
   criterion 23: with no `parameters`, all four shipped adapters raise `ExecutorError` on
   launch, which is why the acceptance tests inject their own adapters.

## Open questions

None. The three gaps that looked open on first read — what "Praxis configuration" refers to
when the repository has no configuration system, where a per-graph executor requirement can
live when `graph.schema.json` is closed at the top level, and how a graph node supplies the
launch parameters every shipped adapter demands — each resolved against evidence already in
the tree: the `version: unknown` precedent for reporting an unfillable gap in CLI output, the
schema's own closed-object shape read together with the development overlay's existing
`metadata["requirement"]` convention, and `node.metadata`'s four existing uses as the
engine-and-policy extension point. The one decision that could reasonably have gone the other
way — whether an explicit `--executor` may ignore a node's declared requirement — is recorded
as assumption 1 with the fail-closed convention that justifies it and an explicit note that it
is the likeliest thing for a reviewer to flip.
