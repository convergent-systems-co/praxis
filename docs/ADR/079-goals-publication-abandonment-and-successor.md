# ADR-079: Goals Publication Abandonment and Exact-State Successor

- Status: Proposed — architecture-owner review required
- Date: 2026-09-16
- Identity: exact artifact path and SHA-256 of frozen UTF-8 bytes
- Predecessor: `docs/ADR/078-goals-initial-publication-authority.md` — `sha256:41ab3929094f35a23f568ed35890d6da95772d453cbed6088a4f4c2ef71f0010`
- Related predecessor: `docs/SPEC/041-goals-initial-publication.md` — `sha256:f8e107bec312b1754a71280550df7729f67bc142eaebe46c081fb98ef081da7e`
- Related plan predecessor: `docs/PLAN/007-goals-initial-publication.md` — `sha256:eabbef03bf81b215d0a4d89a58a9a94743b46ead55f288ef9768b006d8b7b51f`

## Decision requested

Add only the recovery boundary required after the one authorized initial Goals
publication has an irreducibly unknown external effect. Preserve the accepted
initial-empty-repository path unchanged. Add (1) owner-authorized terminal
abandonment of its exact execution, (2) one separately authorized successor
ActionIntent from the exact established repository/release state below, and
(3) bounded sanitized GitHub dispatch outcome evidence for future effects.

This is a proposal, not authority to implement, adopt an authority model, issue
a grant, abandon an execution, or mutate GitHub.

## Relationships and immutable predecessors

ADR-078, SPEC-041 and PLAN-007 remain immutable governing architecture for the
original initial-publication path. This is an additive successor for exactly
one established-state transition after that path cannot continue. It does not
replace or reinterpret the original empty-repository precondition, alter
historical v1-v4 authority semantics, or activate the parked design in issue
#127. The predecessor unknown effect and every predecessor observation remain
historical evidence.

The accepted boundaries for durable effects, exact authority generations,
command/event mutation, and package deployment continue to apply. Revocation
is not part of this decision. Execution lifecycle and authority lifecycle are
separate; an exact execution fence does not rewrite or automatically revoke
the historical publication decision.

## Fixed predecessor and successor boundary

The only predecessor is the exact original Goals request/ActionIntent and its
one execution in this installation. The established remote state to which the
successor may bind is:

- repository `convergent-systems-co/praxis-packages`, stable repository ID
  `1372388187`, owner ID `263966243`;
- parentless commit `fbdc98828d49cf5ddd515edf91d457b606e89a97`, tree
  `06476050446988f592cd2064823ff73c5a7a09f0`;
- `refs/heads/main` and `refs/tags/goals/v0.1.0` both point to that commit;
- exact draft release ID `389997269`, tag `goals/v0.1.0`, name
  `praxis.package.goals@0.1.0`, with the original fixed draft settings;
- the current release asset inventory is empty;
- predecessor refs and draft effects are succeeded; its manifest effect is
  permanently unknown, and its reconciliation evidence remains unresolved;
- predecessor publication completion was never established.

These facts bind the proposal only. Before any future request is prepared, the
handler must freshly read and verify every state component. A mismatch rejects
the successor; it does not authorize repair, adoption, replacement, or cleanup.

The successor uses operation `publish-goals-from-established-state` under the
closed contract `goals-established-state-publication/1`. Its terminal objective
is the already-authorized package identity and release destination. Its only remote effects are exact existing manifest,
archive and signature uploads, draft inventory/byte verification, publishing
the existing exact draft release, and final read-back verification. It does
not recreate or update refs or the draft release. The old unknown manifest
effect is neither retried nor resolved by this successor: any manifest upload
under the successor has a distinct ActionIntent, effect identity, authority,
and current absence precondition.

## Execution abandonment

