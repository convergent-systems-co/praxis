# Plan: b-issue51 — Executor configuration surface + validation + precedence

Spec: `docs/develop/specs/b-issue51.md` (enhanced). Base: `origin/main` at 9366f5b.
Branch: `develop/b-issue51`.

## Shape of the work

The bundle adds one JSON config document, one loader that resolves it across three
layers, and three wiring points that consume the result. Nothing in
`src/praxis_executors/` changes: the loader and the policy it builds live in
`praxis_cli`, preserving that package's documented independence (spec "Explicitly out
of scope").

New modules, both under `src/praxis_cli/`:

- `config_env.py` — pure translation of the five environment variables into config
  fragments. Imports nothing from `config.py`, raises nothing; every malformed value
  is caught downstream by schema validation. This is what lets it be built in parallel
  with the schema.
- `config.py` — defaults, file discovery, layer merge, validation, the
  `ExecutorsConfig` value object, and `build_policy` / `enabled_executor_ids`
  translation helpers.

### The resolved document

`default_document()` returns this, fully materialized — criterion 6 requires resolution
to be total, so no consumer ever handles a missing key:

```json
{
  "spec_version": "1.0.0",
  "executors": {
    "providers": {
      "executor-subprocess-1": {"enabled": true},
      "executor-fake-1": {"enabled": true},
      "executor-claude-cli-1": {"enabled": true},
      "executor-ollama-1": {"enabled": true}
    },
    "policy": {
      "allowed_auth_transports": null,
      "denied_auth_transports": [],
      "allowed_executor_ids": null,
      "denied_executor_ids": []
    },
    "preferences": {"preferred_capability_kinds": []}
  }
}
```

### Two planner decisions the spec left implicit

1. **Defaults are materialized, not implied.** The spec says every default is filled and
   that `PRAXIS_EXECUTORS_ENABLED` "replaces the enabled set", including the empty-string
   case that clears it. A rule like "an id absent from `providers` is enabled" cannot
   express an empty enabled set, so `providers` instead lists every default-enabled id
   explicitly and an id absent from the resolved `providers` map is **not** enabled.
   A user file listing only `{"executor-ollama-1": {"enabled": false}}` still merges
   key-by-key over the four defaults, so the other three stay on.
2. **`DEFAULT_ENABLED_EXECUTOR_IDS` is spelled in `config.py`, not read from
   `adapters.py`.** Reading it from `praxis_cli.adapters` would make `config.py` import
   the module that imports it. The duplication is guarded by a drift test in T5 that
   asserts the two sets are equal, which is also criterion 7's assertion.

### Precedence, once

`default_document()` → user file (validated on its own, then merged key-by-key) →
environment overlay → validate the resolved document again. Objects merge key-by-key,
scalars replace, arrays replace wholesale. `executors.providers` is the one object that
the `PRAXIS_EXECUTORS_ENABLED` layer replaces wholesale rather than merging, because
"replaces the enabled set" is what the spec says it does.

## Task graph

```
T0 ──┬── T1 ──┐
     └── T2 ──┴── T4 ──┬── T5 ──┐
                       └── T6 ──┴── T7
T3 (independent, no dependencies)
```

Critical path: T0 → T1 → T4 → T5 → T7 (5, against a ceiling of 4). No two tasks share a
file, so nothing serializes on a footprint. The one node of overage is T0: this worktree
has no `.venv` and the machine's `python3` is 3.9, below the project's
`requires-python = ">=3.10"`, so every code task needs an interpreter T0 supplies.
Removing that edge would mean validating the bundle under an unsupported interpreter, so
the edge stays. The remaining four layers — schema and environment translation, then the
loader, then the two wiring points, then the CLI entry point — are each a real
dependency: the loader validates against the schema file, the wiring functions consume
the loader's types, and `main` calls both wiring functions.

T3 runs from the start; T1 and T2 run together; T5 and T6 run together.

---

## T0 — Development environment bootstrap

Kept deliberately tiny: it installs the package and nothing else, so T1, T2 and T3 start
almost immediately.

**Files:** `.venv/**` (git-ignored; no tracked file changes)

**Interfaces:** none.

**Depends on:** none

**Steps:**

