# Enhanced spec: b-issue51

## Original content

> # Bundle b-issue51: Executor configuration surface + validation + precedence
>
> ## Issues
> - #51 — Executor configuration surface + validation + precedence
>
> ## Scope
> Add a clean, validated configuration surface for the `executors:` block (policy,
> preferences, per-provider enable flags) without requiring users to edit Python source:
> - Define and validate the config schema (new `schemas/v1/executor-config.schema.json`
>   or extend an existing config schema if one already exists — check first).
> - Config precedence: Praxis defaults -> user config -> environment overrides (confirm
>   exact precedence order against existing config-loading conventions in the repo).
>
> ## Base
> `origin/main` at commit 9366f5b.

## Environment note

This worktree has **no `.venv`** at enhancement time, unlike sibling bundles whose specs
state one already exists. `pyproject.toml` sets `[tool.pytest.ini_options] pythonpath = ["src"]`
and `testpaths = ["tests"]`, so the suite is runnable from the repo root once a
`pip install -e .[dev]` environment exists. Whoever implements this creates or is given
that environment; it is not a spec gap.

The issue body could not be fetched from GitHub in this session — `gh issue view 51` was
denied by the sandbox. The bundle spec quoted above is the authoritative spec text for
this pass. Every clarification below is grounded in repository code and docs, not in
unfetched issue text.

## Clarified acceptance criteria

The original spec states two scope bullets and no acceptance criteria. The twelve below
are the criteria a reviewer can hold the delivered work against.

1. **A new schema file, not an extension of an existing one.** The "check first"
   instruction resolves to: no existing schema covers this. The only file with "config"
   in its name, `src/praxis_contracts/schemas/v1/candidate-config.schema.json`, is an
   evaluation/promotion artifact whose `configuration` payload is documented as
   "intentionally opaque -- this package must never assume its internal shape"; it is not
   a user-editable settings surface. Nothing else in `schemas/v1/` describes deployment
   configuration. Create a new file.

2. **The schema's path in the original spec is stale; the correct path is
   `src/praxis_contracts/schemas/v1/executor-config.schema.json`.** Issue #52 moved every
   schema under `src/praxis_contracts/` and `pyproject.toml` already globs
   `praxis_contracts/schemas/v1/*.schema.json` as package data. A file added there is
   packaged and resolvable through `praxis_contracts.schema_paths.SCHEMA_DIR` with no
   `pyproject.toml` change. A top-level `schemas/v1/` directory no longer exists.

3. **The schema follows the conventions every sibling v1 schema follows**: draft 2020-12
   `$schema`, `$id` of `https://schemas.praxis.dev/v1/executor-config.schema.json`,
   `spec_version` in `required` with pattern `^1\.\d+\.\d+$`, and
   `additionalProperties: false` at every object level. Validation goes through
   `praxis_contracts.validator.validate_document`, which already enforces the
   object-at-top-level, `spec_version`-major, and full-structural checks in that order.

4. **The config document is JSON, not YAML.** `pyproject.toml` declares only `jsonschema`
   and `referencing` as runtime dependencies; no YAML parser is available and adding one
   is out of scope (see below). The `executors:` notation in the original spec is
   README-style illustrative YAML, not a file format commitment — it resolves to a JSON
   object with a top-level `executors` key.

5. **The `executors` block has exactly three sub-blocks**, one per phrase in the original
   scope line, each mapping onto machinery that already exists:

   - `executors.providers` — an object keyed by executor id, each value
     `{"enabled": boolean}` with `additionalProperties: false`. Keys are **open strings,
     not a fixed enum**, so a new adapter needs no schema change.
   - `executors.policy` — `allowed_auth_transports` (array of string or `null`),
     `denied_auth_transports` (array of string), `allowed_executor_ids` (array of string
     or `null`), `denied_executor_ids` (array of string). These map one-to-one onto the
     constructor fields of `AuthTransportPolicy`, `AllowListPolicy`, and `DenyListPolicy`
     in `src/praxis_executors/policy.py`. Auth-transport values are constrained to the
     five recognized values `subscription_cli`, `oauth_cli`, `local`, `metered_api`,
     `api_key` — the same set as `policy._RECOGNIZED_AUTH_TRANSPORTS` and
     `capability.schema.json`'s enum.
   - `executors.preferences` — `preferred_capability_kinds`, an array of strings matching
     the capability-kind pattern `^[a-z0-9]+(-[a-z0-9]+)*$`.

