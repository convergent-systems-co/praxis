# Governed Execution Supervision Decision Packet v1

Status: Accepted by architecture owner

Date: 2026-09-15

## Decision requested

The architecture owner accepted the exact execution-supervision artifacts
below. Governed live supervision and the associated CLI/control-plane
qualification are now prerequisites for `JAPETELLA_LONGITUDINAL_READY`.

The acceptance does not itself authorize release publication, signing, package
activation, installation, or Japetella execution.

## Exact proposal artifacts

- ADR-075: `docs/ADR/075-governed-execution-supervision.md`
  - SHA-256: `sha256:9f1063f4a2670ed92fd5c8e4e7c9c34ae02b422c5e87c4204a9b3883fd3012a6`
- SPEC-038: `docs/SPEC/038-governed-execution-supervision.md`
  - SHA-256: `sha256:1345a1d720e25cc2cfdc6afff0e98cc72f22fecaedaef910ab9457ed60c222fe`

The digests are calculated from the exact accepted proposal bytes. The formal
acceptance record is [acceptance v1](governed-execution-supervision-acceptance-v1.md).

## Minimum architecture

Reuse the existing local append-only event store, provenance envelope,
projection model, controller authority boundary, provider adapter boundary,
and core/dynamic CLI architecture. Add only:

1. typed activity/state events for the active turn;
2. bounded, redacted provider-message observations;
3. a restartable CLI follow/query projection;
4. exact-scope durable human intervention commands;
5. controller checks at safe boundaries;
6. a controller-owned repeated bounded-transition mode for continuous
   execution.

No second governance store or CLI authority model is proposed.

## Advisory challenge

Architecture review recommendation: **proceed to owner decision, with the
proposal unchanged**. The design reuses ADR-031 event history and ADR-035
command/query separation, preserves ADR-040 evidence/authority separation, and
keeps ADR-042 exact approval binding at the authority boundary.

Security challenge: provider output, tool output, human commentary, and
replayed events must remain non-authoritative unless promoted through existing
deterministic validation. Allowlisted payloads, sensitivity classification,
artifact references, exact turn binding, and stale/replay rejection are
required. A regex-only secret filter is insufficient as the sole boundary.

Runtime challenge: event persistence must not wait for the observer, provider
failure must not create fabricated activity, and cancellation/suspension must
report requested versus effective state. The provider process boundary cannot
promise rollback of filesystem or irreversible external effects.

Adversarial challenge: qualification must exercise malformed and substituted
events, stale constraints, provider claims, secret leakage, reconnect/replay,
crash during streaming, cancellation at effect boundaries, authority requests,
revocation, blocker, and completion paths. These are specified in SPEC-038 and
are release gates, not optional telemetry tests.

## Explicitly parked

- final TUI and rich visualization;
- token-by-token private reasoning/model internals;
- distributed event streaming or a general observability platform;
- arbitrary chat collaboration;
- unrestricted raw transcript retention;
- #117 and #118, unless implementation demonstrates a direct dependency.

## Release-candidate impact

The supervision contract is part of the minimum Japetella release candidate.
The candidate cannot claim `JAPETELLA_LONGITUDINAL_READY` until the proposal is
accepted, implemented, and qualified. Existing trust, package, conformance,
authority, and Goals lifecycle gates remain in force.

## Recorded owner decision

The architecture owner decided:

1. accept ADR-075 and SPEC-038 as written;
2. accept the closed typed event vocabulary recorded in acceptance v1;
3. accept continuation of already-authorized bounded reversible work after
   observer loss, with stopping before a boundary requiring unavailable human
   supervision or authority;
4. include continuous mode in the minimum Japetella release boundary.