- [ ] Find an interpreter satisfying `requires-python = ">=3.10"`. The system `python3`
      on this machine is 3.9 and will not do. Try `python3.13`, `python3.12`,
      `python3.11`, `python3.10`, then `uv python find`, and use the first that answers.
- [ ] `<python> -m venv .venv` in the worktree root, then
      `.venv/bin/pip install -e '.[dev]'`.
- [ ] Run `.venv/bin/python -m pytest -q` and record the baseline. Four tests in
      `tests/test_praxis_cli_executors.py` fail under an uninstalled package because
      `importlib.metadata.version("praxis-contracts")` raises; after the editable
      install they must pass. A green baseline is the deliverable.
- [ ] Do not commit anything. `.venv/` is already in `.gitignore`; adding it again is a
      defect.

---

## T1 — `executor-config.schema.json`

**Files:**
- `src/praxis_contracts/schemas/v1/executor-config.schema.json` (new)
- `tests/test_executor_config_schema.py` (new)

**Interfaces:** the document shape above. No Python interface.

**Depends on:** T0

**Steps:**

- [ ] Create the schema with `"$schema": "https://json-schema.org/draft/2020-12/schema"`
      and `"$id": "https://schemas.praxis.dev/v1/executor-config.schema.json"`, matching
      the header of every sibling in that directory.
- [ ] `"required": ["spec_version"]`, `spec_version` with pattern `^1\\.\\d+\\.\\d+$`.
- [ ] `additionalProperties: false` at the root, at `executors`, at `executors.policy`,
      at `executors.preferences`, and at each `providers` value object.
- [ ] `executors.providers`: `"type": "object"` whose `additionalProperties` is the
      value schema `{"type":"object","required":["enabled"],
      "properties":{"enabled":{"type":"boolean"}},"additionalProperties":false}`, with
      `"propertyNames": {"type":"string","minLength":1}`. Keys are open strings, not an
      enum — add a `description` saying why (spec assumption 7: a new adapter must need
      no schema change), because this is the one place the file does not close
      `additionalProperties` and a reader will otherwise read it as an oversight.
- [ ] `executors.policy.allowed_auth_transports` and `denied_auth_transports`: arrays of
      the five recognized transports. Verify the enum against
      `_RECOGNIZED_AUTH_TRANSPORTS` in `src/praxis_executors/policy.py` and against the
      `auth_transport` enum in `capability.schema.json`, and cite both in the field's
      `description`. `allowed_auth_transports` is `{"type": ["array", "null"]}` with a
      `default` of `null`; `denied_auth_transports` is array-only with a `default` of
      `[]`.
- [ ] `executors.policy.allowed_executor_ids` (`["array","null"]`) and
      `denied_executor_ids` (array), both of plain strings.
- [ ] `executors.preferences.preferred_capability_kinds`: array of strings with pattern
      `^[a-z0-9]+(-[a-z0-9]+)*$`, the same pattern `capability.schema.json` puts on
      `satisfies[].kind`.
- [ ] Write `tests/test_executor_config_schema.py` driving
      `praxis_contracts.validator.validate_document` against
      `praxis_contracts.schema_paths.schema_path("executor-config.schema.json")`
      directly, so this file has no dependency on `praxis_cli.config`. Cover: a minimal
      `{"spec_version": "1.0.0"}`; the fully populated default document from the plan;
      an unknown root key rejected; an unknown key inside `executors.policy` rejected; a
      `providers` value with an extra key rejected; an arbitrary new provider id
      accepted; an unrecognized `auth_transport` string rejected; a
      `preferred_capability_kinds` entry violating the pattern rejected; `spec_version`
      of `"2.0.0"` rejected as a version mismatch; `allowed_auth_transports: null`
      accepted.

---

## T2 — Environment-variable translation

**Files:**
- `src/praxis_cli/config_env.py` (new)
- `tests/test_executor_config_env.py` (new)

**Interfaces:**

