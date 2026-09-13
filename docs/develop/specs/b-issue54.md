# Bundle b-issue54 — Enhanced Spec

> **Enhancement verdict: `NEEDS_CONTEXT` on sequencing.** The spec's own instruction is
> "Do this last … so it documents what actually exists, not aspirational behavior."
> Verification against `origin/main` @ `9366f5b` shows that **seven of the bundles this
> documentation set exists to document have not merged**, and all seven are parked at
> `awaiting_human`. The unmerged work is not peripheral — it is `praxis doctor`, the
> executor configuration/precedence surface, and the MLX / local Hugging Face adapter,
> which between them are the spine of three of the five files this bundle must write.
> Everything else in this spec has been resolved and is ready to plan the moment that one
> question is answered. See **Open questions**, item 1, for the binary decision required.

## Original content

```markdown
# Bundle b-issue54: Documentation set: USER_GUIDE, INSTALL_GUIDE, CUSTOMIZATION_GUIDE, EXECUTOR_DEVELOPMENT, README, /develop status

Issue: #54
Branch: develop/b-issue54
Base: origin/main
Footprint: docs/USER_GUIDE.md, docs/INSTALL_GUIDE.md, docs/CUSTOMIZATION_GUIDE.md, docs/EXECUTOR_DEVELOPMENT.md, README.md

## Source issue body

### Task

Write the full documentation set required by #37. Do this last (or near-last) so it documents what actually exists, not aspirational behavior — cross-check every command/example against the real, merged CLI (#H, #I, #J) before writing it into a guide.

- `docs/USER_GUIDE.md`: what Praxis is (models are executors, not the architecture), first run (`praxis doctor` / `praxis executors discover` / `praxis executors`), logging into each provider (without exposing credentials to Praxis), explicit vs. auto executor selection, running workflows, dashboard walkthrough, troubleshooting (CLI missing, auth required, Ollama not running, model missing, no executor satisfies policy, graph failure, dashboard unavailable).
- `docs/INSTALL_GUIDE.md`: supported platforms (macOS Apple Silicon confirmed; macOS Intel and Linux — state actual tested status, don't claim untested support; Windows explicitly marked untested/unsupported unless someone actually tests it), prerequisites, Praxis install, each provider's CLI install/login, Ollama install, optional MLX setup, local Hugging Face model setup, dashboard setup, ending with `praxis doctor` and its expected successful output.
- `docs/CUSTOMIZATION_GUIDE.md`: executor preferences, the no-metered-API/no-API-key policy, default models, per-task model preferences, capability requirements, graph/overlay customization, evidence requirements, evaluation thresholds, recovery behavior, human approval gates, dashboard preferences, adding local models, adding custom executors (link to EXECUTOR_DEVELOPMENT.md), and the actual configuration precedence order implemented in issue #N.
- `docs/EXECUTOR_DEVELOPMENT.md`: how to add a new executor (interface, discovery, registration, capabilities, policy metadata, execution, evidence, testing, security requirements) with one complete minimal example executor (a real, runnable example, not pseudocode — mirror `adapters/fake.py`'s existing pattern if that's the right minimal template).
- `README.md`: update further per #37 section 30 (two-minute understanding, architecture diagram, minimal quickstart, links to the four guides above) — the repo has already been renamed to "Praxis — Universal AI Execution Fabric"; build on that, don't redo it.
- `/develop` migration status: fold in and update the existing Phase B roadmap (from the develop-v5 discussion prior to this epic) — state plainly what's proven, what's deferred, and that the known-working `/develop` v4 implementation is untouched until parity is proven, per #37 section 31/32.

### Acceptance

- Every command shown in every guide actually runs as documented against the real merged code — verify each one, don't transcribe from the spec.
- No guide claims "production ready" over a documented remaining limitation.
- `pytest` passes (no code changes expected, but confirm nothing broke).

Part of #37. Land last, after the CLI/adapters/dashboard/config work it documents.

## Orchestrator note

This bundle depends on the CLI/adapter/dashboard/config work landing first per the issue's own "land last" instruction. Several of those bundles (executor adapters, CLI wiring, dashboard, config) are currently blocked awaiting human review in this same run (see HANDOFF.md / awaiting_human bundles). The tech lead should verify what has actually merged to origin/main before writing docs, and flag via NEEDS_CONTEXT/BLOCKED if the prerequisite functionality genuinely isn't present yet to document accurately, rather than writing aspirational documentation.
```

## Verified state of `origin/main` @ `9366f5b`

This is the evidence base for every clarification below. It was established by reading the
merged source, not by transcribing the spec.

