# Praxis Contracts Ontology

This document describes the vocabulary defined in `schemas/v1/` — the machine-readable
contracts that let a graph request "what it needs" and let an executor advertise "what it can
do," without either side ever naming a specific model or vendor.

## Core architectural rule

> Graphs request promises/capabilities. They do not name models or vendors.

Every schema in this ontology enforces this rule by construction, not by convention:

- The fields that name a capability class (`Promise.kind`, `Capability.satisfies[].kind`) are
  free-form, lowercase-hyphenated strings matching `^[a-z0-9]+(-[a-z0-9]+)*$` — an open
  vocabulary (e.g. `text-generation`, `code-execution`), never an enumerated list of real
  product, model, or vendor names.
- The fields that name a proof category (`EvidenceRequirement.evidence[].proof_type`) and a
  resource category (`ResourceClaim.claims[].resource_type`) are likewise open strings, not
  enums, and their descriptions are illustrative only.
- `CapabilityAdvertisement.executor_id` is documented as an opaque identifier that must not
  encode a vendor or model name.
- No schema, field name, or description anywhere in `schemas/v1/` references a specific model
  or vendor.

## Promise

A **Promise** (`schemas/v1/promise.schema.json`) is an abstract, vendor/model-neutral request
for a capability class. It names *what kind* of capability a graph node needs — for example
`text-generation` or `code-execution` — and never *who* or *what* should provide it. A Promise
carries a `spec_version`, a `kind`, and an open-ended `parameters` object for any
capability-specific configuration.

## Requirement

A **Requirement** (`schemas/v1/requirement.schema.json`) is the shape a graph node uses to
declare a set of Promises it needs, each paired with a **constraint**:

- `required` — the node cannot be satisfied without this Promise being met.
- `preferred` — the node prefers this Promise be met, but can proceed without it.
- `prohibited` — the node must not be matched to a capability satisfying this Promise.

This same three-value constraint vocabulary (`required` / `preferred` / `prohibited`) is reused
by Evidence Requirement below, so both "what capability do I need" and "what proof do I need"
are expressed the same way.

## Capability and Capability Advertisement

A **Capability** (`schemas/v1/capability.schema.json`) is an abstract, vendor/model-neutral
statement of what an executor can concretely do. It lists one or more `satisfies` entries, each
naming a `kind` — using the same free-form vocabulary as `Promise.kind` — plus an open
`parameters` object describing that capability's concrete configuration (e.g. a context-window
size). Unlike Promise, Capability allows additional top-level properties, since an executor may
attach executor-specific metadata that is not part of the core ontology.

A **Capability Advertisement** (`schemas/v1/capability-advertisement.schema.json`) is the
document an executor publishes to advertise what it can do: an opaque `executor_id` plus one or
more Capabilities.

## Evidence Requirement

An **Evidence Requirement** (`schemas/v1/evidence-requirement.schema.json`) is the shape a graph
node uses to declare what proof it needs before accepting a claimed outcome. Each entry names a
`proof_type` (an open, illustrative string such as `test-pass` or `peer-attestation` — not a
fixed enum, so this contract-level schema stays domain-neutral), a `constraint`
(`required` / `preferred` / `prohibited`), and an optional `min_confidence` between 0 and 1.

This shape exists so that later issues (#6, evidence gates) have a contract to build against; it
defines the shape of an evidence requirement only, not how evidence is graded or how a claimed
outcome is verified against it — that grading/verification logic is out of scope for this issue.

## Resource Claim

A **Resource Claim** (`schemas/v1/resource-claim.schema.json`) is a set of abstract resource
claims a graph node holds or requests. Each entry names a `resource_type` (an open, illustrative
string such as `compute-slot` or `memory` — not a fixed enum, and never a specific vendor or
hardware model), a positive `quantity`, and an optional `unit`.

This shape exists so that later issues (#7, resource/scheduling) have a contract to build
against; it defines the shape of a resource claim only. Issue #7 (see
[`docs/resources.md`](resources.md)) now defines that scheduling/lease layer on top of this same
schema.

## How matching is intended to work (contract level only)

At the contract level, a Capability is considered to satisfy a Requirement's Promise when a
`Capability.satisfies[].kind` string equals a `Requirement.requirements[].promise.kind` string.
This ontology defines and validates the *shapes* on both sides of that comparison and the
string vocabulary they share — it does not specify the matching *algorithm* (how a scheduler
searches available Capabilities, ranks multiple matches, applies `preferred` vs. `required`
constraints, or resolves `prohibited` conflicts). That algorithm is implemented in
`src/praxis_executors/matching.py`; see [`docs/executors.md`](executors.md) for its semantics.

## Schema files

All schemas live under `schemas/v1/` — `v1` is the schema directory's major version. Each file
is a plain JSON Schema (draft 2020-12) document:

| File | Purpose |
| --- | --- |
| `schemas/v1/promise.schema.json` | A single abstract capability-class request (`kind` + `parameters`). |
| `schemas/v1/requirement.schema.json` | A graph node's list of Promises, each with a `required`/`preferred`/`prohibited` constraint. References `promise.schema.json`. |
| `schemas/v1/capability.schema.json` | A single abstract statement of what an executor can do (`satisfies[].kind` + `parameters`). |
| `schemas/v1/capability-advertisement.schema.json` | The document an executor publishes: an `executor_id` plus a list of Capabilities. References `capability.schema.json`. |
| `schemas/v1/evidence-requirement.schema.json` | A graph node's list of proof requirements (`proof_type`, constraint, optional `min_confidence`). |
| `schemas/v1/resource-claim.schema.json` | A graph node's list of abstract resource claims (`resource_type`, `quantity`, optional `unit`). |

Two accompanying example documents under `examples/` show a matching request/offer pair:
`examples/graph-requests-capability.json` (a `requirement.schema.json` instance) and
`examples/executor-advertises-capability.json` (a `capability-advertisement.schema.json`
instance), whose `kind` values overlap on `text-generation`.

## Versioning

Versioning has two layers:

1. **Schema directory version** — `schemas/v1/` is the major version of the ontology itself. A
   breaking change to the vocabulary's shape would land in a new directory (e.g. `schemas/v2/`).
2. **Per-instance `spec_version`** — every instance document (not the schema files themselves)
   carries a top-level `spec_version` string field matching `^1\.\d+\.\d+$` for a `v1` schema.
   This lets instances evolve their minor/patch version independently of the schema directory.

A validator checks an instance document's `spec_version` against the schema directory's expected
major version *before* running generic JSON Schema structural validation. If the major version
in `spec_version` does not match, the validator rejects the document immediately with a distinct
"version mismatch" error naming both the found and expected major version — it does not fall
through to generic schema-shape errors for a version mismatch, so the two failure modes are
always distinguishable by their error message. Only once the version check passes does the
validator run full draft-2020-12 structural validation and report any further violations.