```python
CONFIG_PATH_VAR = "PRAXIS_CONFIG"
ENABLED_VAR = "PRAXIS_EXECUTORS_ENABLED"
DISABLED_VAR = "PRAXIS_EXECUTORS_DISABLED"
DENIED_AUTH_TRANSPORTS_VAR = "PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS"
PREFERRED_CAPABILITY_KINDS_VAR = "PRAXIS_EXECUTORS_PREFERRED_CAPABILITY_KINDS"

def split_list(raw: str) -> list[str]: ...
def config_path_from_env(env: Mapping[str, str]) -> str | None: ...
def environment_overlay(env: Mapping[str, str]) -> dict: ...
def provider_overrides(env: Mapping[str, str]) -> tuple[list[str] | None, list[str] | None]: ...
```

**Depends on:** T0

**Steps:**

- [ ] Module docstring stating the design constraint: this module never raises and never
      validates. A misspelled transport is passed through verbatim so that the schema
      re-validation in `config.py` is the single place a bad value fails, per spec
      criterion 9's "re-validated against the schema after overlay".
- [ ] `split_list`: split on `,`, strip whitespace from each token, drop empty tokens.
      `""` and `" , "` both yield `[]`. This is the "explicit clear" case — the caller
      distinguishes it from "unset" by whether the key is in `env` at all, never by the
      emptiness of the result.
- [ ] `config_path_from_env`: return `env["PRAXIS_CONFIG"]` if the key is present, else
      `None`. An empty string is returned as-is, not treated as unset, so the loader can
      fail on it as a named-but-missing file.
- [ ] `environment_overlay`: build a fragment carrying only the sub-blocks whose variable
      is present. `PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS` →
      `{"executors": {"policy": {"denied_auth_transports": [...]}}}`;
      `PRAXIS_EXECUTORS_PREFERRED_CAPABILITY_KINDS` →
      `{"executors": {"preferences": {"preferred_capability_kinds": [...]}}}`. Return
      `{}` when neither is set. Providers are deliberately not in this fragment: they do
      not follow the key-by-key merge rule the fragment is merged under.
- [ ] `provider_overrides`: return `(enabled, disabled)` where `enabled` is
      `split_list(env[ENABLED_VAR])` if that key is present else `None`, and likewise for
      `disabled`. `None` means unset; `[]` means an explicit clear.
- [ ] Ship no `PRAXIS_EXECUTORS_ALLOWED_AUTH_TRANSPORTS`. Add a comment naming the spec's
      Open questions section as the reason, so a later reader does not add it as an
      apparent omission.
- [ ] Tests in `tests/test_executor_config_env.py`, over literal dicts rather than
      `os.environ`: unset yields no fragment; empty string yields an explicit empty list;
      whitespace around commas is stripped; a repeated or empty token is dropped; a
      nonsense transport value passes through untouched (the schema layer rejects it, not
      this one); `PRAXIS_CONFIG` present-but-empty comes back as `""`, not `None`.

---

## T3 — Documentation

**Files:**
- `docs/executors.md`
- `README.md`

**Depends on:** none — the surface is fully specified by this plan and the enhanced spec,
so this runs from the start rather than waiting on the code.

**Interfaces:** none.

**Steps:**

- [ ] Add a `## Executor configuration` section to `docs/executors.md`, after
      `## praxis_executors.policy` and before `## praxis_executors.registry`, so the
      policy vocabulary it references is already introduced.
- [ ] Document the document shape, reproducing the fully populated default from this
      plan, and state that the file is JSON.
- [ ] Document discovery order: an explicit path from the caller, else `$PRAXIS_CONFIG`,
      else `~/.config/praxis/config.json`, else defaults only. State that a missing file
      at the default location is not an error while a missing file at an explicitly named
      path is.
- [ ] Document precedence — defaults, then file, then environment — and the merge rules:
      objects merge key-by-key, scalars replace, arrays replace wholesale and never
      union.
- [ ] Reproduce the five-variable table from spec criterion 9, and state the two rules a
      reader cannot guess: an unset variable is no override, a variable set to the empty
      string is an explicit clear to an empty list.
- [ ] State that `PRAXIS_EXECUTORS_ENABLED` replaces `executors.providers` wholesale
      while a config file merges into it, and that an id absent from the resolved
      `providers` map is not enabled.
- [ ] State that `allowed_auth_transports` is honored from the config file only and has
      no environment variable, and why — enabling `metered_api` or `api_key` is the one
      change that loosens the fail-closed default this document already describes.