6. **Precedence is defaults → user config file → environment, with environment winning**,
   exactly as the original spec states. The "confirm against existing conventions"
   instruction resolves to: **no config-loading convention exists in this repository to
   confirm against**. The only `PRAXIS_`-prefixed variable anywhere is
   `PRAXIS_STUB_ADAPTERS` in `tests/test_praxis_cli_executors.py`, a test-only stub
   switch, and nothing in `src/` reads a user config file, a home directory, or an XDG
   path. The spec's stated order therefore stands unopposed.

   Merge semantics, spelled out because the planner would otherwise invent them: objects
   merge key-by-key across layers; scalars replace; **arrays replace wholesale, never
   union**. Resolution is total — the loader returns a fully-populated config with every
   default filled, so no consumer ever handles a missing key.

7. **Defaults reproduce today's behavior byte for byte.** With no config file present and
   no `PRAXIS_EXECUTORS_*` variable set: all four ids in
   `praxis_cli.adapters._ADAPTER_FACTORIES` (`executor-subprocess-1`, `executor-fake-1`,
   `executor-claude-cli-1`, `executor-ollama-1`) are enabled; the policy equals a
   default-constructed `AuthTransportPolicy()` (empty denies, `allowed_auth_transports`
   of `None`, therefore fail-closed on `metered_api` and `api_key`); and no preferred
   kinds are added. A test must assert that `praxis executors`,
   `praxis executors discover`, and `praxis executors match` produce identical output to
   the current implementation under an empty environment.

8. **Config-file discovery order**, most specific first: an explicit path passed by the
   caller; else `$PRAXIS_CONFIG`; else `~/.config/praxis/config.json`; else
   defaults-only. A missing file at the default user location is **not** an error. A
   missing file at an explicitly named path — the caller's argument or `$PRAXIS_CONFIG` —
   **is** an error, because the user named a file that isn't there.

9. **Environment overrides are a fixed, enumerated set, not a generic path mapper.** A
   generic `PRAXIS_EXECUTORS__<PATH>` scheme cannot round-trip executor ids, which
   contain hyphens that upper-snake-casing makes indistinguishable from underscores. Ship
   these five instead, each mapping to exactly one config leaf:

   | Variable | Effect |
   | --- | --- |
   | `PRAXIS_EXECUTORS_ENABLED` | comma-separated executor ids; replaces the enabled set |
   | `PRAXIS_EXECUTORS_DISABLED` | comma-separated executor ids; applied after the above |
   | `PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS` | comma-separated transports |
   | `PRAXIS_EXECUTORS_PREFERRED_CAPABILITY_KINDS` | comma-separated capability kinds |
   | `PRAXIS_CONFIG` | path to the config file (see criterion 8) |

   An unset variable is no override. A variable set to the empty string is an **explicit
   clear** to an empty list, not "unset". Whitespace around commas is stripped. Values
   are re-validated against the schema after overlay, so a misspelled transport from the
   environment fails closed rather than passing through.

   There is deliberately no `PRAXIS_EXECUTORS_ALLOWED_AUTH_TRANSPORTS` — see Open
   questions.

10. **Validation is fail-closed and never degrades to defaults.** Malformed JSON, a
    schema violation, a wrong `spec_version` major, or an unparseable environment value
    each raises with a diagnostic naming the offending file and key, and the CLI exits
    **non-zero** with that message on stderr. Silently falling back to defaults on an
    invalid config is a defect, not a convenience. Note that `match_cmd.run_match`
    currently returns `0` unconditionally; a config error must not take that path.

