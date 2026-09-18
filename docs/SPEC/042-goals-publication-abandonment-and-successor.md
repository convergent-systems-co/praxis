# SPEC-042: Goals Publication Abandonment and Exact-State Successor

- Status: Proposed — architecture-owner review required
- Date: 2026-09-16
- Identity: exact artifact path and SHA-256 of frozen UTF-8 bytes
- Governing proposal: `docs/ADR/079-goals-publication-abandonment-and-successor.md` — `sha256:01c40d0e14062539b3d34d49bae08159b2a68783aff942979c839565fb6601e0`
- Additive predecessor: `docs/SPEC/041-goals-initial-publication.md` — `sha256:f8e107bec312b1754a71280550df7729f67bc142eaebe46c081fb98ef081da7e`
- Authority predecessor: `docs/ADR/078-goals-initial-publication-authority.md` — `sha256:41ab3929094f35a23f568ed35890d6da95772d453cbed6088a4f4c2ef71f0010`

This specification defines one exact local abandonment and one exact successor
operation for the Goals publication boundary. It does not revise SPEC-041's
initial-empty-repository behavior. All identities below are closed qualification
constraints, not caller-selectable configuration.

## 1. Predecessor abandonment contract

### Command and authorization

The control plane SHALL expose `goals-publication.abandon` command version `1`
and append `goals-publication.abandoned` event version `1` for the exact
predecessor publication request. The command SHALL require an affirmative
decision by this installation's authenticated governance owner through the
existing governance kernel. It grants no package.publish capability and no
external mutation permission. The command SHALL bind:

- exact predecessor request ID/version/digest and ActionIntent ID/version/digest;
- exact publication authority decision and generation ref/version/digest;
- execution identity and every admitted effect ID, step, state, attempt count,
  and digest of request, observed result, and reconciliation evidence;
- owner decision identity, reason, command/event identity, and timestamp.

The command is idempotent only for byte-identical replay. A different payload
for an already abandoned execution MUST fail closed. It MUST reject if an
external effect is actively claimed as dispatched. Abandonment and a new
dispatch claim MUST serialize against the same exact execution fence/sequence
so either the dispatch claim wins first and abandonment waits/fails, or
abandonment wins and dispatch is denied.

### Durable event and invariants

The append-only event SHALL state `execution_status=abandoned` and
`completion_established=false`. It SHALL preserve the exact pre-abandonment
effect snapshot and reconciliation-event identities. The event MUST NOT update,
delete, or reinterpret any EffectRecord, including the manifest's `unknown`
state. The two succeeded effects remain succeeded; the manifest remains
unknown; its unresolved reconciliation evidence remains unresolved. No
completion event is emitted.

After this event commits, all external mutation and completion paths for the
predecessor request SHALL fail before effect dispatch. A retry, resume, or
reconciliation result MUST NOT reopen the abandoned execution or resolve its
unknown effect. Read-only inspection may show the immutable history. The
abandonment event does not describe, reverse, or repair external reality.

The predecessor authority decision/generation remains unchanged and retains
its recorded expiry. Abandonment MUST NOT automatically revoke or invalidate
that authority. A separately authorized revocation remains independent and
outside this operation.

## 2. Successor ActionIntent contract

### Exact established start state

The new operation SHALL freshly query and require all of the following before
persisting a successor request and again before each relevant external effect:

- GitHub host/name and stable repository ID `1372388187`, owner ID `263966243`;
- commit `fbdc98828d49cf5ddd515edf91d457b606e89a97`, tree
  `06476050446988f592cd2064823ff73c5a7a09f0`;
- `refs/heads/main` and `refs/tags/goals/v0.1.0` both target that commit;
- existing release ID `389997269`, tag `goals/v0.1.0`, exact package name,
  and original draft settings (`draft=true`, `prerelease=false`,
  `generate_release_notes=false`, `make_latest=false`, and body
  `Exact signed Goals initial publication; intent 7faf8b95afeb3bdd58534b0e1139f01169b4d417cd8d30e3b77a1533ad84aad1`);
- release asset inventory is empty;
- exact predecessor abandonment event and unresolved manifest effect remain
  durably present, with no predecessor completion event.

Any difference is a conflict and MUST stop before mutation. The handler SHALL
not create, adopt, repair, replace, or delete remote objects to make the
preconditions pass.

### Intent and fresh authority

The system SHALL produce a fresh canonical ActionIntent with operation
`publish-goals-from-established-state`, contract
`goals-established-state-publication/1`, a new identity, and a new digest. It
SHALL bind the exact current start-state identities above; the
installation and publisher generations; package/version; unchanged existing
manifest/archive/signature bytes, names, sizes, and digests; existing signing
provenance; exact predecessor request/abandonment identities; exact permitted
effects; and finite expiry. The intent SHALL explicitly state that the prior
manifest outcome remains unknown and is not being resolved by the successor.