### What exists and is documentable today

| Surface | Where it lives | Notes for the writer |
| --- | --- | --- |
| `praxis` console script | `pyproject.toml:14-15` (`[project.scripts]`) | The **only** console script. |
| `praxis --version` (and any non-`executors` first argument) | `src/praxis_cli/main.py:42-44` | Prints the `praxis-contracts` version and exits 0. Known limitation, already recorded in `docs/develop/plans/b2-issue45.md`: `praxis bogus` takes this same path, so an unknown subcommand prints a version and exits 0 with no diagnostic. |
| `praxis executors` / `praxis executors --json` | `src/praxis_cli/status_cmd.py` | Four columns exactly: `executor_id`, `auth_transport`, `status`, `capabilities`. `--json` is rejected on any subcommand (`main.py:49-52`). |
| `praxis executors discover` | `src/praxis_cli/discover_cmd.py` | Per-adapter block: `installed`, `version`, `authenticated`, `auth_transport`, `capabilities`. |
| `praxis executors match --capability K [--explain]` | `src/praxis_cli/match_cmd.py` | `--capability` is repeatable and required; every flag maps to a `required` promise only. |
| Executor adapters (source) | `src/praxis_executors/adapters/` | Five: `fake.py`, `subprocess_executor.py`, `claude_cli.py`, `codex_cli.py`, `ollama.py`. |
| Adapters the CLI actually wires | `src/praxis_cli/adapters.py:31-46` (`_ADAPTER_FACTORIES`) | **Four**: `executor-subprocess-1`, `executor-fake-1`, `executor-claude-cli-1`, `executor-ollama-1`. `CodexCliExecutor` is *not* in `build_adapters()`. |
| No-metered-API / no-API-key policy | `src/praxis_executors/policy.py:45-73`; the deny set at `policy.py:25` | `AuthTransportPolicy`; `_UNSAFE_BY_DEFAULT_AUTH_TRANSPORTS = {"metered_api", "api_key"}`. A library class, not a user setting. |
| Executor extension path | `docs/executors.md`, "Adding a new executor adapter" | Six numbered steps; `adapters/fake.py` is the minimal template the spec asks the example to mirror. |
| Dashboard | `src/praxis_dashboard/cli.py`, `__main__.py` | Invoked as `python -m praxis_dashboard`. Flags: `--graph` (required), `--run-dir` (required), `--lease-dir`, `--host` (default `127.0.0.1`), `--port` (default `0`), `--replay-only`. |
| Existing reference docs | `docs/{ontology,runtime,executors,evidence,resources,policy,eval,learning,overlays,dashboard,distribution}.md` | ~2,600 lines already written. Guides should link into these, not restate them. |
| README, already accurate | `README.md:296-448` | Installation, quickstart, dashboard usage, CLI usage, and a "Development Plan" roadmap section already exist and already match merged behavior. |

### What the spec asks the guides to document, that does not exist

| Spec requirement | Reality on `origin/main` | Owning bundle (unmerged) |
| --- | --- | --- |
| `praxis doctor` — the USER_GUIDE first-run flow and the INSTALL_GUIDE's closing step | No such subcommand. `_build_parser()` registers only `executors`. Repo-wide, `doctor` appears solely in comments about the *third-party* `codex doctor`. | `b-issue46-47` — "praxis doctor + praxis run --executor CLI wiring", `awaiting_human` |
| `praxis run --executor` / "running workflows", "explicit vs. auto executor selection" | No `run` subcommand. Driving a graph is library-only today (`README.md:318-375`). | `b-issue46-47`, `awaiting_human` |
| "the actual configuration precedence order implemented in issue #N", executor preferences, default models, per-task model preferences, dashboard preferences | **No configuration system exists at all.** No config-file loader, no `PRAXIS_*` environment variables, no precedence rules. The only `os.environ` reads are the two adapters copying the ambient env into a subprocess. | `b-issue51` — "Executor configuration surface + validation + precedence", `awaiting_human` |
| "optional MLX setup", "local Hugging Face model setup" | No MLX adapter. Zero occurrences of "hugging" anywhere in the repository. | `b-issue44` — "MLX / local Hugging Face executor adapter", `awaiting_human` |
| Dashboard walkthrough covering the executors panel and selection reasoning | Dashboard exists, but without the executors panel / selection-reasoning / recovery views. | `b-issue50` — "Dashboard: executors panel, selection reasoning, recovery visualization", `awaiting_human` |
| Provider coverage implying Copilot | No Copilot adapter. `docs/executors.md:257` names it explicitly as not existing yet. | `b-issue42` — "GitHub Copilot executor adapter", `awaiting_human` |
| Troubleshooting "no executor satisfies policy" as a user-facing failure path, human approval gates | Partial: `match --explain` reports policy exclusion, but the human-executor decision path is unmerged. | `b-issue49` — "Executor evidence/eval/learning wiring + human-executor decision", `awaiting_human` |

