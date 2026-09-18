# ADR-078: Exact Goals Initial Publication Authority

- Status: Proposed — architecture-owner review required
- Date: 2026-09-16
- Identity: exact artifact path and SHA-256 of frozen UTF-8 bytes
- Source baseline: `6a40160bb0445b19679cede20d5ef781142f28e7`
- Source tree: `e704770a451866e5e09227644da3914e41b212bf`

## Decision requested

Authorize the architectural addition of one closed delegation rule and one
publication operation for the existing signed `praxis.package.goals@0.1.0`,
this dogfood installation, its existing publisher generation, and initial
publication into the existing empty `convergent-systems-co/praxis-packages`
GitHub repository. This proposal is not operational authorization.

The trust gap is authorization and evidence for the external mutation. The
package signature and its historical signing authority are already valid.
Signing possession, GitHub credentials, matching remote bytes, and installation
ownership alone do not authorize repository mutation.

## Governing constraints and relationships

- `docs/ADR/058-durable-effect-outcome-lifecycle.md` — `sha256:fd2e271795a84c9d3511e9fe1e97ab7493bd9fecd126298884bc48f027e344a3`
- `docs/ADR/069-first-installation-governance-principal-bootstrap.md` — `sha256:b3516166737df0b1f0897461795910b3cdfc4b8ec4400b99127c5caa691369f4`
- `docs/ADR/070-turn-owned-provider-workspaces.md` — `sha256:e1c701e11e53f1e31674e84e41a3cbc4c572dc7ae79e2c878c672f17aaf446b5`
- `docs/ADR/076-installation-lifecycle-contracts-successor.md` — `sha256:6622f53cb6aa6993dd71127a14dbf693c9ad5074401b229fd746476b52d0df91`
- `docs/SPEC/039-installation-lifecycle-contracts-successor.md` — `sha256:9fe5af8c621df3d59e371c59db93d4e2ecdd8a7d3c159a7c682248cef14755fd`
- `docs/PLAN/006-installation-lifecycle-contracts-successor.md` — `sha256:98df961c07ba71939a5172b13f93eec8b7263f024237740348c2128c7367d027`

These accepted boundaries remain governing. This proposal does not supersede
them or activate deferred installation-lifecycle work. It adds a narrowly
specified authorization edge rather than inferring repository authority from
the installation root. Provider-workspace publication authority is not borrowed;
the publisher operation is a separate core control-plane effect.

Historical context, not acceptance authority:

- `docs/ADR/033-architecture-decision-lifecycle-and-authority.md` — `sha256:d9ef754395f3e5f44ac05c8cdede3c595e320b5e267677aa11c4e3feb727581e`
- `docs/ADR/046-dynamic-package-command-registration-and-distribution.md` — `sha256:5e966889e790113e4ffe2049ab25b7aec16ea466de7f3e07c070dca144e67263`
- `docs/ADR/074-built-in-authority-model-v1.md` — `sha256:6b4153fdc6ddc10bddbb0dfdc1f75318ff3f1670e19065e5a91ccfdf19880ea2`
- `pkg/contracts/authority_model.go` — `sha256:99592818a5a6365f99f3598b0271355740bb36eac3c6036ccf8309954a178871`

ADR-033 is invoked by the accepted lifecycle convention; its own Draft status
is not silently changed. Explicit human acceptance must identify frozen proposal
bytes. No merge, test result, issue, or agent statement confers acceptance.

## Ownership and minimum authority delta

The trusted Praxis publisher handler executes as `publisher:praxis-first-party`,
bound to its exact enrolled generation and this installation. The authenticated
installation owner may authorize only the exact protected publication intent
through the existing canonical request/decision/delegation mechanism, after
this rule is accepted, implemented, qualified, and explicitly adopted.

Propose authority-model v4 solely because v1/v2/v3 have immutable closed rules.
V4 adds one profile, `GOALS_INITIAL_PUBLICATION`, using governed authority
`package.publish` and operation `publish-initial-goals`. It binds the exact
ActionIntent digest and all constraints in the companion specification. It
permits no arbitrary repository writes, wildcard namespace, other package, or
other destination. The successor digest must cover the new rule's semantics
and constraints; no successor digest is invented in this proposal.

V1/v2/v3 identities and validators retain their historical meanings. In
particular, old `package.publish` grants authorize only their original signing
operation. No grant is relabeled, widened, or automatically created by adoption.
Existing valid signing provenance remains evidence of the original operation;
Goals is not re-signed. Existing v3 package.deploy generations retain their exact
scope, expiry and validation under an explicit successor compatibility rule;
adoption does not recreate, renew or grant deployment authority.

One fresh exact authorization permits one publication execution and its bounded
recovery. Use existing ApprovalBinding where needed to bridge the exact decision
to effect admission, not as a second independent authority ceremony. Possession
of a generic approval object cannot substitute for a validated issuer and rule.

## Execution and evidence

Reuse ActionIntent, protected objects, authority records, commands/events,
EffectRecord, and existing effect coordination/reconciliation. New versioned
operation payload validation and a completion event are sufficient; do not
introduce a generalized publication intent or receipt protocol or parallel ledger.

Freeze a parentless descriptor commit and exact release operation before
approval. Execute only absence-checked ref/tag creation, draft release creation,
unchanged signed asset upload, read-back verification, and final publication.
Record each external dispatch before it occurs. Resolve ambiguous outcomes from
remote observations and the original authorized execution, never matching bytes
alone. Do not force, overwrite, or automatically delete conflicting state.

The local Goals acquisition entry path must resolve that completed authorized
execution and match the release and downloaded digests before passing the same
bytes to the existing package verifier. It creates no portable trust protocol.
Publication, acquisition verification, and package.deploy remain separate gates.

## Scope firewall

- `package.sign` renaming or reinterpretation of historical grants.
- Generalized `PublicationIntent`/`PublicationReceipt` protocols.
- Portable attestations or cross-installation trust.
- Additional packages, repositories, transports, or repository creation.
- General GitHub automation or CI/OIDC signing.
- Federation, transparency infrastructure, or generalized registries.
- General acquisition-policy redesign.
- Cross-installation revocation/compromise policy.
- Re-signing Goals or replacing valid deployment authority.

Issue #127 retains future design knowledge only. It is not a normative dependency,
accepted architecture, or addition to this qualification denominator. Revisit
triggers initiate scoped review, not automatic activation of its design. Nothing
in that parked work blocks Goals → Agent/Graph → weather → continuous → Japetella.

## Review and authorization boundaries

This artifact remains Proposed. Acceptance of frozen ADR/SPEC/PLAN bytes,
implementation authorization, schema migration if needed, model adoption, exact
publication authorization, and deployment authorization are distinct decisions.
This bundle performs none of them. Subsequent corrections require new reviewed
bytes/digests; preserve the frozen proposal and its acceptance identity.