11. **Three wiring points consume the resolved config**, and they are the only places
    that hardcode these choices today:

    - `praxis_cli.adapters.build_adapters()` filters `_ADAPTER_FACTORIES` by the enabled
      provider set. `discover_cmd` and `status_cmd` operate over whatever that function
      returns, so they inherit the filtering with no change of their own.
    - `praxis_cli.match_cmd.run_match()` builds its `ExecutorPolicy` from config instead
      of the hardcoded `policy.AuthTransportPolicy()` at line 201.
    - `praxis_cli.match_cmd.build_requirement()` appends config-derived `preferred`
      constraints. This uses the existing documented semantics — `preferred` never
      disqualifies a candidate, it only affects ranking — so no change to `matching.py`
      is required or permitted.

    A provider id in config that matches no known adapter is an error naming the unknown
    id, so a typo disables nothing silently.

12. **Edge cases that need tests**, none of which the original spec addresses: absent
    config file; a file containing `{}`; JSON syntax error; wrong `spec_version` major;
    an unknown key rejected by `additionalProperties: false`; an unrecognized
    auth-transport value; an unknown provider id; **every provider disabled** (`match`
    must report no selection and not crash — with an empty advertisement list,
    `matching.match` returns one `UnsatisfiedPromise` per required kind); an environment
    variable set to the empty string; whitespace around commas; and an unreadable config
    file (permission denied).

13. **Documentation.** `docs/executors.md` gains a section describing the config surface,
    its precedence, and its environment variables, and the README's `praxis` CLI section
    notes the config file. Every other module in `src/` has a corresponding page under
    `docs/`; this surface should not be the exception.

## Explicitly out of scope

- **Adding a YAML dependency.** JSON only, per criterion 4.
- **Changing `ExecutorRegistry.select`/`execute`/`execute_with_proof_records`'s built-in
  `AuthTransportPolicy()` default.** `praxis_executors` stays free of any config
  dependency; wiring happens at the CLI/caller boundary, preserving the module's
  documented independence from `praxis_runtime` and `praxis_contracts` alike.
- **Changing `matching.py`'s documented `(-preferred_score, cost, executor_id)` tie-break
  order**, or the `required`/`preferred`/`prohibited` semantics in `docs/executors.md`.
- **Registering `CodexCliExecutor` in the CLI's adapter map.** The adapter exists at
  `src/praxis_executors/adapters/codex_cli.py` but is absent from
  `praxis_cli.adapters._ADAPTER_FACTORIES`. Wiring it in is a real gap and separate work;
  this bundle must not change which adapters ship, only which of the shipped ones are
  enabled.
- **Repository-local or cwd-relative config discovery** (`./praxis.config.json`,
  `./.praxis/`). A config file picked up from a checked-out repository is a trust surface
  this bundle does not open.
- **A `praxis config` subcommand** (show, validate, init, or edit).
- **Credentials, API keys, tokens, or any secret as a config field.** Never a field in
  this schema, in any layer.
- **Per-model, per-cost, budget, rate-limit, or retry configuration**, and anything from
  #49's evidence/eval/learning wiring or #50's dashboard panels.
- **Moving `praxis_cli.adapters._FAKE_CAPABILITIES` into config.** The fake adapter's
  scripted capabilities stay in Python.

## Assumptions made

1. **Schema lives at `src/praxis_contracts/schemas/v1/executor-config.schema.json`.**
   Evidence: `pyproject.toml`'s `[tool.setuptools.package-data]` globs
   `praxis_contracts = ["schemas/v1/*.schema.json"]`;
   `praxis_contracts/schema_paths.py` resolves `SCHEMA_DIR` through
   `importlib.resources`; all 23 existing schemas are already there, and no top-level
   `schemas/` directory exists in this worktree.

2. **JSON, not YAML.** Evidence: `pyproject.toml` `dependencies = ["jsonschema>=4.18",
   "referencing>=0.28.4"]`; a repo-wide grep finds no `import yaml` anywhere in `src/`;
   `praxis_contracts.validator.load_schema` reads JSON. Adding a parser dependency is a
   packaging decision this bundle should not make on its own.