Mapping of the spec's placeholder issue letters, recovered from sibling bundle specs in this
run: `#H`/`#I`/`#J` (CLI) = `b2-issue45` (**merged**, PR #78) plus `b-issue46-47` (**not merged**);
`#N` (config precedence) = `b-issue51` (**not merged**).

## Clarified acceptance criteria

Numbered so the planner can map tasks onto them and a reviewer can check each independently.
Criteria 1-3 are the original acceptance list, made concrete. Criteria 4-12 are the per-file
criteria the original stated as prose bullets.

1. **Every command in every guide is executed before it is written down**, and the guide shows
   that run's real output — not a hand-composed approximation. Concretely, at minimum:
   `praxis --version`, `praxis executors`, `praxis executors --json`, `praxis executors discover`,
   `praxis executors match --capability <kind> --explain`, `python -m praxis_dashboard --graph
   examples/sample-graph.json --run-dir <dir> --replay-only`, `pip install -e ".[dev]"`, `pytest`.
   A command the writer could not run (needs a `claude` binary, a live Ollama service, a second
   platform) is either omitted or explicitly marked as unverified with the reason.
2. **No guide names a command, flag, subcommand, adapter, config key, or environment variable
   that does not exist on the branch being merged.** This is the criterion that fails first if
   the bundle is written ahead of its prerequisites, and it is checkable mechanically: every
   `praxis …` invocation in the four guides parses under `praxis_cli.main._build_parser()`, and
   every `python -m praxis_dashboard …` invocation parses under `praxis_dashboard.cli.parse_args()`.
3. **`pytest` passes** from the repo root with no changes to `src/`. Docs-only bundle: any `src/`
   diff is a scope breach that needs its own justification.
4. **`docs/USER_GUIDE.md`** covers, in order: what Praxis is with the "models are executors, not
   the architecture" framing stated explicitly; the first-run sequence using the commands that
   exist; how to authenticate each supported provider **through that provider's own CLI**, with
   an explicit statement that Praxis never receives, stores, or reads a credential (grounded in
   `README.md:393` — every CLI check is read-only and cannot trigger an interactive login prompt);
   executor selection; the dashboard; and a troubleshooting section.
5. **The USER_GUIDE troubleshooting section has one entry per named failure mode**, each giving
   the observable symptom and the fix. The seven named modes are: backing CLI missing, auth
   required, Ollama not running, model missing, no executor satisfies policy, graph failure,
   dashboard unavailable. Grounding for the first five: an adapter whose backing CLI or service is
   absent degrades its own row to `capabilities: unavailable (<reason>)` rather than failing the
   command (`README.md:409`); `status: unknown` means the health probe itself raised and is *not*
   the same as `degraded` (`README.md:411`); `--explain` attributes a policy exclusion to
   `AuthTransportPolicy` by name.
6. **`docs/INSTALL_GUIDE.md` states tested status per platform, and never implies support it has
   not observed.** macOS Apple Silicon: confirmed. macOS Intel, Linux: state the actual tested
   status, which is "not tested by this bundle" unless the writer tests it. Windows: explicitly
   marked untested and unsupported. The guide ends with a runnable verification step and its
   real expected output.
7. **`docs/CUSTOMIZATION_GUIDE.md` documents only customization surfaces that exist**, and says
   plainly, in one place, which of the spec's listed customization axes are not configurable
   today. The surfaces that do exist and must be covered: executor eligibility policy
   (`AllowListPolicy`, `DenyListPolicy`, `AuthTransportPolicy`), the no-metered-API/no-API-key
   default and how to widen it deliberately, capability requirements and matching, graph/overlay
   customization (`docs/overlays.md`), evidence requirements (`docs/evidence.md`), evaluation
   thresholds (`docs/eval.md`), recovery behavior (`docs/policy.md`, `docs/runtime.md`), and
   adding custom executors (link to `docs/EXECUTOR_DEVELOPMENT.md`, do not duplicate it).
8. **`docs/EXECUTOR_DEVELOPMENT.md` contains one complete, runnable minimal executor**, not
   pseudocode, mirroring `src/praxis_executors/adapters/fake.py`. It implements all six members of
   the `Executor` ABC (`src/praxis_executors/interface.py:75-99`): `capabilities`, `health`,
   `launch`, `status`, `cancel`, `result`. The example is copy-pasteable and the writer has run it.
9. **`docs/EXECUTOR_DEVELOPMENT.md` states the capability-naming rule as a hard requirement**:
   `capabilities()` advertises capability `kind`s (`text-generation`, `code-execution`), never a
   vendor or model name — this is the ontology's core rule and the same rule README states as
   "Graphs request promises and capabilities. They do not name models or vendors."
10. **`docs/EXECUTOR_DEVELOPMENT.md` has a security-requirements section** covering, at minimum:
    declaring `auth_transport` honestly (a wrong declaration silently defeats `AuthTransportPolicy`);
    never logging or returning credentials in `ExecutionResult` or evidence; and the
    no-secrets-by-default rule for `ProofRecord` (`docs/evidence.md`).
11. **`README.md` gains links to all four guides** and keeps its existing accurate content. The
    rename to "Praxis — Universal AI Execution Fabric", the architecture diagram, the quickstart,
    the Installation and Usage sections, and the Development Plan section already exist and are
    already correct — extend them, do not rewrite them.
12. **The `/develop` migration status lands as an update to `README.md`'s existing "Development
    Plan" section** (`README.md:426-448`) and states plainly: what is proven, what is deferred,
    and that the working `/develop` v4 implementation is untouched until parity is proven. The
    existing migration rule at `README.md:445-447` already states the last of these and should be
    kept verbatim, not reworded.

## Explicitly out of scope

- **Any change under `src/`.** This is a documentation bundle. If writing a guide surfaces a code
  defect, file it; do not fix it here.
- **Implementing `praxis doctor`, `praxis run`, a configuration system, or an MLX / Hugging Face
  adapter** so that the guides have something to describe. Those are `b-issue46-47`, `b-issue51`,
  and `b-issue44`. Writing this bundle must not preempt them.
- **Rewriting the existing reference docs** under `docs/` (`ontology`, `runtime`, `executors`,
  `evidence`, `resources`, `policy`, `eval`, `learning`, `overlays`, `dashboard`). The four new
  guides are task-oriented and link into those; they do not replace or duplicate them.
- **Wiring `CodexCliExecutor` into `praxis_cli.adapters.build_adapters()`.** The gap is real and
  named below, but closing it is a code change.
- **Reconciling `docs/executors.md:257` ("Five concrete adapters ship today") with the CLI's
  four-adapter wiring.** Also real, also named below, also not this bundle's to fix — but no new
  guide may repeat the claim that the CLI reports five adapters.
- **Publishing to PyPI, or any packaging/distribution change.** `README.md:298` states Praxis is
  installed from a source checkout and is not yet on PyPI; the INSTALL_GUIDE says the same.
- **Testing macOS Intel, Linux, or Windows.** If nobody tests them, the guide says they are
  untested. Acquiring the hardware is not in this bundle.
- **The actual `/develop` runtime cutover.** README already scopes it as a roadmap item, not
  filed issues; the migration-status update reports on it, it does not perform it.

## Assumptions made

1. **The spec's acceptance criteria govern its illustrative command list, not the reverse.** Where
   the bullet list names a command that does not exist (`praxis doctor`), the guide documents the
   commands that do exist instead of the ones the bullet names. *Evidence:* the spec's own first
   acceptance criterion — "Every command shown in every guide actually runs as documented against
   the real merged code — verify each one, don't transcribe from the spec" — plus the task
   preamble, "so it documents what actually exists, not aspirational behavior." Resolve-or-name:
   in scope, on-record default in the document itself, does not shrink any criterion (it is the
   criterion), no security or compatibility surface, trivially correctable.
2. **Unimplemented capability is named as unimplemented rather than omitted silently.** Where the
   spec asks for coverage of something that does not exist (MLX, Hugging Face, config precedence),
   the guide states that it is not implemented today and names the tracking issue, rather than
   dropping the topic without trace. *Evidence:* the spec's second acceptance criterion — no guide
   may claim readiness over a documented remaining limitation — and the precedent set by
   `README.md:19-29` and `docs/executors.md:257`, both of which already name absent adapters
   explicitly rather than omitting them. This is why criterion 7 requires the statement "in one
   place" rather than allowing silence.
3. **The dashboard is documented as `python -m praxis_dashboard`, never as a `praxis-dashboard`
   console script.** *Evidence:* `pyproject.toml:14-15` declares exactly one script, `praxis`;
   `README.md:379` already documents the `python -m` form.
4. **The "/develop migration status" deliverable updates `README.md`'s existing "Development Plan"
   section rather than creating a new roadmap file.** *Evidence:* the bundle footprint names
   `README.md` and no fifth doc file; the string "Phase B" appears nowhere in the repository,
   while `README.md:426-448` is the only existing roadmap-shaped section and already carries the
   Epic #1 / Epic #26 phasing and the migration rule. Resolve-or-name: in scope (footprint), an
   on-record location exists, the criterion ("state plainly what's proven, what's deferred") is
   unchanged, no security surface, trivially relocatable if a reviewer disagrees.
5. **"Each provider" means the providers with a merged adapter**, i.e. Claude (via the `claude`
   subscription CLI) and Ollama, plus Codex flagged as shipped-but-not-CLI-wired. Copilot, MLX,
   Hugging Face, and OpenCode are named as not-yet-supported. *Evidence:* `src/praxis_cli/adapters.py:29-46`
   for what the CLI actually reports, `docs/executors.md:257-269` for the five source adapters and
   the explicit statement that Copilot/OpenCode/MLX do not exist.
6. **Issue #37's "section 30" is read through README's existing structure**, since the epic body
   could not be retrieved (`gh` is unavailable in the enhancer's environment). Section 30's four
   named deliverables — two-minute understanding, architecture diagram, minimal quickstart, links
   to the four guides — map onto `README.md:1-30` (understanding), `README.md:51-81` (diagram),
   `README.md:318-375` (quickstart), and a new links block. Only the links block is missing.
   *Evidence:* the spec itself enumerates section 30's contents inline, so the epic text is not
   needed to act on it. Flagged rather than silently assumed because a reviewer with `gh` access
   can confirm in one command.

## Open questions

1. **Should this bundle be written now against merged-only reality, or held until its
   prerequisites merge?** This is the one decision the resolve-or-name test does not permit the
   enhancer to make, and it changes what gets built.

   The facts: `origin/main` @ `9366f5b` does not contain `praxis doctor`, `praxis run --executor`,
   any configuration/precedence surface, any MLX or Hugging Face adapter, the dashboard executors
   panel, the Copilot adapter, or the human-executor decision path. All seven owning bundles
   (`b-issue42`, `b-issue44`, `b-issue46-47`, `b-issue48`, `b-issue49`, `b-issue50`, `b-issue51`)
   are parked at `awaiting_human` in this same run. The spec's closing line — "Land last, after the
   CLI/adapters/dashboard/config work it documents" — is the on-record default, and it says wait.

   - **Option A — defer (the spec's own default).** Hold `b-issue54` until the seven bundles clear
     human review and merge, then plan against the widened surface. Cost: the documentation set
     ships later. Benefit: it is written once, and `docs/CUSTOMIZATION_GUIDE.md` has actual
     configuration to document rather than a statement that configuration does not exist.
   - **Option B — write now, against merged-only reality.** All five files are produced under the
     clarified criteria above, which are already written to be satisfiable on today's `main`.
     Cost: `docs/USER_GUIDE.md` has no first-run `doctor` step and no workflow-running section,
     `docs/INSTALL_GUIDE.md` has no MLX/Hugging Face sections and ends on `praxis executors`
     rather than `praxis doctor`, and `docs/CUSTOMIZATION_GUIDE.md`'s central subject reduces to
     library-level policy classes plus a "no configuration file exists yet" statement. Three of
     the five files then need substantial rewriting when the seven bundles land — a rewrite debt
     that should be filed as a follow-up issue at merge time, not left implicit.

   Why this is not resolvable here: picking Option B changes what acceptance criteria 4, 6, and 7
   deliver (resolve-or-name condition 3), and the only default on record points the other way
   (condition 2). The orchestrator note attached to this bundle anticipated exactly this and
   directed that it be flagged rather than decided.

   **The planner can proceed under either answer without re-enhancement** — criteria 1-12 above
   are written to hold in both cases, and the "does not exist" table is the delta to re-verify
   against `origin/main` at planning time.

2. **Non-blocking: issue #37's sections 30, 31, and 32 were not read directly.** `gh` is
   unavailable in this environment, so the epic body could not be fetched. Assumption 6 records
   how section 30 was reconstructed from the spec's own inline enumeration; sections 31-32 are
   covered by criterion 12, which restates what the spec says they require. A reviewer or the
   planner with `gh` access should spot-check `gh issue view 37` for any section-30/31/32
   requirement the spec's inline summary dropped. This does not gate planning.