- [ ] State that an unknown provider id and any schema violation are errors that exit
      non-zero, never a silent fall back to defaults.
- [ ] Cross-reference the existing "Adding a new executor adapter" section: adding an
      adapter still requires no schema change, because provider keys are open strings.
- [ ] In `README.md`, extend the `### Inspecting executors: the praxis CLI` section
      (around line 391) with two or three sentences naming the config file location, the
      `PRAXIS_CONFIG` variable, and a pointer to `docs/executors.md` for the full
      surface. Do not restate the table there.
- [ ] Add nothing about a `praxis config` subcommand or a `--config` flag; neither ships
      in this bundle.

---

## T4 — Config loader

The largest task, and the one every wiring task waits on. It owns layer merging,
discovery, validation, and the two translation helpers, so T5 and T6 stay thin.

**Files:**
- `src/praxis_cli/config.py` (new)
- `tests/test_executor_config.py` (new)

**Interfaces:**

```python
SCHEMA_FILENAME = "executor-config.schema.json"
DEFAULT_SPEC_VERSION = "1.0.0"
DEFAULT_ENABLED_EXECUTOR_IDS: tuple[str, ...]   # the four ids in adapters._ADAPTER_FACTORIES

class ConfigError(Exception):
    """Raised with a message naming the offending file and key."""

@dataclass(frozen=True)
class ExecutorsConfig:
    providers: Mapping[str, bool]
    allowed_auth_transports: frozenset[str] | None
    denied_auth_transports: frozenset[str]
    allowed_executor_ids: frozenset[str] | None
    denied_executor_ids: frozenset[str]
    preferred_capability_kinds: tuple[str, ...]

DEFAULT_CONFIG: ExecutorsConfig

def default_document() -> dict: ...
def user_config_path() -> Path: ...                       # ~/.config/praxis/config.json
def resolve_document(*, path: str | Path | None = None,
                     env: Mapping[str, str] | None = None) -> dict: ...
def load_config(*, path: str | Path | None = None,
                env: Mapping[str, str] | None = None) -> ExecutorsConfig: ...
def enabled_executor_ids(config: ExecutorsConfig,
                         known_ids: Iterable[str]) -> list[str]: ...
def build_policy(config: ExecutorsConfig) -> policy.ExecutorPolicy: ...
```

`env` defaults to `os.environ` when `None`; every test passes a literal mapping.

**Depends on:** T1, T2

**Steps:**

- [ ] `default_document()` returns a fresh deep copy of the document in this plan's
      "The resolved document" section each call, built from
      `DEFAULT_ENABLED_EXECUTOR_IDS`. Never hand out a shared mutable constant — the
      merge below mutates its accumulator.
- [ ] `DEFAULT_ENABLED_EXECUTOR_IDS = ("executor-subprocess-1", "executor-fake-1",
      "executor-claude-cli-1", "executor-ollama-1")`, with a comment pointing at
      `praxis_cli.adapters._ADAPTER_FACTORIES` as the source of truth and at T5's drift
      test as the guard. Do not import `adapters` here; it imports this module.
- [ ] Private `_merge(base: dict, overlay: dict) -> dict`: objects merge key-by-key,
      every other type replaces wholesale. Arrays replace and never union — state that in
      the docstring, since it is the rule a reader is most likely to assume otherwise.
- [ ] `_validate(document, *, source)`: call
      `praxis_contracts.validator.validate_document(document,
      schema_path(SCHEMA_FILENAME))` and re-raise `ContractValidationError` as
      `ConfigError` whose message names `source` (the file path, or the phrase
      "resolved configuration" for the post-overlay pass) and joins `exc.errors` so the
      offending key appears. Chain with `raise ... from exc`.
- [ ] `resolve_document`: (1) start from `default_document()`; (2) pick the file per
      criterion 8 — the `path` argument, else `config_env.config_path_from_env(env)`,
      else `user_config_path()`; (3) read it; (4) validate the file document on its own
      before merging, so a missing or wrong-major `spec_version` and any unknown key are
      blamed on the file rather than surfacing later against the merged document; (5)
      merge it over the defaults; (6) merge `config_env.environment_overlay(env)`;
      (7) apply `config_env.provider_overrides(env)`; (8) validate the result again.