The old execution may be terminally abandoned by one exact owner-authorized,
append-only local command/event. It binds the predecessor request and digest,
ActionIntent digest, authority generation reference/version/digest, and every
admitted effect identity/state/result/reconciliation-evidence digest. It
records `completion_established=false` and the explicit terminal status
`abandoned` for the execution. It does not alter an EffectRecord or its result.

Abandonment is serialized with effect admission/dispatch claims on the exact
execution identity. It is refused while an external mutation remains actively
claimed as dispatched; once committed, the durable abandonment fence prevents
that request from dispatching or completing again. Read-only historical
inspection remains possible. Abandonment does not claim the remote state was
undone and does not infer anything about the historical unknown effect.

The old decision/generation remains historically legitimate and retains its
own expiry. Revocation, if independently warranted, remains a separate
governed transition. No new package.publish authority is inferred from
abandonment or from the predecessor's succeeded effects.

## Exact successor authority

V4 is immutable and closed around the original `publish-initial-goals`
ActionIntent with the empty-repository precondition. A new authority-model
version is therefore required. The minimum v5 delta preserves every v1-v4
meaning and adds one closed profile for this exact established-state Goals
successor operation. It binds this installation, the existing
`publisher:praxis-first-party` generation, `praxis.package.goals@0.1.0`, the
existing signing provenance and exact signed bytes, repository/owner IDs,
commit/tree, refs, draft release ID/settings, empty asset inventory, exact
permitted effects, predecessor abandonment evidence, finite expiry, and one
fresh ActionIntent digest.

V5 does not grant general repository or GitHub mutation, authorize another
package/version/destination, or imply signing or deployment authority. The
successor needs a fresh request, owner decision, and operational generation
under the adopted v5 model. V5 preview, adoption, and that exact grant are
separate later governance transitions; none is authorized here. Existing
package.deploy authority remains separate and unchanged.

## Bounded dispatch evidence

Future GitHub dispatches retain a versioned, bounded, sanitized outcome in the
existing effect evidence. It distinguishes process launch failure from a
started process; preserves bounded stdout only when safe; stores sanitized,
bounded stderr; and records HTTP status and provider request ID when available.
Classification is conservative: proven local/pre-dispatch failure, explicit
provider response, acknowledged success, or ambiguous. A response is not a
substitute for read-back. Provider errors whose semantics permit partial
effects remain unknown. Missing/lost responses remain unknown.

Credentials, authorization headers, tokens, request bodies, and unrestricted
diagnostics are never retained. This improves future reconciliation evidence;
it cannot retroactively change the predecessor manifest's unknown state.

## Exact implementation denominator

1. Implement the exact owner-authorized abandonment command/event and durable
   fence, preserving all predecessor effects and authority lineage without
   changing any effect state or revoking the old generation.
2. Implement one validated successor ActionIntent/handler and the v5 closed
   authority rule, using existing governance, effect, event, reconciliation,
   signing-provenance, verification, and acquisition mechanisms.
3. Implement bounded sanitized GitHub dispatch evidence in the existing
   transport/effect path.

Focused adversarial qualification is specified separately in PLAN-008; it does
not add another implementation item. No generic recovery framework is in
scope.

Source/model implementation authorization, v5 adoption, exact successor
publication authorization, and the eventual external publication are separate
decisions. No such action follows from proposal acceptance alone.

## Scope firewall

- Generic execution supersession or abandonment for unrelated operations.
- Generalized `S → T` successor-execution or partial-publication architecture.
- Generalized provider observability, transport, or reconciliation.
- Changes to unrelated providers or operations.
- Portable/cross-installation publication trust or provenance.
- New package-distribution architecture or destinations/transports.
- Automatic authority revocation or reinterpretation of old authority.
- Re-signing Goals or changing package.deploy authority.
- Any mutation, cleanup, or retry of the current predecessor execution.

The current predecessor's unknown manifest effect remains unknown permanently.
Nothing here establishes publication completion or authorizes a consequential
successor transition outside governance.