V4's closed rule remains byte-for-byte and semantically unchanged. A successor
authority-model version SHALL retain v1-v4 validation and add exactly one
closed profile, `GOALS_PUBLICATION_FROM_ESTABLISHED_STATE`, bound to this
successor contract. The rule SHALL delegate only `package.publish` for this
ActionIntent to the exact existing first-party publisher generation and this
installation/repository/package. The owner must approve a fresh exact request
after the successor model has been separately reviewed and adopted. No
predecessor authority, successful effect, credential, root ownership alone,
or v5 adoption substitutes for that fresh grant.

### Permitted external effects

The authorized sequence is exactly:

1. Upload the unchanged `praxis-package.json` bytes as a new successor effect.
2. Upload the unchanged `praxis-package.tar.gz` bytes.
3. Upload the unchanged `praxis-package.sig.json` bytes.
4. Read back the existing draft release inventory and download/hash all three
   assets against the successor intent and original signing provenance.
5. Publish the existing exact draft release.
6. Read back repository, refs, tag, release identity/settings, complete asset
   inventory, and downloaded asset hashes; record completion only on a full
   match.

No ref or tag push, ref update, draft-release creation/update, asset
replacement, conflict cleanup, or other remote effect is permitted. Each
successor effect has its own deterministic identity under the fresh request;
no predecessor EffectRecord is resumed or reused. Immediately before the
manifest upload, the exact asset inventory must be empty. Before archive
upload, inventory must contain only the exact verified successor manifest;
before signature upload, it must contain only that manifest and the exact
successor archive. Before publish, it must contain exactly the three bound
assets and their downloaded bytes must match. Each observation is read back
and bound to the successor intent. Any missing, unexpected, or mismatched
asset stops the execution without replacement or cleanup. Unknown successor
outcomes enter the existing reconciliation path and are never blindly retried.

### Completion and acquisition lineage

The successor completion event SHALL bind the new ActionIntent, fresh decision
and authority generation, signing provenance, exact observed start state,
predecessor abandonment event, all successor effect identities/results, and
the final verified state. It SHALL explicitly preserve the predecessor
manifest's unknown status; that predecessor is context, never evidence of a
successful or failed successor step.

The local Goals acquisition check SHALL accept only this complete successor
lineage or the original valid SPEC-041 completion lineage. For a successor, it
must resolve the fresh grant and effect evidence, verify the exact final
release/assets and downloaded digests, and pass the same bytes to the existing
package verifier. Acquisition does not grant package.deploy authority.

## 3. Successor authority model

The v5 model is an additive immutable successor. It preserves all v1-v4 model
identities, validators, and historical meanings. Its only semantic delta is
the exact new `GOALS_PUBLICATION_FROM_ESTABLISHED_STATE` profile defined above.
It grants no general GitHub/repository mutation, package signing, deployment,
other package/version/destination, or arbitrary asset authority. Model preview,
owner review, and adoption are separate transitions. This specification does
not adopt v5 or issue the successor operational grant.

## 4. GitHub transport outcome evidence

The existing GitHub adapter SHALL return a versioned bounded dispatch outcome
to the existing EffectRecord lifecycle. At minimum it records:

- whether the process failed to launch or started;
- bounded stdout only when safe to retain;
- bounded sanitized stderr;
- HTTP status and GitHub request ID when exposed by the transport;
- conservative classification: `local_pre_dispatch_failure`,
  `provider_response`, `acknowledged_success`, or `ambiguous`;
- effect/request identity and observation time.

The implementation MUST NOT persist credentials, authorization headers,
tokens, request bodies, or unbounded process/provider diagnostics. Sanitization
must be deterministic, tested against secret-shaped output, and applied before
durable persistence. Provider responses do not replace read-back verification.
Only outcomes whose provider semantics prove no effect may be classified as
known failure; partial-effect responses and lost/missing responses remain
unknown. A successful process exit without a valid exact response is not
completion.

This schema applies only to this fixed GitHub publication handler. It does not
change the historical predecessor manifest EffectRecord or reconciliation
event, and MUST NOT be used to rewrite that unknown outcome.

## 5. Scope and fail-closed cases

Reject wrong installation, publisher generation, signing provenance, package,
asset digest, repository/owner, commit/tree, ref/tag target, release ID/settings,
predecessor/abandonment identity, non-empty inventory, expired/revoked new
grant, changed precondition, unauthorized effect, ambiguous outcome, missing
read-back, or mismatched completion/acquisition lineage. No error authorizes a
manual GitHub mutation. Signing provenance and package.deploy authority remain
unchanged.