- [ ] File-reading rules: a missing file at `user_config_path()` is not an error and
      leaves the defaults in place. A missing file at a path the caller named, or at
      `$PRAXIS_CONFIG`, raises `ConfigError` naming the path. `json.JSONDecodeError`
      becomes `ConfigError` naming the path and the decoder's line and column. `OSError`
      (permission denied, a directory where a file was named) becomes `ConfigError`
      naming the path and the OS message — catch `OSError` rather than only
      `PermissionError`. Read via `Path.read_text()`.
- [ ] Provider overrides, in this order: if the enabled list is not `None`, replace
      `executors.providers` wholesale with `{id: {"enabled": True} for id in list}` — an
      empty list therefore leaves `{}` and enables nothing; then, if the disabled list is
      not `None`, set `{"enabled": False}` for each of its ids, merging into whatever
      map is now there. An empty disabled list is a no-op, which is the one place an
      explicit clear and an unset variable coincide; say so in a comment.
- [ ] `load_config` projects the resolved document into `ExecutorsConfig`. `providers`
      becomes `MappingProxyType({id: entry["enabled"] ...})` so the frozen dataclass is
      genuinely immutable. `allowed_auth_transports` and `allowed_executor_ids` stay
      `None` when null, else become frozensets. `preferred_capability_kinds` becomes a
      tuple, preserving document order.
- [ ] `DEFAULT_CONFIG` is `load_config` applied to `default_document()` with an empty
      environment, evaluated once at module scope. It is the value the wiring functions
      in T5 and T6 fall back to when passed `None`, which is what keeps their intermediate
      states behavior-preserving.
- [ ] `enabled_executor_ids(config, known_ids)`: raise `ConfigError` naming every id in
      `config.providers` that is not in `known_ids` (sorted, so the message is stable),
      then return the ids of `known_ids` whose `providers` value is `True`, in
      `known_ids` order. An id in `known_ids` but absent from `providers` is not enabled.
- [ ] `build_policy(config)`: return a bare
      `policy.AuthTransportPolicy(denied_auth_transports=..., allowed_auth_transports=...)`
      when both id lists are at their defaults, so the default config produces exactly
      the object `match_cmd` constructs today. Otherwise wrap it in a private frozen
      `_AllOfPolicy(ExecutorPolicy)` that ANDs it with `policy.AllowListPolicy` (only
      when `allowed_executor_ids` is not `None`) and `policy.DenyListPolicy` (only when
      `denied_executor_ids` is non-empty). Do not add anything to
      `src/praxis_executors/policy.py`; the composite belongs at the CLI boundary.
- [ ] Tests in `tests/test_executor_config.py` covering spec criterion 12 at the loader
      level, each with `tmp_path` and a literal env mapping: no file present anywhere →
      exactly `DEFAULT_CONFIG`; a file of `{"spec_version": "1.0.0"}` → also
      `DEFAULT_CONFIG`; a file of `{}` → `ConfigError` for the missing `spec_version`;
      malformed JSON → `ConfigError` naming the path; `spec_version` of `"2.0.0"` →
      `ConfigError`; an unknown key → `ConfigError`; an unrecognized auth transport in
      the file → `ConfigError`; the same value arriving from
      `PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS` → also `ConfigError`, proving the
      post-overlay re-validation runs; a missing file at an explicit path → `ConfigError`;
      a missing file at `$PRAXIS_CONFIG` → `ConfigError`; a missing file at the default
      user path → defaults; a file with mode `0o000` → `ConfigError` (skip the test when
      running as root, where the chmod does not bite).
- [ ] Further loader tests: key-by-key merge leaves untouched provider entries alone;
      an array in the file replaces the default array rather than unioning;
      `PRAXIS_EXECUTORS_ENABLED` replaces the provider map wholesale;
      `PRAXIS_EXECUTORS_ENABLED=""` enables nothing; `PRAXIS_EXECUTORS_DISABLED` applies
      after it; whitespace around commas is tolerated end to end; `enabled_executor_ids`
      raises on an unknown id and names it; `build_policy(DEFAULT_CONFIG)` equals
      `policy.AuthTransportPolicy()`; a config denying `local` makes the fake adapter's
      advertisement ineligible through `build_policy`.

