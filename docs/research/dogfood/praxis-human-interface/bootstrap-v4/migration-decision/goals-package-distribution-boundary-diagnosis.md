# Goals-package distribution boundary — diagnosis only, nothing deployed

Date: 2026-09-22. Diagnosis-only phase, per explicit authorization. No GitHub publication, no artifact substitution, no bypass of `package-deploy-intent-preview`, no source modification, no deployment performed.

**Classification: `IMPLEMENTATION_GAP`** (with one caveat noted in §9 — the architecture that would close it is documented only as a Draft ADR, not yet Accepted).

## 1. Evidence supporting the classification

Traced directly from source, not assumed:

- **The domain model already abstracts distribution.** `internal/distribution/distribution.go` defines `type Adapter interface { Discover, Info, Resolve, FetchArtifact, CheckUpdate }` and `PackageRef{ Source, Owner, Repo }` — `Source` is a first-class field, not a GitHub-specific concept.
- **But exactly one implementation exists anywhere in the codebase:** `internal/distribution/github_releases.go` (`GitHubReleases`). No second `Adapter` implementation exists (`grep`-confirmed: `ls internal/distribution/` shows only `distribution.go`, `github_releases.go`, `resolver.go` and their tests).
- **`PackageRef.Source` is never used to select an adapter.** `grep -rn "\.Source\b"` across `internal/distribution`, `internal/packagecatalog`, and the relevant `cmd/praxis` files finds it read in exactly one place (`resolver.go:125`, descriptive only) and never branched on to choose a transport. Every call site constructs `GitHubReleases{}` directly and hardcodes it — there is no adapter registry or factory keyed by `Source`.
- **`cmd/praxis/packages.go`'s `parseGitHubPackageRef`** is the only ref-parsing function in the CLI and hardcodes `Source: "github-releases"` — there is no parallel `parseLocalPackageRef` or equivalent.
- **The one candidate "local" mechanism does not actually provide a local transport.** ADR-095's `Execution.HoldsLocalLineage` / `goalspublication.RequiresLocalLineage` (`internal/goalspublication/acquisition.go`) exists specifically so the *publishing* installation can trust its own first-party release without re-verifying a third-party signature — but `CheckAcquisition` still hard-requires `release.Ref.Source == "github-releases"` (rejects anything else with `"wrong Goals publication locator"`). It changes *which trust check* applies to a GitHub-sourced release; it does not provide an alternative way to *fetch* one. This installation does not hold that lineage in any case — independently confirmed read-only: zero `goals-publication.completed` / `goals-publication-recovery.completed` events exist in this installation's `events`/`commands` tables.

## 2. Intended distribution architecture

`docs/ADR/025-catalog-trust-provenance-and-signing.md` (**Status: Draft**, 2026-09-13) is direct, unambiguous evidence of intent:

> "Distribution convenience must not imply execution trust. GitHub may be a practical catalog transport, but repository visibility, stars, ownership, or source location are not sufficient trust signals."
>
> "The catalog transport may initially be GitHub, but trust semantics must not depend on GitHub itself."
>
> "**The model also permits private catalogs, organization-approved registries, and purely local packages without changing core trust semantics.**"

This directly answers questions 1, 4, 5, 7, and 8 below: GitHub is explicitly documented as the *first* implemented transport, not a required one, and trust is meant to rest on the artifact's own provenance/integrity metadata (which this candidate already has in full: exact hashes, source manifest, two independent qualification reviews) — not on which transport carried the bytes.

## 3. Security/provenance properties involved

Per ADR-025's own stated trust model (integrity, provenance, capability risk, compatibility, evidence quality, local trust decision), a signature "or equivalent verifiable integrity mechanism" is required specifically **"when distributed beyond a trusted local source."** This exact Goals package candidate already carries stronger provenance than a bare signature would: its exact bytes are bound to a committed source commit (`25c7330`), a 125-file source manifest, and two independently-reviewed qualification chains (Astra Review #10 ACCEPTED, plus the underlying seven repairs/reviews). GitHub publication would add *transport-level* discoverability and a signature envelope, but would not add any integrity or provenance fact not already established more directly by this repository's own qualification evidence.

## 4. Answers to the specific questions posed

