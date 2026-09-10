# Bundle b2-issue45 — Enhanced Spec

## Original content

> # Bundle b2-issue45
>
> ## Issues
> - #45 — praxis CLI scaffold + executors discover / executors / executors match --explain
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b2-issue45
> branch: develop/b2-issue45, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build.
>
> ## Existing architecture (read before writing code)
>
> - `src/praxis_cli/main.py` — **already exists** (added by issue #52's
>   packaging fix): a minimal `main()` that prints the package version and
>   exits 0, wired as the `praxis` console-script entry point in
>   `pyproject.toml`. **Extend this file's `main()` with subcommand
>   dispatch — do not replace it or break `praxis --version`/`praxis
>   --help`'s existing behavior**, whatever that currently is (read the file
>   first).
> - `src/praxis_executors/registry.py` — `ExecutorRegistry`, `.advertisements()`,
>   `.select()`.
> - `src/praxis_executors/matching.py` — `match()`, `MatchResult`,
>   `UnsatisfiedPromise` (now carries a `policy_excluded` field per issue #38).
> - `src/praxis_executors/policy.py` — `AuthTransportPolicy` (now wired as
>   `ExecutorRegistry`'s default per issue #62/remediation-1).
> - The adapters shipped so far: `src/praxis_executors/adapters/{fake,subprocess_executor,claude_cli,ollama}.py`
>   (and possibly `codex_cli.py` if issue #41 has landed by the time this
>   bundle runs — check `git log`/the adapters directory for what actually
>   exists, don't assume).
>
> ## Task
>
> - `praxis executors discover`: runs discovery across every registered
>   adapter this repo ships (construct instances of whichever adapters
>   actually exist in `src/praxis_executors/adapters/` at build time — this
>   command has no registry of adapters to construct wired up anywhere yet,
>   so it needs to know how to instantiate each one; a simple explicit list
>   in the CLI module is fine, this doesn't need to be a plugin-discovery
>   system) and prints a human-readable report: for each adapter, whether
>   it's installed, its version, whether it's authenticated (where
>   detectable), and its capabilities. Must never trigger a login dialog —
>   every check must be non-destructive. Must degrade gracefully (report
>   "not installed"/"unavailable") for an adapter whose backing CLI/service
>   isn't present, not crash the whole command.
> - `praxis executors`: a status table (executor id, auth transport, status,
>   capabilities); `praxis executors --json` for machine-readable output of
>   the same data.
> - `praxis executors match --capability X [--capability Y ...] --explain`:
>   synthesizes a `requirement`-shaped dict from the given `--capability`
>   flags, runs `praxis_executors.matching.match` against the discovered
>   advertisements (respecting the default `AuthTransportPolicy`), and
>   prints the selected executor plus every candidate's eligibility/score
>   and, for ineligible ones, the reason (including the `policy_excluded`
>   distinction from issue #38).
>
> ## Acceptance
>
> - Unit tests for discovery aggregation, status formatting, `--json` output
>   shape, and `match --explain`'s output, all against a fake/stubbed
>   registry — no real adapter CLI/service required for the standard test
>   suite.
> - `praxis --version` and `praxis --help` (or whatever pre-existing
>   behavior `src/praxis_cli/main.py` had) continue to work unchanged.
> - Full test suite (`.venv/bin/python -m pytest`) passes.
>
> ## Delivery
>
> Open a PR against `main` referencing #45 with a closing keyword (`Closes
> #45`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **`codex_cli.py` does not exist in this worktree.** Confirmed via
   `git log --oneline --all -- src/praxis_executors/adapters/` and a directory
   listing: only `fake.py`, `subprocess_executor.py`, `claude_cli.py`, `ollama.py`
   are present (issue #41 has not landed). The explicit adapter list this bundle
   hardcodes has four entries, not five.

2. **All four shipped adapters are constructed, including `FakeCapabilityExecutor`.**
   The spec's own "Existing architecture" bullet names all four files —
   `{fake,subprocess_executor,claude_cli,ollama}.py` — as "the adapters shipped so
   far" feeding directly into "construct instances of whichever adapters actually
   exist." Do not narrow this to the three adapters that wrap a real external
   CLI/service; `docs/executors.md`'s framing of `FakeCapabilityExecutor` as
   "for tests" describes its *purpose*, not a license to omit it from the
   adapter list this task explicitly names.
   - `FakeCapabilityExecutor(executor_id, capabilities, script)`
     (`src/praxis_executors/adapters/fake.py:33-35`) and
     `SubprocessExecutor(executor_id, satisfies_kinds)`
     (`src/praxis_executors/adapters/subprocess_executor.py:29`) both require
     capability data at construction time that neither adapter can derive from a
     real environment (neither wraps one specific named external tool). Construct
     each with the same placeholder values its own existing unit test already
     uses as a fixture — `SubprocessExecutor(executor_id="executor-subprocess-1",
     satisfies_kinds=["code-execution"])` per `tests/test_subprocess_executor.py:25`,
     and an equivalent single-placeholder-capability `FakeCapabilityExecutor`
     with an empty `script={}` (discover/status/match never call `.launch()`, so
     `script` is never consulted; see criterion 6). Do not invent a different
     capability kind — reuse exactly what the adapter's own test already treats
     as its canonical example.
   - `ClaudeCliExecutor(executor_id)` and `OllamaExecutor(executor_id, base_url=...)`
     construct with no capability data — their `capabilities()`/`health()` probe
     the real environment (PATH lookup / local HTTP) at call time, per
     `claude_cli.py:44-74` and `ollama.py:163-227`.
   - One shared, single construction path (e.g. a module-level function returning
     the four instances) should back `discover`, the bare status table, and
     `match`, so all three subcommands see the same adapter set built the same
     way — avoid three separate ad hoc adapter lists that could drift.

3. **Preserving `main()`'s current behavior is a real hazard, not a formality — read this before writing `main()`.**
   Today, `src/praxis_cli/main.py:6-7` is:
   ```python
   def main() -> None:
       print(importlib.metadata.version("praxis-contracts"))
   ```
   It takes no arguments and does not inspect `sys.argv` at all — *any* invocation
   (no args, `--version`, `--help`, or garbage) prints the version and exits 0.
   `tests/test_praxis_cli.py:8-12` calls `main()` **bare, with no arguments**, and
   asserts on stdout.
   - `praxis_dashboard.cli` (`src/praxis_dashboard/cli.py:19,30`) is this repo's one
     existing precedent for a subcommand-dispatching CLI: `def main(argv: list[str]
     | None = None) -> int` calling `parser.parse_args(argv)`. Copying this pattern
     **verbatim** — a single top-level `argparse.ArgumentParser` whose
     `parse_args(None)` falls through to reading real `sys.argv[1:]` — would break
     the existing bare-`main()` test: under `pytest`, `sys.argv[1:]` is pytest's own
     invocation arguments (test paths, flags), not the empty/legacy invocation the
     test expects, and argparse would reject or misparse them.
   - Resolution: `main()` must special-case on the *first* token before handing
     anything to `argparse`. Read argv as `sys.argv[1:]` when no explicit `argv` is
     given (needed so the real installed console-script — which setuptools invokes
     as a bare `main()` call, per `pyproject.toml`'s `praxis =
     "praxis_cli.main:main"` — can see real user-typed args). If `argv` is empty or
     `argv[0] != "executors"`, preserve today's exact behavior unconditionally:
     print the version and return/exit 0 — do not route `--version`/`--help`/
     anything else through argparse at all. Only when `argv[0] == "executors"` does
     the rest (`argv[1:]`) go through an `argparse` subparser tree for `discover`,
     `match`, `--json`, etc. Under `pytest`, the real `sys.argv[1:]` at test time
     will not happen to start with the literal token `"executors"`, so the existing
     bare-`main()` test keeps passing unchanged, while `main(["executors",
     "discover"])` (or equivalent) becomes the way new tests drive the new
     subcommands explicitly, mirroring how `tests/test_dashboard_cli.py` always
     passes an explicit `argv` list rather than relying on process-global `sys.argv`.
   - Keep `main`'s return behavior consistent with what the existing test observes
     (prints to stdout, does not raise) for the legacy branch; the new `executors`
     branch may return an `int` exit code the console-script can propagate, same as
     `praxis_dashboard.cli.main`.

4. **CLI argument parsing uses stdlib `argparse`, not a new dependency.**
   `pyproject.toml`'s `[project] dependencies` is `["jsonschema>=4.18",
   "referencing>=0.28.4"]` — no CLI framework. `argparse` is stdlib and is the
   one already-established pattern in this repo (`praxis_dashboard/cli.py:11,19-27`).
   Use nested subparsers: a top-level `executors` subcommand with its own
   subparsers `discover` and `match`; bare `praxis executors` (no further
   subcommand) is the status-table path; `--json` is a flag on the bare
   `executors` invocation only (see criterion 8).

5. **`praxis executors discover`'s per-adapter report fields, and how each is derived — using only each adapter's public `Executor` interface, never a private/adapter-specific method:**
   - **installed**: no adapter exposes a distinct "installed" signal on the public
     `Executor` ABC (`src/praxis_executors/interface.py`) — only `health()` and
     `capabilities()`. Derive it per adapter, reusing exactly the technique the
     adapter itself already uses internally (never modify the adapter to expose a
     new method):
     - `claude_cli`: `shutil.which("claude") is not None`, the same call
       `ClaudeCliExecutor.health()` already makes internally (`claude_cli.py:63`,
       `_CLI_NAME = "claude"` at `claude_cli.py:25`). Safe to duplicate at the CLI
       layer: it is a read-only PATH lookup, not a call into `_probe_version`/
       `_detect_authenticated` (both private, prefixed `_`, and specific to that
       one adapter).
     - `ollama`: "installed" means the local Ollama HTTP service is reachable.
       `OllamaExecutor.health()` (`ollama.py:216-227`) already returns
       `UNAVAILABLE` only when the service can't be reached at all (`_OllamaUnreachable`),
       distinct from `DEGRADED` (reachable, zero models). So "installed" for
       Ollama = `health() != ExecutorAvailability.UNAVAILABLE`.
     - `subprocess`/`fake`: both `health()` unconditionally return `AVAILABLE`
       (`subprocess_executor.py:49-52`, `fake.py:48-49`) — neither wraps one
       specific external tool to be "installed" or not. Report "installed: n/a
       (built-in)" for both rather than inventing a check that doesn't apply.
   - **version**: **no adapter today exposes a queryable version string on the
     public interface.** `ClaudeCliExecutor._probe_version` (`claude_cli.py:76-80`)
     runs `claude --version` but discards the output entirely — it exists only as
     a non-destructive liveness probe, not a version accessor — and it is private.
     No other adapter attempts version detection at all. Report "version: unknown"
     for every adapter in this bundle rather than: (a) adding a new public method
     to the `Executor` ABC (out of scope — see **Explicitly out of scope**), or
     (b) reaching past the public interface into `claude_cli`'s private probe or
     re-parsing subprocess output the adapter itself discards. This is a real,
     named gap, not a formatting choice.
   - **authenticated**: only `claude_cli`'s `auth_transport` (`"subscription_cli"`)
     is a transport that can be "not authenticated" at all; `ollama`/`subprocess`/
     `fake` all advertise `"local"` (`ollama.py:199`, `subprocess_executor.py:44`;
     `fake.py`'s placeholder from criterion 2 should likewise use `"local"` or omit
     `auth_transport`), which `AuthTransportPolicy` treats as safe/no-login-required
     by default (`policy.py:22-25`). So: report "authenticated: n/a" for
     `ollama`/`subprocess`/`fake`. For `claude_cli`, combine the independently
     computed `installed` (above) with `health()`'s three-state result, reverse-
     engineering `ClaudeCliExecutor.health()`'s own documented branches
     (`claude_cli.py:62-74`: not installed → `UNAVAILABLE`; authenticated `False` →
     `UNAVAILABLE`, "deliberately no fallback branch"; authenticated `True` →
     `AVAILABLE`; authenticated unknown (`None`) → `DEGRADED`):
     - not installed → "authenticated: n/a (not installed)"
     - installed, `health() == AVAILABLE` → "authenticated: yes"
     - installed, `health() == DEGRADED` → "authenticated: unknown"
     - installed, `health() == UNAVAILABLE` → "authenticated: no" (installed was
       already confirmed True, so this branch of `UNAVAILABLE` can only be the
       adapter's own "authenticated is False" case per its current logic)
     This mapping is derived entirely from `claude_cli.py`'s current, already-committed
     `health()` logic — it is not invented policy layered on top.
   - **capabilities**: call `capabilities()` and report the `satisfies[].kind` list
     (and `auth_transport`) per capability entry. Must not crash the whole command
     if a call raises: `OllamaExecutor.capabilities()` raises `ExecutorError` when
     the service is unreachable or reports zero models (`ollama.py:166-169,205-208`)
     — catch `ExecutorError` per adapter and report "capabilities: unavailable
     (<reason>)" for that row, continuing with the rest. This is the spec's own
     "degrade gracefully... not crash the whole command" requirement, made concrete
     per adapter.
   - Never call any check that could trigger an interactive login prompt. Every
     technique above (`shutil.which`, `health()`, `capabilities()`,
     `--version` subprocess probe) is already established in this codebase as
     non-destructive; do not add a new probe that isn't.

6. **`praxis executors` (bare) status table and `--json`.**
   - Columns: executor id, auth transport, status, capabilities — exactly the four
     named in the spec. "Status" is `Executor.health()`'s `ExecutorAvailability`
     value (`"available"`/`"degraded"`/`"unavailable"`, the enum's own `.value`
     strings, `interface.py:24-29`). "Auth transport" is each capability's
     `auth_transport` field; if an advertisement's capabilities carry more than one
     distinct value (not the case for any of today's four adapters, but structurally
     possible per `capability.schema.json:36-39`), join the distinct values with a
     comma rather than picking one arbitrarily. "Capabilities" is the same
     `satisfies[].kind` list used in criterion 5, deduplicated per executor.
   - `--json` prints one JSON document (a list of the same four fields per
     executor) to stdout, matching this repo's one existing CLI-JSON precedent —
     `praxis_dashboard.cli.main`'s `--replay-only` path
     (`praxis_dashboard/cli.py:40`): a single `json.dumps(...)` call with no
     `indent`/pretty-printing, one line to stdout.
   - `discover` and `match` never call `.launch()`, so `FakeCapabilityExecutor`'s
     placeholder `script={}` (criterion 2) is never exercised and never needs real
     entries.

7. **`praxis executors match --capability X [--capability Y ...] --explain`.**
   - Requirement synthesis: each `--capability` value becomes one `required`-constraint
     entry — `{"promise": {"spec_version": "1.0.0", "kind": <value>}, "constraint":
     "required"}` — assembled into a `Requirement`-shaped dict (`spec_version:
     "1.0.0"`, `requirements: [...]`) per `schemas/v1/requirement.schema.json` and
     `promise.schema.json`. No `preferred`/`prohibited` flags are in scope for this
     bundle (see **Explicitly out of scope**) — every `--capability` flag maps to
     `required` only.
   - Eligibility/policy: run `matching.match(requirement, advertisements,
     is_eligible=...)` using the same default the registry uses —
     `policy.as_eligibility_callable(policy.AuthTransportPolicy(), advertisements)`
     (`registry.py:72-75`) — so `match --explain`'s eligibility decisions are
     identical to what `ExecutorRegistry.select` would actually do, not a
     reimplementation.
   - **Per-candidate eligibility/reason, using only `matching.match`'s public
     return shape — do not import or call `matching._satisfied_kinds` or any other
     private (`_`-prefixed) helper in `matching.py`.** `MatchResult.ranked`/`.selected`
     only ever contain candidates that already passed both the eligibility and
     required/prohibited-kind checks (`matching.py:90-99,123-145`); `.unsatisfied`
     is a *kind*-level explanation, not a *per-candidate* one. To report, for every
     discovered advertisement, whether it is eligible and why not when it isn't:
     call `matching.match(requirement, [advertisement], is_eligible=is_eligible)`
     **once per advertisement**, reusing the one shared `is_eligible` callable built
     above. This is a legitimate, fully public-API use of `match`: when called with
     a single-element advertisement list, `union_satisfied_any` (`matching.py:107-109`)
     is computed over that same one-element list, so the returned `unsatisfied`
     entries correctly distinguish "this candidate is ineligible under policy but
     would otherwise satisfy the requirement" (`policy_excluded=True`,
     `matching.py:150-161`) from "this candidate doesn't satisfy the requirement at
     all" — exactly the per-candidate reason the spec asks for, including the
     `policy_excluded` distinction from issue #38, without touching `matching.py`.
   - **"Score"**: `MatchCandidate` (`matching.py:14-17`) does not expose a raw
     numeric score, and this bundle synthesizes only `required` kinds (no
     `--prefer`), so the ranking tie-break's `preferred_score` term is always `0`
     for every run. Present "score" as each eligible/satisfying candidate's rank
     position in `MatchResult.ranked` from the normal (full-advertisement-list)
     `match()` call (1st, 2nd, ...), not an invented numeric value — call the normal
     multi-candidate `match()` once for the winning ranked order, and the
     one-per-advertisement calls above only for the candidates *not* in that
     `ranked` list, to get their exclusion reason.
   - `--explain` is an optional flag (`action="store_true"`, default `False`).
     When absent, print only the selected executor id (or "no executor selected"
     plus the aggregate `unsatisfied` reasons if `selected is None`). When present,
     additionally print the full per-candidate eligibility/score/reason breakdown
     above. Only `--explain`'s output is covered by this bundle's acceptance tests
     (per the original Acceptance section); the flag exists because the spec's own
     command syntax names it explicitly, not because both modes are equally
     load-bearing here.

8. **`praxis --version` / `praxis --help` continue to print the version and exit 0** — not real `argparse`-driven `--help` text, since that was never true today either (criterion 3): today's `main()` prints the version for *any* argv, including `--help`. Preserve exactly that, per criterion 3's dispatch rule.

## Explicitly out of scope

- `--prefer <kind>` / `--prohibit <kind>` flags for `match`, or any CLI surface for
  the `preferred`/`prohibited` requirement constraints. The spec's command syntax
  names only `--capability`; `matching.py` already supports `preferred`/`prohibited`
  internally, but wiring CLI flags for them is a separate increment.
  - `--json` output for `discover` or `match --explain`. The spec ties `--json`
  explicitly to the bare `praxis executors` status table only.
- Adding a `version()` (or similar) method to the `Executor` ABC, or to any
  individual adapter, to make real version strings queryable. This bundle reports
  "version: unknown" instead (criterion 5) — extending the shared interface is a
  larger, separate change with its own blast radius across every adapter, not
  something this CLI-only bundle should do silently.
- Reaching into any adapter's private (`_`-prefixed) methods or module-level
  helpers (`ClaudeCliExecutor._detect_authenticated`/`_probe_version`,
  `matching._satisfied_kinds`, etc.) from the CLI module. Every field this bundle
  reports is derived from each module's already-public surface only.
- Registering any constructed adapter instance into a default/shared
  `ExecutorRegistry` as part of application wiring. `docs/executors.md`'s "Adding a
  new executor adapter" section states construction-and-registration is "the
  caller"'s job (`docs/executors.md:252-253`); this CLI constructs adapters
  transiently, per-invocation, purely to discover/report/match against them — it
  does not stand up or persist a registry.
- `codex_cli.py` / any fifth adapter. Confirmed absent from this worktree
  (criterion 1); the explicit adapter list is exactly the four that exist today.
- Any change to `src/praxis_executors/interface.py`, `matching.py`, `policy.py`,
  `registry.py`, or any `schemas/v1/*.schema.json` file. This bundle is CLI-only —
  `src/praxis_cli/main.py` (and any new CLI-only submodules it needs) plus tests.
- Real `argparse`-generated `--help` text/usage strings for the top-level `praxis`
  command beyond what's needed for `executors`'s own subparsers. The bare
  `praxis --help` legacy path (criterion 3/8) intentionally does not go through
  `argparse` at all.

## Assumptions made

- **Construct all four existing adapters, with placeholder construction args for
  `subprocess`/`fake` copied from their own unit tests** (criterion 2). Evidence:
  the original spec's own architecture section names all four files as "shipped so
  far" feeding the construction instruction; `tests/test_subprocess_executor.py:25`
  is the sibling example for `SubprocessExecutor`'s placeholder args.
  Resolve-or-name: in scope (the spec names these adapters directly), defensible
  default from the adapter's own existing test fixture, doesn't change any stated
  acceptance criterion, no security/compat/deployment impact (adapters are
  constructed transiently for reporting only, never registered), trivially
  correctable later if a reviewer wants different placeholder values.

- **`main()` dispatches on `argv[0] == "executors"` before touching `argparse`,
  falling back unconditionally to today's print-version-and-exit behavior
  otherwise** (criterion 3). Evidence: `src/praxis_cli/main.py:6-7`'s actual current
  body (ignores argv entirely) and `tests/test_praxis_cli.py:8-12`'s bare `main()`
  call, read together with `praxis_dashboard/cli.py:19,30`'s `parse_args(None)`
  convention — copying that convention verbatim would route pytest's own argv
  through `argparse` and break the existing test. Resolve-or-name: in scope (the
  spec explicitly demands preserving current behavior), evidence is the actual
  current source and the actual existing test, doesn't loosen or change any stated
  criterion (it's the mechanism that keeps criterion 8 true), no security/compat
  impact, and is trivially checkable by running the existing test suite.

- **Version is reported as "unknown" for every adapter; authenticated/installed
  are derived only from each adapter's already-public `health()`/PATH-lookup
  behavior, never from a private method** (criterion 5). Evidence:
  `claude_cli.py:76-86`'s `_probe_version`/`_detect_authenticated` are private and
  the former discards its own subprocess output; no other adapter attempts either
  check. Resolve-or-name: in scope, the conservative reading of "(where detectable)"
  extended to version for the same underlying reason it's already stated for
  authentication, doesn't touch any adapter's code or the `Executor` ABC (avoiding
  exactly the public-API blast radius the resolve-or-name test asks about),
  correctable later by whoever adds a real version-reporting mechanism.

- **`match --explain`'s per-candidate breakdown is built by calling the public
  `matching.match()` once per single-candidate advertisement list, reusing the
  shared `is_eligible` callable, rather than touching any private helper in
  `matching.py`** (criterion 7). Evidence: reading `matching.py:79-201` directly —
  `union_satisfied_any` is computed over whatever advertisement list is passed to
  that call, so a single-element call correctly reproduces the `policy_excluded`
  distinction for that one candidate. Resolve-or-name: in scope, the fill is a
  reuse pattern derived directly from already-committed code (not a new algorithm),
  doesn't change `matching.py` or any acceptance criterion's meaning, no
  security/compat impact, and is easily verified against `matching.py`'s existing
  unit tests / `docs/executors.md`'s documented semantics.

- **CLI argument parsing uses stdlib `argparse`, following `praxis_dashboard/cli.py`'s
  `parse_args(argv)` shape for the `executors` subtree; `--json` uses a single
  un-indented `json.dumps(...)` call, following `praxis_dashboard/cli.py:40`**
  (criteria 4, 6). Evidence: `pyproject.toml`'s dependency list has no CLI
  framework, and `praxis_dashboard/cli.py` is this repo's one existing
  subcommand-CLI precedent. Resolve-or-name: in scope, defensible default from the
  one sibling example in the repo, doesn't add a dependency (stdlib only), no
  security/compat impact, correctable later.

## Open questions

None. The two gaps that looked open at first read — what capability data to
construct `SubprocessExecutor`/`FakeCapabilityExecutor` with, and how `main()` can
add subcommand dispatch without breaking its own already-checked-in bare-`main()`
test — both resolved against evidence already in the repository (the adapters'
own unit-test fixtures, `main.py`'s actual current source read literally, and
`praxis_dashboard/cli.py`'s existing dispatch convention read carefully enough to
see where copying it verbatim would have silently broken an existing test). Every
other gap (version/installed/authenticated detection, the `--explain` per-candidate
breakdown, `--json`'s exact shape) resolved the same way, against the `Executor`
ABC, `matching.py`'s actual algorithm, `docs/executors.md`, and the two existing
adapters' already-committed `health()` logic.
