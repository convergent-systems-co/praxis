# SPEC-041: Exact Goals Initial Publication

- Status: Proposed — architecture-owner review required
- Date: 2026-09-16
- Identity: exact artifact path and SHA-256 of frozen UTF-8 bytes
- Governing proposal: `docs/ADR/078-goals-initial-publication-authority.md` — `sha256:41ab3929094f35a23f568ed35890d6da95772d453cbed6088a4f4c2ef71f0010`

## Existing contracts preserved

- `docs/SPEC/011-package-catalog-and-dependency-trust.md` — `sha256:13837b09a70d04a600a192e4b7272ee82df13f48b3a5f1e7291910bd7f6289ba`
- `docs/SPEC/015-dynamic-command-registry-and-package-distribution.md` — `sha256:2e2ae8f34790c593c1efd6ad978948a56a10970d16960945181d5ec9d7800eed`
- `docs/SPEC/039-installation-lifecycle-contracts-successor.md` — `sha256:9fe5af8c621df3d59e371c59db93d4e2ecdd8a7d3c159a7c682248cef14755fd`

This proposed operation adds a local pre-acquisition lineage check; it does not
replace existing package verification, dependency or deployment semantics. The
historical distribution specification describes consumption and is not treated
as authority to mutate GitHub. No historical artifact is rewritten.

## 1. Fixed qualification boundary

The handler MUST reject identities outside this boundary:

| Identity | Exact value |
| --- | --- |
| Installation bootstrap digest | `sha256:3a4152a726102de408cc4e6ee329113ff8455e4924538bf28e0bab23e4995b00` |
| Installation root generation | `sha256:7e247747e70c88ad0feb59f485d31d3b2803e9049c22983587ddf61b500c1e47` |
| Publisher principal | `publisher:praxis-first-party` |
| Publisher generation | `sha256:c199600cb21987e9010917c60d8ea526c16aa5f4a58f98257f7a71a0108dd17d` |
| Package | `praxis.package.goals@0.1.0` |
| Executable | `sha256:455d1a6af72bd206071e52daf2b3bfbb906177a5eea36f1e6840fab0053c5581` |
| Manifest | `sha256:023a191f4844e05c32e85a464e23bf229bc00c5c29be07ea14ace21356bb5e9d` |
| Archive | `sha256:857e3ca52b8708376f4ed3b5db3578292b69605ea4af243df293c4b57702e156` |
| Signature envelope | `sha256:2296ed167c642648abca44d2f0e561b55f991e845575d074d1816d09b371fe59` |
| Signing provenance | `sha256:fc1d94e6bdc69e9d8e31b27b30db5ebd90945ff587e3f638fb2871aa19562d01` |
| Destination | `github.com/convergent-systems-co/praxis-packages` |
| Initial branch / tag | `refs/heads/main` / `refs/tags/goals/v0.1.0` |

These are qualification constraints, not grants. Runtime MUST load and verify
actual durable signing, publisher and installation lineage; constants alone are
not authority. Resolve the GitHub stable repository ID and ownership read-only
when preparing the request, show them for review, and bind them in the intent.
Do not invent an unobserved repository ID, commit SHA, release ID or grant.

## 2. Exact ActionIntent and authorization

Use the existing ActionIntent canonicalization and digest, with a strictly
validated versioned payload for this one operation. Required parameters and
preconditions MUST bind:

- installation/root and publisher generation identities above;
- signed asset names, sizes and digests, and signing-provenance digest;
- stable GitHub repository/owner identities and expected host/name;
- empty initial repository/ref state and absent release/tag;
- exact descriptor bytes/digest, parentless Git tree/commit identity and frozen
  commit metadata, branch and tag target;
- exact draft/final release settings and asset inventory;
- permitted ordered effects and immutable execution identity.

The descriptor records existing signed package/provenance identities. It does
not contain its own commit SHA or the later intent digest, avoiding digest
cycles. The intent binds the prepared commit. The distribution commit never
replaces the original source commit in signing provenance.

Persist the system-produced request and intent through existing protected-object
and authority machinery. The new closed profile permits the authenticated
installation owner to delegate only this exact intent to the exact publisher
under v4, with mandatory finite expiry and exact root/request/decision/model
bindings. Reject caller-authored approvals or arbitrary intent-file grants.

