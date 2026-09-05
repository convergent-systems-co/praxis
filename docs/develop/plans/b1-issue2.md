# Plan: b1-issue2 — Praxis contracts & Promise/Capability ontology

Spec: `docs/develop/specs/b1-issue2.md` (issue #2).

## Scaffolding choice

The repository is empty (only LICENSE/NOTICE/README). This bundle's deliverable is a set of
versioned, machine-readable JSON Schema documents plus deterministic validation and tests, so:

- **Schemas live at the repo root**, under `schemas/v1/*.schema.json`, as plain JSON Schema
  (draft 2020-12) — not nested inside any language package. They are the ontology every later
  Praxis component is built against (runtime, scheduler, executor registry, evidence gates,
  policy), and some of those may not be Python, so the contracts themselves must not be coupled
  to one language's package layout.
- **Validation tooling is a small Python package** (`src/praxis_contracts/`) using the
  `jsonschema` library (draft 2020-12 support, actively maintained, no need to hand-roll a
  validator) and `pytest` as the test runner. Python is chosen only because it is the lowest-
  ceremony way to get a working validator + test runner with no existing precedent in this repo
  to follow instead; it is not a statement about what language later Praxis components use.
- Versioning convention: schema directory `v1/` is the major version. Every instance document
  (not the schemas themselves) carries a top-level `"spec_version"` string field matching
  `^1\.\d+\.\d+$` for a `v1` schema. A `spec_version` outside that pattern is treated by the
  validator as a **version mismatch**, reported as a distinct, clearly-worded error from generic
  schema-shape errors, before generic JSON Schema validation runs.
- Naming convention: every "abstract capability class" field (`Promise.kind`,
  `Capability.satisfies[].kind`) is a lowercase-hyphenated free-form string
  (`^[a-z0-9]+(-[a-z0-9]+)*$`) such as `text-generation` or `code-execution` — never a model or
  vendor name. This is enforced by construction (the schemas never enumerate real model/vendor
  names anywhere, and field/type names are all domain-neutral: `promise`, `capability`,
  `requirement`, `evidence`, `resource-claim` — no `pr`, `branch`, `github-issue`, `code-review`,
  or similar software-development vocabulary appears in the core ontology).

## File layout

```
pyproject.toml
.gitignore
src/praxis_contracts/__init__.py
src/praxis_contracts/validator.py
schemas/v1/promise.schema.json
schemas/v1/requirement.schema.json
schemas/v1/capability.schema.json
schemas/v1/capability-advertisement.schema.json
schemas/v1/evidence-requirement.schema.json
schemas/v1/resource-claim.schema.json
examples/graph-requests-capability.json
examples/executor-advertises-capability.json
tests/test_valid_contracts.py
tests/test_malformed_contracts.py
tests/test_version_mismatch.py
docs/ontology.md
```

## Shared conventions (fixed in advance so schema tasks need no coordination)

- `$schema`: `"https://json-schema.org/draft/2020-12/schema"` on every schema file.
- `$id`: `"https://schemas.praxis.dev/v1/<file-name>"` on every schema file (nominal identifier,
  not required to resolve over the network — the validator loads schemas from local disk by
  path, never by fetching `$id`).
- Every schema's root object requires `"spec_version"`: `{"type": "string", "pattern":
  "^1\\.\\d+\\.\\d+$"}`.
- The constraint enum used by both `requirement.schema.json` and
  `evidence-requirement.schema.json` is the literal 3-value enum `["required", "preferred",
  "prohibited"]`, inlined in each file (two small inlined copies beat a shared `$defs` file for
  three total uses).
- Cross-file linkage is by **plain string convention**, not JSON Schema `$ref`, except where
  noted below — `Capability.satisfies[].kind` and `Promise.kind` use the same free-form string
  vocabulary but are not schema-linked, so schema authoring tasks never block on each other for
  content. The one place a `$ref` is used (`requirement.schema.json` → `promise.schema.json`,
  `capability-advertisement.schema.json` → `capability.schema.json`) is a same-directory
  sibling-file reference by filename (e.g. `"$ref": "promise.schema.json"`), which the validator
  resolves by loading both files from `schemas/v1/` — the referencing task only needs the
  referenced file's *name*, fixed in this plan's file layout above, not its content.
- Every schema sets `"additionalProperties": false` at every object level it defines, so unknown
  fields are rejected rather than silently accepted (fail closed).

## Tasks

### T1 — Bootstrap: Python project scaffolding

**Files:** `pyproject.toml`, `.gitignore`, `src/praxis_contracts/__init__.py`

**Depends on:** none

**Interfaces:** none (no functions/classes — packaging only). `src/praxis_contracts/__init__.py`
contains only `__version__ = "0.1.0"`.

**Steps:**
- [ ] Create `pyproject.toml` with `[build-system]` (`setuptools>=68`, `setuptools.build_meta`),
      `[project]` (`name = "praxis-contracts"`, `version = "0.1.0"`, `requires-python = ">=3.10"`,
      `dependencies = ["jsonschema>=4.18"]`), `[project.optional-dependencies] dev =
      ["pytest>=7"]`, `[tool.setuptools.packages.find] where = ["src"]`, and `[tool.pytest.ini_options]
      testpaths = ["tests"]`.
- [ ] Create `.gitignore` covering `__pycache__/`, `*.pyc`, `.pytest_cache/`, `*.egg-info/`,
      `.venv/`, `build/`, `dist/`.
- [ ] Create `src/praxis_contracts/__init__.py` with just `__version__ = "0.1.0"`.
- [ ] Confirm `python3 -c "import sys; print(sys.version)"` reports >=3.10 in this environment; if
      not, lower `requires-python` to match and note it in the PR description.

### T2 — Promise & Requirement schemas (request-side vocabulary)

**Files:** `schemas/v1/promise.schema.json`, `schemas/v1/requirement.schema.json`,
`examples/graph-requests-capability.json`

**Depends on:** none

**Interfaces (schema shapes):**
- `promise.schema.json` — object, `required: [spec_version, kind]`, `properties: {spec_version,
  kind: {type: string, pattern: "^[a-z0-9]+(-[a-z0-9]+)*$", description forbids model/vendor
  names}, parameters: {type: object, additionalProperties: true}}`, `additionalProperties: false`.
- `requirement.schema.json` — object, `required: [spec_version, requirements]`, `properties:
  {spec_version, requirements: {type: array, minItems: 1, items: {type: object, required:
  [promise, constraint], properties: {promise: {"$ref": "promise.schema.json"}, constraint:
  {enum: [required, preferred, prohibited]}}, additionalProperties: false}}}`,
  `additionalProperties: false`. This is the shape a graph node uses to declare what it needs
  without naming a model or vendor.

**Steps:**
- [ ] Write `schemas/v1/promise.schema.json` per the shape above, with a `description` on `kind`
      stating it must name an abstract capability class, never a model or vendor.
- [ ] Write `schemas/v1/requirement.schema.json` per the shape above, `$ref`-ing
      `promise.schema.json` by filename (same directory).
- [ ] Write `examples/graph-requests-capability.json`: a `requirement.schema.json` instance with
      `spec_version: "1.0.0"` and at least two entries in `requirements`, one `constraint:
      "required"` and one `constraint: "preferred"`, using `kind` values like `text-generation`
      and `code-execution` (illustrative abstract classes, not real product/model names).
- [ ] Re-read both schema files and the example for any literal model or vendor name (e.g.
      "gpt", "claude", "openai", "anthropic") and confirm there are none — this is acceptance
      criterion 3.

### T3 — Capability & Capability Advertisement schemas (offer-side vocabulary)

**Files:** `schemas/v1/capability.schema.json`, `schemas/v1/capability-advertisement.schema.json`,
`examples/executor-advertises-capability.json`

**Depends on:** none

**Interfaces (schema shapes):**
- `capability.schema.json` — object, `required: [spec_version, satisfies]`, `properties:
  {spec_version, id: {type: string}, satisfies: {type: array, minItems: 1, items: {type: object,
  required: [kind], properties: {kind: {type: string, pattern: "^[a-z0-9]+(-[a-z0-9]+)*$"},
  parameters: {type: object, additionalProperties: true}}, additionalProperties: false}}}`,
  `additionalProperties: true` (executors may attach executor-specific metadata not part of the
  core ontology).
- `capability-advertisement.schema.json` — object, `required: [spec_version, executor_id,
  capabilities]`, `properties: {spec_version, executor_id: {type: string, description: opaque
  identifier, must not encode a vendor/model name}, capabilities: {type: array, minItems: 1,
  items: {"$ref": "capability.schema.json"}}}`, `additionalProperties: false`. This is the
  document an executor publishes to advertise what it can do.

**Steps:**
- [ ] Write `schemas/v1/capability.schema.json` per the shape above. `satisfies[].kind` uses the
      same free-form vocabulary as `Promise.kind` (documented in a `description`, not schema-linked).
- [ ] Write `schemas/v1/capability-advertisement.schema.json` per the shape above, `$ref`-ing
      `capability.schema.json` by filename (same directory).
- [ ] Write `examples/executor-advertises-capability.json`: a `capability-advertisement.schema.json`
      instance with `spec_version: "1.0.0"`, an opaque `executor_id` (e.g. `"executor-7f3a"`,
      not a real vendor/product name), and at least one capability whose `satisfies[].kind`
      matches one of the `kind` values used in `examples/graph-requests-capability.json`
      (`text-generation`) so the pair reads as a matching request/offer.
- [ ] Re-read both schema files and the example for any literal model or vendor name; confirm
      none — acceptance criterion 3.

### T4 — Evidence requirement schema

**Files:** `schemas/v1/evidence-requirement.schema.json`

**Depends on:** none

**Interfaces (schema shape):**
- object, `required: [spec_version, evidence]`, `properties: {spec_version, evidence: {type:
  array, minItems: 1, items: {type: object, required: [proof_type, constraint], properties:
  {proof_type: {type: string, description: abstract proof category, e.g. "test-pass" or
  "peer-attestation" — illustrative only, not a fixed enum, so this schema itself does not bake
  in software-development-specific proof types as required vocabulary}, constraint: {enum:
  [required, preferred, prohibited]}, min_confidence: {type: number, minimum: 0, maximum: 1}},
  additionalProperties: false}}}`, `additionalProperties: false`.

**Steps:**
- [ ] Write `schemas/v1/evidence-requirement.schema.json` per the shape above. Keep `proof_type`
      an open string (not an enum) so this contract-level schema stays domain-neutral per the
      Epic constraint — do not enumerate software-development-specific proof types (e.g. do not
      write "unit-test-pass", "pr-approved" as enum values).
- [ ] Confirm the schema contains no software-development vocabulary (PR, TDD, branch, issue,
      code review) anywhere, including in `description` fields — acceptance criterion 4.

### T5 — Resource claim schema

**Files:** `schemas/v1/resource-claim.schema.json`

**Depends on:** none

**Interfaces (schema shape):**
- object, `required: [spec_version, claims]`, `properties: {spec_version, claims: {type: array,
  minItems: 1, items: {type: object, required: [resource_type, quantity], properties:
  {resource_type: {type: string, description: abstract resource category, e.g. "compute-slot" or
  "memory" — illustrative only, not a fixed enum}, quantity: {type: number, exclusiveMinimum: 0},
  unit: {type: string}}, additionalProperties: false}}}`, `additionalProperties: false`.

**Steps:**
- [ ] Write `schemas/v1/resource-claim.schema.json` per the shape above. Keep `resource_type` an
      open string (not an enum) for the same domain-neutrality reason as T4.
- [ ] Confirm no vendor/model-specific resource names appear (e.g. do not name a specific GPU
      model) — acceptance criterion 3.

### T6 — Validator module

**Files:** `src/praxis_contracts/validator.py`

**Depends on:** T1 (needs `pyproject.toml`'s `jsonschema` dependency and the package to exist to
be importable/testable)

**Interfaces:**
```python
class ContractValidationError(Exception):
    """Raised with a human-readable reason; .errors holds every underlying schema violation."""
    def __init__(self, reason: str, errors: list[str] | None = None) -> None: ...