---

## T5 — Wire the resolved config into adapter construction

**Files:**
- `src/praxis_cli/adapters.py`
- `tests/test_cli_adapters.py`

**Interfaces:**

```python
def build_adapters(config: config.ExecutorsConfig | None = None) -> dict[str, Executor]: ...
```

**Depends on:** T4

**Steps:**

- [ ] Import `from praxis_cli import config as config_module` (or equivalent) and give
      `build_adapters` the optional parameter above. `None` means `DEFAULT_CONFIG`, not
      "load from disk" — construction must never probe the environment, which is the
      property this module's docstring already states. Extend that docstring to say the
      caller resolves configuration and passes it in.
- [ ] Filter `_ADAPTER_FACTORIES` through
      `config_module.enabled_executor_ids(config, _ADAPTER_FACTORIES)`, preserving
      declaration order, and let the `ConfigError` for an unknown id propagate.
- [ ] Leave `_FAKE_CAPABILITIES` and the per-call deep copy exactly as they are.
- [ ] Confirm `discover_cmd` and `status_cmd` need no change: both take the mapping
      `build_adapters` returns. Do not touch either file.
- [ ] Add the drift test: `set(adapters._ADAPTER_FACTORIES) ==
      set(config.DEFAULT_ENABLED_EXECUTOR_IDS)`. This is what keeps T4's hardcoded
      default list honest, and it is the assertion that fails the day someone registers
      `CodexCliExecutor` without deciding whether it ships enabled.
- [ ] Tests: `build_adapters()` with no argument returns the same four ids in the same
      order as before this change; a config disabling one id drops exactly that adapter;
      a config enabling none returns `{}`; a config naming an unknown provider id raises
      `ConfigError` whose message contains that id.

---

## T6 — Wire the resolved config into `match`

**Files:**
- `src/praxis_cli/match_cmd.py`
- `tests/test_cli_match.py`

**Interfaces:**

```python
def build_requirement(capabilities: list[str],
                      config: config.ExecutorsConfig | None = None) -> dict: ...
def run_match(adapters: Mapping[str, Executor], *, capabilities: list[str],
              explain: bool, config: config.ExecutorsConfig | None = None) -> int: ...
```

**Depends on:** T4

**Steps:**

- [ ] `None` means `DEFAULT_CONFIG` in both functions, so an existing caller keeps
      today's behavior exactly.
- [ ] Replace the hardcoded `policy.AuthTransportPolicy()` at line 201 with
      `config_module.build_policy(config)`. Keep `policy.as_eligibility_callable` and
      everything downstream unchanged.
- [ ] `build_requirement` appends one `{"constraint": "preferred"}` entry per
      `config.preferred_capability_kinds`, after the `required` entries. Drop any kind
      already present as `required`: `matching._ordered_unique_kinds` would otherwise
      let a kind count toward `preferred_score` purely because it was also required,
      which is a ranking change nobody asked for.
- [ ] Verify against `src/praxis_contracts/schemas/v1/requirement.schema.json` that
      `constraint` accepts `"preferred"` and that the requirement still satisfies
      `minItems: 1`, and cite the schema in a code comment.
- [ ] Change nothing in `src/praxis_executors/matching.py` and nothing about the
      `(-preferred_score, cost, executor_id)` tie-break — the spec puts both out of
      scope. `preferred` ranks without disqualifying, which is exactly why it is the
      mechanism used here.
- [ ] Update the module docstring: its second paragraph currently says `preferred` and
      `prohibited` are out of scope for this command. `preferred` is now config-derived;
      `prohibited` remains out of scope.
- [ ] Leave the unconditional `return 0` and its recorded-limitation comment alone. A
      config error cannot reach this function: T7 resolves the config before calling it.
- [ ] Tests: the default config produces the requirement `build_requirement` produced
      before this change, byte for byte; a preferred kind appears as a `preferred` entry;
      a kind both required and preferred appears once, as `required`; a config denying
      `local` makes the fake adapter ineligible and `--explain` reports it with the
      existing `(policy_excluded)` suffix; a config with an `allowed_executor_ids`
      allowlist excludes an adapter outside the list; the composite policy and the
      auth-transport policy compose (an id on the allowlist whose transport is denied is
      still ineligible).