At admission, validate and atomically bind/consume the exact authorization with
the command/event/effect state for one execution. If ApprovalBinding is used,
its issuer, decision and operational generation must resolve from durable state;
its intent digest alone is insufficient. Prevent concurrent duplicate admission.
Recovery resumes the original execution without minting a second grant. Before
each new external mutation, revalidate authority, revocation/expiry and that the
step is still permitted. An exhausted admission token may identify an admitted
execution; it cannot authorize a new one.

Credentials authenticate the expected external account and permit transport
access. They MUST NOT create the Praxis authorization decision. No general VCS
lease or previously existing signing grant substitutes for this profile.

## 3. Bounded external effects

The trusted core handler executes only these steps:

1. Verify stable destination identity and absence preconditions; create the
   prepared parentless commit and required branch/tag without forced mutation.
2. Create the exact draft release targeting that tag.
3. Upload unchanged `praxis-package.json`, `praxis-package.tar.gz`, and
   `praxis-package.sig.json` with the bound inventory.
4. Read back repository identity, commit/tree, tag target, release identity and
   draft settings; download and hash every asset against the approved bytes.
5. Publish the verified draft; read back final release/ref/asset state.
6. Record completion only after the required observations verify.

Repository creation, automatic adoption of unrelated matching objects, forced
ref updates, asset replacement, conflict deletion, and other destinations are
forbidden. Freeze complete release settings, not ambient CLI defaults. Check
preconditions immediately before relevant effects; use conditional ref creation
where available. Concurrent or unexpected state fences execution. There is no
claim of global atomicity across GitHub and SQLite.

## 4. Existing effect lifecycle and completion evidence

Reuse commands/events, EffectRecord, the coordinator's revalidator/precondition
checker, and pending/dispatched/succeeded/failed/unknown/reconciling states.
Record dispatch before network mutation. An idempotency-key string alone MUST
NOT be treated as proof that GitHub provides idempotent execution. Explicitly
reconcile ambiguous responses and dispatched/unknown/reconciling steps; resume
only with sufficient evidence, otherwise fail closed. No blind retry or automatic
destructive rollback. Authority expiry may permit read-only reconciliation but
not additional remote mutations.

Persist the exact authorized intent and decision/generation references, step
request and observed results, stable repository/release/asset IDs, commit/tree,
tag target, asset digests, timestamps and reconciliation evidence in existing
command/event/effect payloads. Append a versioned completion event binding their
identities/digests and the signing-provenance digest. The event is evidence, not
new authority. Records claiming success without the authorized dispatch lineage
are invalid even when remote bytes match. If an uncertain creator/outcome cannot
be established, keep it unresolved rather than invent provenance.

No new generalized PublicationReceipt contract or publication ledger is required.
Retain enough existing records to reconstruct this execution after restart.

## 5. Local acquisition check

For this Goals qualification path, before existing package verification:

- resolve the authoritative local completion event and its exact authorized
  intent, decision/generation, signing receipt and effect lineage;
- verify authorization was legitimate for the recorded execution and apply
  current applicable invalidation/compromise rules; grant expiry alone does not
  turn historically authorized publication into unauthorized publication;
- resolve the exact repository/release, verify commit/tag and release identities,
  and compare downloaded manifest/archive/signature bytes to completion evidence;
- pass those same bytes to the unchanged existing package verifier;
- retain the completion-event identity with this acquisition's command/event
  evidence, without revising the generic package-verification protocol.

Missing, inconsistent or substituted evidence fails closed for this operation.
Do not accept user-supplied success flags or bypass the local check through a
fallback within this qualification path. This does not claim a general restriction
on all third-party acquisition. No cross-installation trust is introduced.
Acquisition success grants no package.deploy or runtime authority.

## 6. Scope and acceptance

The governing ADR's scope firewall applies in full. Issue #127 is non-normative
parked knowledge. These proposed semantics require explicit frozen-artifact
acceptance and separate implementation/operational authorization. No source,
schema, authority, signing or external state is changed by this specification.
