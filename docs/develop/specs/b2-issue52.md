# Bundle b2-issue52 — Enhanced Spec

## Original content

> # Bundle b2-issue52
>
> ## Issues
> - #52 — Packaging/installation fix: console_scripts, schemas packaging, clean-env install test
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b2-issue52
> branch: develop/b2-issue52, base: origin/main
>
> ## Environment
>
> `.venv` already exists with the package installed editable. Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for build — never bare
> `pytest`/`pip`, and never a global `--break-system-packages` install.
>
> ## Problem
>
> `pyproject.toml` declares no `console_scripts` entry point. `schemas/v1/*.json`
> lives outside `src/` with no `package-data`/`MANIFEST.in` — a `pip install`
> (even editable) does not make Praxis usable outside a repo checkout today.
> Every place in `src/` that currently does `Path("schemas/v1/...")` relies on
> the process's current working directory being the repo root.
>
> ## Task
>
> - Scope note: there is no `praxis` CLI module yet in this repository (it's
>   being built by a separate, not-yet-landed issue). **Do not invent CLI
>   subcommands for this issue.** If a console-script entry point needs a
>   target module, add a minimal `src/praxis_cli/__init__.py` +
>   `src/praxis_cli/main.py` with just a `main()` that prints Praxis's
>   version and exits 0 (sourced from `pyproject.toml`'s version, not
>   hardcoded) — the real CLI (doctor/executors/run subcommands) is separate,
>   later work that will extend this same `main()`, not replace it.
> - Add the `praxis` console-script entry point
>   (`[project.scripts] praxis = "praxis_cli.main:main"`) to `pyproject.toml`.
> - Package `schemas/`: decide between (a) moving it under
>   `src/praxis_contracts/schemas/` or (b) keeping its current location and
>   adding `package-data`/`MANIFEST.in` so a built wheel includes it. Check
>   first whether `praxis_contracts.validator` (or any other module) already
>   has an opinion about where schemas should live before choosing. Whichever
>   you choose, update every `Path("schemas/v1/...")`-style relative-path
>   reference across `src/` and `tests/` to resolve correctly regardless of
>   the process's current working directory — prefer `importlib.resources`
>   over a hardcoded relative path if you relocate under `src/`.
> - Add a real, tested clean-environment install check: build the package
>   (`python -m build` or equivalent), create a fresh venv elsewhere on disk,
>   `pip install` the built artifact (not `-e .`) into it, and confirm
>   `praxis --version` and at least one schema-validating code path
>   (`praxis_contracts.validator.validate_document` against a real schema)
>   both work from a working directory that is *not* the repo checkout.
>   Capture this as an actual test or a documented, runnable script under
>   `scripts/` (create that directory if it doesn't exist) — not just manual
>   verification that isn't checked into the repo.
>
> ## Acceptance
>
> - Full test suite (`.venv/bin/python -m pytest`) passes from the repo
>   checkout.
> - The clean-environment install check passes and is either an automated
>   test or a checked-in, runnable verification script.
> - No remaining relative-path `schemas/...` reference that only works when
>   the cwd happens to be the repo root.
>
> ## Delivery
>
> Open a PR against `main` referencing #52 with a closing keyword (`Closes
> #52`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **Full test suite passes from the repo checkout** (unchanged from original) — `.venv/bin/python -m pytest`, all ~90 test files, zero failures.

2. **`praxis` console script exists and works.**
   - `[project.scripts]` in `pyproject.toml` adds `praxis = "praxis_cli.main:main"`.
   - `src/praxis_cli/__init__.py` + `src/praxis_cli/main.py` are added; `main()` prints Praxis's version and exits 0.
   - `main()` must read the version via `importlib.metadata.version("praxis-contracts")` (the distribution name already declared in `pyproject.toml`'s `[project] name`), **not** by opening/parsing `pyproject.toml` at runtime. `pyproject.toml` is not shipped inside a built wheel, and the clean-environment check (criterion 5) explicitly requires this to work from a cwd outside the repo checkout — a runtime `pyproject.toml` read would fail exactly the scenario this issue exists to fix. `importlib.metadata` is the standard-library mechanism for a package to read its own installed version and satisfies "sourced from pyproject.toml's version, not hardcoded" (the value still comes from the single `version = "0.1.0"` field, just resolved through installed package metadata instead of a repo-relative file read).
   - No CLI subcommands beyond printing the version; do not invent `doctor`/`executors`/`run` etc.

3. **Schemas are packaged and resolve correctly regardless of cwd or install location.**
   - Move `schemas/v1/*.schema.json` to `src/praxis_contracts/schemas/v1/*.schema.json` (option (a) from the original task — see **Assumptions made** for why this is resolved rather than left open).
   - Add `[tool.setuptools.package-data]` (e.g. `praxis_contracts = ["schemas/v1/*.schema.json"]`) to `pyproject.toml`. This is required either way — setuptools does not include non-`.py` files inside a discovered package by default, so the original task's "(a) ... or (b) ... adding package-data" phrasing undersells this: package-data (or an equivalent) is needed under option (a) too, not just option (b).
   - Every `SCHEMA_PATH`/`SCHEMA_DIR`-style constant in `src/` that currently walks up from `Path(__file__)` to the repo root and into `schemas/v1/` (found in: `praxis_runtime/{events,graph,state}.py`, `praxis_runtime/resources/{leases,claims}.py`, `praxis_eval/{parity,types}.py`, `praxis_evidence/types.py`, `praxis_learning/types.py`, `praxis_policy/profiles.py`, `praxis_overlay/manifest.py`) must instead resolve via `importlib.resources` against the relocated `praxis_contracts/schemas/v1/` package data. All of these modules already import from `praxis_contracts` for `validate_document`/`ContractValidationError`, so depending on `praxis_contracts` for schema location too introduces no new cross-package coupling.
   - The one bare, genuinely cwd-relative reference — `tests/test_overlay_trivial_fixture.py:37`, `_EVIDENCE_REQUIREMENT_SCHEMA = Path("schemas/v1/evidence-requirement.schema.json")` — must be fixed to resolve the same way the relocated schemas do (or via the same `Path(__file__)`-anchoring pattern already used by every sibling test file, e.g. `tests/test_valid_contracts.py`'s `SCHEMAS_DIR`).
   - `praxis_contracts/validator.py`'s `_build_registry` sibling-`$ref` resolution (`schema_path.parent / ref_filename`) needs no change — it's already location-agnostic given a correct `schema_path`.

4. **Clean-environment install check.**
   - Build via `python -m build` (or equivalent); the `build` package is not currently a dev dependency (`[project.optional-dependencies] dev = ["pytest>=7"]`) and must be added so `.venv/bin/python -m pip install -e .[dev]` provides everything the check needs.
   - The fresh venv should be created via `tempfile.TemporaryDirectory()` (or equivalent), outside the repo tree, and torn down after the check runs — mirroring the ephemeral-directory pattern already used for run state in `README.md`'s quickstart and in `praxis_runtime` tests — so the check is repeatable and leaves no artifacts in the repo.
   - Install the built artifact (wheel or sdist) with `pip install <artifact>` into that venv (not `-e .`).
   - From a cwd that is not the repo checkout, confirm: (a) `praxis --version` (or `praxis_cli.main.main()` invoked as the console script) exits 0 and prints a version string; (b) `praxis_contracts.validator.validate_document` succeeds against a real schema resolved from the installed package (not the repo's `schemas/` or `examples/` directories) and a minimal valid instance literal matching that schema's required shape — e.g. mirroring the `requirement.schema.json` / `graph-requests-capability.json` shape already used in `tests/test_valid_contracts.py`, but constructed inline in the check rather than read from `examples/` (which is not packaged and is out of scope here).
   - Ship this as either a pytest test (if it can run from inside the existing suite via subprocess) or a checked-in script under `scripts/` (create the directory) — either way it must be runnable non-interactively and fail loudly (non-zero exit / assertion) on any of the above not holding.

5. **No remaining relative-path `schemas/...` reference that only works when cwd happens to be the repo root** (unchanged from original) — satisfied by criterion 3's cleanup; verify with a repo-wide search after the change, the same way `tests/test_repair_findings_b5_issue6.py`'s existing schema-inventory tests already do for a related concern.

6. **README consistency (added).** `README.md`'s Installation section (`README.md:298`, `:306-308`) and Usage section (`README.md:320`) currently state, as fact, that "there is no `praxis` console script" and that schemas are "not yet packaged as package data" and that Praxis must be "run... from a repo checkout, not an installed wheel." Once this bundle lands, all three statements are false. Update these passages to describe the new `pip install` / `praxis --version` / clean-env behavior. This is a direct, in-scope consequence of the Task section (which is entirely about making these README claims obsolete), not a scope expansion — leaving them as-is would make the shipped README self-contradictory the moment this PR merges.

## Explicitly out of scope

- Any real CLI subcommand (`doctor`, `executors`, `run`, etc.) — `praxis_cli.main.main()` only prints the version and exits 0. That work is a separate, not-yet-landed issue and this bundle's `main()` is meant to be extended by it later, not preempted.
- Publishing to PyPI. The clean-environment check installs a locally built artifact into a local fresh venv; it does not touch any package index.
- A `schemas/v2/` migration or any change to schema *content* — this bundle only relocates/packages the existing `schemas/v1/*.schema.json` files and fixes how they're located, it does not change what they validate.
- Wiring Praxis into `/develop`'s actual dispatch (tracked separately per `README.md`'s Status section) — out of scope regardless of this bundle.
- Packaging `src/praxis_dashboard/static/*` or any other non-schema data file — out of scope for #52 even though it's an analogous latent gap (no `package-data` entry currently covers it either); do not fold it into this bundle's diff.
- Renaming the `praxis-contracts` distribution — the console-script entry point and `importlib.metadata.version(...)` lookup both use the existing `[project] name = "praxis-contracts"` as-is.

## Assumptions made

- **Schema relocation: option (a), move to `src/praxis_contracts/schemas/v1/`, not option (b).** Evidence: (1) every module that currently loads a schema already imports from `praxis_contracts` for `validate_document`/`ContractValidationError` (`praxis_runtime`, `praxis_eval`, `praxis_evidence`, `praxis_learning`, `praxis_policy`, `praxis_overlay` all do — confirmed by grep), so `praxis_contracts` is already the de facto "owns the contracts" package and gains no new coupling by also owning their storage location; (2) the repo already has a sibling precedent for packaging non-Python data inside the package that uses it — `src/praxis_dashboard/static/{index.html,style.css,app.js}`, referenced via `Path(__file__).resolve().parent / "static"` in `src/praxis_dashboard/server.py`; (3) the original task text itself says to "prefer `importlib.resources` over a hardcoded relative path *if you relocate under `src/`*," signaling the spec author's own lean toward relocation; (4) option (b) (files outside any package, included via `MANIFEST.in`) only reliably reaches an sdist, not a wheel, under `[tool.setuptools.packages.find]` — it cannot satisfy "works from a cwd that is not the repo checkout" after a `pip install` of a built wheel as cleanly as option (a) can. Passes the resolve-or-name test: in scope (the task explicitly asks for this decision), a defensible default is on record (points 1–3 above), it doesn't change what any acceptance criterion means, it touches no security/compat/cost/deployment boundary (no released version, no external consumer yet per `README.md`'s Status section), and it's trivially checkable by a reviewer.

- **`praxis_cli.main.main()` reads its version via `importlib.metadata.version("praxis-contracts")`, not by parsing `pyproject.toml`.** Evidence: `importlib.metadata` is the Python standard library's documented mechanism for a package to read its own installed distribution metadata, works identically for editable and wheel installs, and is the only way to satisfy both "sourced from pyproject.toml's version, not hardcoded" and "works from a working directory that is not the repo checkout" simultaneously — `pyproject.toml` is not present in a built wheel install. Resolve-or-name test: in scope, standard-library convention is as strong a "defensible default" as a repo convention, doesn't change acceptance-criteria meaning, no security/compat impact, trivially correctable.

- **`build` added to `[project.optional-dependencies] dev`.** Evidence: the task explicitly requires `python -m build` (or equivalent) for the clean-env check; the `Environment` section mandates using `.venv/bin/python -m pip install -e .[dev]` for all setup, so the tool the check depends on must be reachable through that same extras group. Low-risk, dev-only dependency addition.

- **Fresh venv for the clean-env check is created via a temporary directory and cleaned up after use**, rather than a fixed path under the repo. Evidence: matches the existing ephemeral-directory pattern already used for run state in `README.md`'s own quickstart snippet (`tempfile.TemporaryDirectory()`) and keeps the check repeatable/side-effect-free.

- **`tests/test_overlay_trivial_fixture.py:37`'s bare `Path("schemas/v1/evidence-requirement.schema.json")` is in scope to fix** as part of "every `Path("schemas/v1/...")`-style relative-path reference" — it is the one remaining literal instance of exactly the pattern the original task calls out by name.

- **README's Installation/Usage sections (`README.md:298`, `:306-308`, `:320`) are updated** to stop asserting the pre-fix behavior. See Clarified acceptance criteria item 6 for the consistency rationale.

## Open questions

None. Every gap found passed the resolve-or-name test against evidence already in the repository (existing `praxis_contracts` import graph, the `praxis_dashboard/static` packaging precedent, the standard-library `importlib.metadata` convention, and the README's own explicit description of the current, soon-to-be-fixed limitation).