---

## T7 — CLI entry point, behavior preservation, and CLI-level edge cases

**Files:**
- `src/praxis_cli/main.py`
- `tests/test_praxis_cli_config.py` (new)
- `tests/test_praxis_cli_executors.py`

**Interfaces:** no signature change to `main`.

**Depends on:** T5, T6

**Steps:**

- [ ] In `main`, after the lazy `from praxis_cli import adapters, discover_cmd,
      match_cmd, status_cmd` import and before `build_adapters`, add `config` to that
      import and resolve once: `resolved = config.load_config()`. Resolving inside the
      lazy block keeps the `praxis --version` path free of any config read, which is the
      property the existing comment at line 55 protects.
- [ ] Wrap the resolution, `build_adapters(resolved)`, and the `run_match(...,
      config=resolved)` call in one `except config.ConfigError` that prints
      `f"praxis: {exc}"` to `sys.stderr` and returns `2`. `build_adapters` is inside the
      guard because the unknown-provider-id error is raised there, not by the loader.
      Exit code 2 matches what `argparse`'s own `parser.error` returns from this command.
- [ ] Pass `resolved` to `build_adapters` and to `run_match`. `discover_cmd.run_discover`
      and `status_cmd.run_status` keep their current signatures; they consume the already
      filtered mapping.
- [ ] Add `tests/test_praxis_cli_config.py` covering the CLI-level edge cases in spec
      criterion 12, driving `main.main(argv)` in-process with `monkeypatch.setenv` and
      `capsys`: a `$PRAXIS_CONFIG` file disabling one adapter drops its row from
      `praxis executors` and `praxis executors discover`; **every provider disabled** →
      `praxis executors match --capability text-generation` prints "no executor selected"
      followed by one unsatisfied line per required kind and returns 0, and neither
      `discover` nor the bare status command raises; a malformed config file → non-zero
      exit with the diagnostic on stderr and nothing on stdout; an unknown provider id →
      the same, with the id named; a bad
      `PRAXIS_EXECUTORS_DENIED_AUTH_TRANSPORTS` value → the same. Assert on the exit code
      and on stderr, since criterion 10 is precisely that a config error does not take
      `run_match`'s unconditional `return 0`.
- [ ] Add the behavior-preservation test required by criterion 7 to
      `tests/test_praxis_cli_executors.py`, beside the existing subprocess-driven CLI
      tests: point `HOME` at an empty `tmp_path`, leave `PRAXIS_CONFIG` unset, unset
      every `PRAXIS_EXECUTORS_*` variable, then assert `praxis executors`, `praxis executors discover`, and
      `praxis executors match --capability text-generation --explain` produce output
      identical to the same commands run with the pre-change code path. Capture the
      expected output by calling the command functions with `config=None` (the
      `DEFAULT_CONFIG` path) rather than pinning a string literal, so the test asserts
      equivalence rather than a snapshot that drifts.
- [ ] Point `HOME` at a temporary directory in every test that could otherwise pick up
      the developer's real `~/.config/praxis/config.json`. A test suite whose result
      depends on the machine's home directory is a defect this bundle introduces.
- [ ] Run the full suite: `.venv/bin/python -m pytest -q`. Every pre-existing test must
      still pass; none of them may need editing to accommodate this bundle, since every
      new parameter defaults to today's behavior.

---

## Deliberately not planned

- **A `--config` CLI flag.** Criterion 8's "explicit path passed by the caller" is the
  `path` argument of `load_config`; `$PRAXIS_CONFIG` covers the CLI. Adding an argparse
  flag would widen `main.py`'s parser beyond the spec, and the spec puts CLI surface for
  configuration (`praxis config`) out of scope.
- **`PRAXIS_EXECUTORS_ALLOWED_AUTH_TRANSPORTS`.** The spec's Open questions section
  resolves this to "config file only, no environment variable", pending a reviewer
  decision. T3 documents the gap and the PR body must flag it.
- **Registering `CodexCliExecutor`.** Out of scope; T5's drift test is where that
  decision will surface next.
- **Any change to `src/praxis_executors/`.** The composite policy lives in
  `praxis_cli.config`.