def load_schema(schema_path: Path) -> dict: ...

def validate_document(
    instance: dict,
    schema_path: Path,
    *,
    expected_major_version: int = 1,
) -> None:
    """Fail-closed: returns None on success, else raises ContractValidationError.
    Checks, in order: (1) instance["spec_version"] matches
    f"^{expected_major_version}\\.\\d+\\.\\d+$" — else raises with reason
    "version mismatch: ..." naming the found and expected major version, without running
    full schema validation; (2) full draft-2020-12 structural validation via jsonschema,
    collecting every violation (not just the first) into .errors and raising with reason
    "schema validation failed: <n> error(s)" if any.
    """
```

**Steps:**
- [ ] Add `ContractValidationError` as specified.
- [ ] Add `load_schema(schema_path)`: read and `json.loads` the file at `schema_path`.
- [ ] Implement `validate_document`: first check `instance.get("spec_version")` against the
      expected-major-version pattern and raise `ContractValidationError` with a message that
      names both the found value and the expected major version if it fails, before doing
      anything else (this makes the version-mismatch failure mode distinguishable from a
      generic schema error, per acceptance criterion 6). Then run full schema validation.
- [ ] For schemas that `$ref` a sibling file by filename (`requirement.schema.json` →
      `promise.schema.json`, `capability-advertisement.schema.json` → `capability.schema.json`),
      register both files with the validator so the relative `$ref` resolves against the
      sibling in the same directory (`schema_path.parent / ref_filename`) rather than attempting
      network resolution of the nominal `$id`. Verify the exact registration API against the
      installed `jsonschema` package version's docs (the `referencing` library's `Registry` /
      `Resource` API replaced the older `RefResolver` in `jsonschema` >= 4.18) and cite the
      library/version in a code comment.
- [ ] Use `jsonschema.Draft202012Validator`, and use `.iter_errors(instance)` (not
      `.validate()`) so every violation is collected into `ContractValidationError.errors`, not
      just the first.

### T7 — Tests: valid contracts

**Files:** `tests/test_valid_contracts.py`

**Depends on:** T1, T2, T3, T4, T5, T6

**Steps:**
- [ ] Test that `examples/graph-requests-capability.json` validates against
      `schemas/v1/requirement.schema.json` via `validate_document` with no exception raised.
- [ ] Test that `examples/executor-advertises-capability.json` validates against
      `schemas/v1/capability-advertisement.schema.json` via `validate_document` with no
      exception raised.
- [ ] Construct one inline valid instance dict for `evidence-requirement.schema.json` (one
      `evidence` entry, `constraint: "required"`) and one for `resource-claim.schema.json` (one
      `claims` entry with positive `quantity`), and assert both validate with no exception.

### T8 — Tests: malformed contracts

**Files:** `tests/test_malformed_contracts.py`

**Depends on:** T1, T2, T3, T4, T5, T6

**Steps:**
- [ ] Take the valid `requirement.schema.json` instance from T7's fixture shape (re-declare
      inline; do not import from `test_valid_contracts.py`) and produce at least 3 malformed
      variants covering distinct failure modes: (a) drop a required field (`spec_version`), (b)
      set `constraint` to a value outside the 3-value enum, (c) add an extra top-level property
      not permitted by `additionalProperties: false`. Assert each raises
      `ContractValidationError`.
- [ ] Do the same (at least 1 malformed variant each) for `capability-advertisement.schema.json`,
      `evidence-requirement.schema.json`, and `resource-claim.schema.json`.
- [ ] Assert the raised exception's message is non-empty and does not silently swallow the
      failure (fail-closed, clear reason) — acceptance criterion 5.

### T9 — Tests: version mismatches

**Files:** `tests/test_version_mismatch.py`

**Depends on:** T1, T2, T3, T4, T5, T6

**Steps:**
- [ ] Take a valid `requirement.schema.json` instance, set `spec_version` to `"2.0.0"`, and
      assert `validate_document(..., expected_major_version=1)` raises
      `ContractValidationError` whose message mentions the version mismatch (not a generic
      schema error).
- [ ] Do the same for a valid `capability-advertisement.schema.json` instance with
      `spec_version` set to `"0.9.0"`.
- [ ] Assert that a correct `spec_version` (e.g. `"1.2.3"`) with `expected_major_version=1`
      passes the version check (does not raise for that reason), to confirm the check is
      major-version-only, not exact-match.

### T10 — Ontology documentation

**Files:** `docs/ontology.md`

**Depends on:** none (the schema shapes are fully specified above; this task writes prose from
this plan, not from reading other tasks' commits)

**Steps:**
- [ ] Write `docs/ontology.md` explaining: what a Promise is (an abstract, vendor/model-neutral
      request for a capability class); what a Capability is (what an executor can concretely do,
      advertised via a Capability Advertisement); the required/preferred/prohibited constraint
      vocabulary and where it applies (Requirement, EvidenceRequirement); the Evidence
      Requirement and Resource Claim shapes and what they are for (needed by later issues #6 and
      #7, not implemented here); and how matching is *intended* to work at the contract level
      only — a Capability's `satisfies[].kind` is compared against a Requirement's
      `requirements[].promise.kind` string, without specifying the matching *algorithm* (that is
      issue #5).
      list every schema file, its purpose, and its version directory (`schemas/v1/`).
- [ ] State the core architectural rule verbatim: "Graphs request promises/capabilities. They do
      not name models or vendors," and explain how the ontology enforces it (free-form
      `kind`/`resource_type`/`proof_type` strings, no enumerated model/vendor values anywhere).
- [ ] State the versioning convention: schema directory version (`v1`) plus per-instance
      `spec_version` field, and what a validator does on mismatch (rejects before generic schema
      validation, with a distinct message).

## Verification (for whichever task/tech-lead runs it after all tasks land)

- [ ] `pip install -e .[dev]` (or equivalent) then `pytest` — all of T7/T8/T9 pass.
- [ ] Manually confirm both example documents in `examples/` still validate after all schema
      tasks land (T7 already asserts this).
