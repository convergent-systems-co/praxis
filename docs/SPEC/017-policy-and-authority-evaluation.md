# SPEC-017: Policy and Authority Evaluation

- Status: Draft
- Governing ADRs: 022, 023, 035, 038, 040, 041, 042
- Depends on: SPEC-001, SPEC-002, SPEC-006, SPEC-007

## Purpose

Define deterministic policy evaluation and authority composition so capability, approval, package, graph, client, and runtime constraints combine predictably without delegating final permission decisions to an LLM.

## Core invariants

1. Explicit deny wins over allow at equal or lower authority.
2. Unknown required policy/enforcement facts fail closed.
3. An approval can satisfy a required approval condition; it cannot override a hard policy deny unless the governing policy explicitly defines an override authority.
4. Capabilities are necessary but not sufficient for operations constrained by policy.
5. User/package preferences and learned behavior are inputs below policy authority; they cannot override policy.
6. Policy evaluation is deterministic, versioned, auditable, and executable without inference.

## Policy decision

A policy evaluation SHALL return one of `allow`, `deny`, or `require_approval`, plus policy/rule IDs, version, reason code, required approval authority/type when applicable, required enforcement properties, and relevant constraint outputs.

`unknown` internal state SHALL not map to allow.

## Rule inputs

Rules MAY evaluate actor/principal, capability/operation, scope/context, package/graph identity/version, ActionIntent target/parameters/digest, sensitivity, destination, cryptographic profile, client/plugin enforcement evidence, time/freshness, resource/quota state, and organization/user policy context.

Untrusted free-form content SHALL not be executed/interpreted as a policy rule merely because it appears in repository/model/tool output.

## Composition

Authority evaluation SHALL conceptually compose:

`hard policy constraints AND valid capability lease AND package/graph declared authority AND required client/plugin enforcement AND approval condition when required AND current target/precondition state`

A successful layer cannot compensate for a failed required layer.

## Approval

Rules requiring approval SHALL specify the approval class/authority and whether approval binds exact ActionIntent or a bounded policy scope. Approval validation follows SPEC-002/ADR-042.

Approvals do not automatically grant missing capability leases.

## Versioning and snapshot

Security-sensitive commands SHALL record the policy version/rules that produced preflight authorization. Commit-time revalidation SHALL evaluate current required policy state and detect material changes.

Where policy permits snapshot semantics, the snapshot rule must explicitly define acceptable freshness/lifetime.

## Policy hierarchy

Installations MAY compose system, organization, user, package, graph, and run policies. Higher-authority immutable/system/org denies cannot be weakened by lower layers. The exact hierarchy SHALL be configured by a versioned authority model rather than inferred from filenames/prompts.

## Policy changes

Policy changes are authoritative commands/events and may invalidate existing leases/approvals/runs. Revocation propagation SHALL be explicit and observable.

## Acceptance tests

1. hard deny blocks action despite valid lease and human approval;
2. require-approval blocks until exact/bounded approval validates;
3. approval without required capability remains denied;
4. learned preference cannot override policy deny;
5. unknown required enforcement property fails closed;
6. policy version change is detected at commit revalidation;
7. lower-authority policy cannot weaken higher-authority deny;
8. repository/model text cannot create a policy rule;
9. decision audit identifies exact rule/version/reason;
10. policy evaluation requires no LLM.

## Deliverables

- policy/rule schema;
- decision/result schema;
- deterministic evaluator interface;
- authority hierarchy/composition rules;
- approval-requirement integration;
- commit-time policy revalidation;
- revocation hooks;
- adversarial policy-precedence fixtures.