3. **`spec_version`, `$id` under `https://schemas.praxis.dev/v1/`, and
   `additionalProperties: false` at every level.** Evidence: every file in
   `src/praxis_contracts/schemas/v1/` follows this; `validator.validate_document`
   rejects a document whose `spec_version` doesn't match `^1\.\d+\.\d+$` before it runs
   structural validation at all.

4. **The stated precedence order stands because nothing contradicts it.** Evidence: the
   grep for `XDG`, `Path.home()`, `expanduser`, and `.praxis` returns nothing in `src/`;
   the only `PRAXIS_`-prefixed variable in the repository is `PRAXIS_STUB_ADAPTERS`, used
   only inside `tests/test_praxis_cli_executors.py`.

5. **`PRAXIS_` is the environment-variable prefix.** Evidence: `PRAXIS_STUB_ADAPTERS` is
   the sole in-repo precedent. Note that its own read
   (`os.environ.get("PRAXIS_STUB_ADAPTERS")`, bare truthiness) is a test helper, not a
   coercion convention worth copying — hence the explicit empty-string and whitespace
   rules in criterion 9.

6. **"Preferences" means preferred capability kinds.** Evidence: the README's
   Promise/Capability section expresses deployment preference as a `prefers:` block of
   capability kinds (`context.large`, `latency.low`), and `matching.match` already
   implements a `preferred` constraint that ranks without disqualifying. An alternative
   reading — an ordered executor-id preference list used as a tie-break — was rejected
   because it would require changing the tie-break order `docs/executors.md` documents.
   If the reviewer intended that reading, it is a contained change to criterion 5's
   `preferences` block and criterion 11's third wiring point.

7. **Provider keys are open strings, not an enum.** Evidence: `docs/executors.md`'s
   "Adding a new executor adapter" states the extension path "requires no change to
   `praxis_runtime`, `praxis_contracts`, or any graph document". A fixed enum of executor
   ids in a `praxis_contracts` schema would break exactly that guarantee.

8. **Defaults must be behavior-preserving.** Evidence: the original spec asks for a
   configuration surface, not a behavior change; a user who writes no config should see
   what they see today. The four default-enabled ids are read directly from
   `praxis_cli/adapters.py`'s `_ADAPTER_FACTORIES`.

9. **An unknown provider id is an error rather than a warning.** Evidence: the
   fail-closed posture `validator.validate_document` and `AuthTransportPolicy` both
   already take. A silently ignored typo in an `enabled: false` entry leaves an executor
   running that the user believes they disabled.

10. **User config path is `~/.config/praxis/config.json`.** Evidence: no in-repo
    precedent exists, so this follows the XDG convention, which is correctable by a
    reviewer in one line and is fully covered by `$PRAXIS_CONFIG` in the meantime.

11. **The `allowed_auth_transports` knob ships in the schema, defaulting to `null`,
    honored from the config file only.** Evidence: `docs/executors.md` states a caller
    "can still loosen or replace this default by passing `is_eligible` explicitly", so a
    config-file caller doing the same is surfacing an existing capability, not creating
    one. The default of `null` leaves today's fail-closed behavior exactly unchanged. The
    environment layer is deliberately excluded — see Open questions.

## Open questions

One, and it does not block planning: **may the configuration surface enable the
`metered_api` and `api_key` auth transports, and if so, may an environment variable do
it?**

This fails the resolve-or-name test on the security-and-cost condition. Enabling those
transports permits metered billing and loosens the fail-closed default
`docs/executors.md` documents; issue #41's spec independently states "Never fall back to
a metered API key". The repository records that a *Python* caller may loosen this, but
nothing records whether a config file or an environment variable should be able to.

Interim resolution so the bundle proceeds, per assumption 11: ship
`executors.policy.allowed_auth_transports` in the schema with a default of `null`,
honor it from the config file only, and ship **no**
`PRAXIS_EXECUTORS_ALLOWED_AUTH_TRANSPORTS` variable. Nothing loosens by default, and the
least auditable layer cannot loosen anything at all. Flag this in the PR body so the
reviewer decides explicitly rather than inheriting it.