1. **Is GitHub Releases policy or implementation?** Implementation only, per ADR-025's own text ("may initially be GitHub… must not depend on GitHub itself").
2. **Does the domain model already abstract distribution such that a local/prequalified provider belongs behind the interface?** Yes — the `Adapter` interface and `PackageRef.Source` field are exactly the seam a local/prequalified adapter would implement; nothing about the interface assumes a network fetch.
3. **Is there already a supported mechanism for deploying exact locally-qualified bytes?** No. `HoldsLocalLineage`/`RequiresLocalLineage` (the closest candidate) still requires `Source == "github-releases"` and, independently, this installation doesn't hold that lineage regardless.
4. **Does requiring GitHub publication add a security/provenance property not already established?** No, per §3 — the qualification chain already exceeds what a signature alone would establish for this specific artifact.
5. **Would allowing local deployment bypass an important property currently established by GitHub discovery?** No property specific to *discovery via GitHub* was identified; GitHub Releases here functions as a transport and a signature carrier, both of which this candidate's own qualification evidence already satisfies through a different, arguably stronger, channel (full source-commit binding plus independent review, vs. a bare publisher-key signature).
6. **Is the Review-10-qualified package sufficient as a provenance-bound deployment input, or does the lifecycle explicitly require a distribution-origin identity in addition?** The *code* currently requires a GitHub-origin identity unconditionally (`CheckAcquisition`'s hard `Source` check; `parseGitHubPackageRef`'s hardcoding). The *documented architecture* (ADR-025) does not require this in principle. This is the gap.
7. **Did PRE-V4 qualification implicitly assume a later GitHub publication step?** No such assumption is stated anywhere in the bootstrap-v4 reports, reviews, or qualification records read across this entire multi-week effort; deployment was consistently listed as a separate, later, not-yet-authorized blocker without specifying *how* — this diagnosis is the first point anyone (implementer or reviewer) has had to resolve that "how."
8. **Historical evidence that local/offline/first-party deployment was deliberately deferred or prohibited?** None found. No ADR, SPEC, PLAN, or review in this repository states local deployment is disallowed; ADR-025 (Draft) states the opposite intent, and no Accepted ADR overrides it.
9. **Would publishing this exact package as a GitHub Release be the intended next transition, a legitimate optional choice, or a workaround?** Given ADR-025's explicit "must not depend on GitHub" language, treating GitHub publication as *mandatory* for this locally-qualified, already-doubly-reviewed candidate would be a workaround for a missing local capability, not the architecturally intended path — though it remains a legitimate *optional* distribution choice if Thomas independently wants this package externally published for unrelated reasons.

## 5. Consumer-v1 lens

A future consumer with "I have an exact qualified package artifact, how does it become deployable?" has **no good current answer** for a locally-built artifact: the only implemented answer is "publish it externally first," which is a real gap for exactly the bootstrap/first-party case this PRE-V4 work represents — a package built, qualified, and reviewed entirely inside the installation that will run it, with no need for third-party discovery. This matches ADR-025's own "purely local packages" language, which exists in the document precisely because this consumer need was anticipated, not because it's exotic.

## 6. Smallest correct next action

Not determined by this diagnosis, and not performed here (out of scope — "do not repair the distribution subsystem"). The smallest correct action would be a local/first-party `distribution.Adapter` implementation (reading a package from a local path or the installation's own `artifacts/package/` directory, using this repository's own qualification chain — source manifest, `qualification-results.json`, Astra Review #10 acceptance — as its provenance record in place of a GitHub signature) wired in behind the existing interface via `PackageRef.Source`. This is source modification and is explicitly not authorized by this diagnosis phase.

## 7. Source modification required?

Yes, to close the gap — but **not performed and not authorized here.**

## 8. Is GitHub publication actually warranted?

Not by architectural necessity (§4.9). It remains available as Thomas's independent choice if there's a reason to want this specific package externally distributed, but it should not be treated as *required* by the repository's own stated design.

## 9. Effect on the already-qualified PRE-V4 candidate

None. This is a distribution-mechanism finding, not a defect in the candidate itself — the candidate's identity, qualification, and Review #10 acceptance are all untouched and unaffected. It does mean the candidate cannot currently be *installed via the governed package lifecycle* until either a local adapter is built (an explicit future decision, requiring source modification and its own qualification) or the package is published externally (Thomas's independent choice, with the caveat in §4.9).

One caveat on the classification itself: `docs/ADR/025-catalog-trust-provenance-and-signing.md` is **Status: Draft**, not Accepted. The *intent* to support local packages is clearly documented, but not yet formally ratified the way ADR-095 (Accepted) is. Closing this gap therefore plausibly also warrants an explicit architectural decision/ratification step, not purely a coding task — hence this is reported as `IMPLEMENTATION_GAP` with that caveat rather than a clean-cut `EXISTING_PATH_FOUND`.

## 10. Package-manager delegation status

Not allowed to expire deliberately or rushed to use — per instruction, its state is simply recorded: `package.deploy` delegated to `package-manager:praxis`, expiry `2026-09-22T17:03:51Z`, still valid as of this diagnosis (`2026-09-22T15:17:38Z`, independently re-checked). No action was taken to preserve or extend it artificially. If it expires before any future deployment decision, a fresh delegation ceremony (already fully qualified and repeatable, per the prior `package-deploy-preview/-proposal/-review/-request` + `authority delegate` sequence) is the normal, unremarkable way to obtain it again.

---

**No GitHub publication, no artifact substitution, no bypass, no source modification, no deployment was performed.** Governance/FAA state, installed core, and package artifact identities are all unchanged from the prior evidence freeze.
