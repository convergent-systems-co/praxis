# Goals-package deployment — authority prerequisite found, deployment not executed

Date: 2026-09-22. Inspection-only phase, per explicit authorization limited to: reconstructing/verifying the qualified package candidate, verifying deployment prerequisites and current authority, executing deployment, verifying identity/compatibility, freezing evidence. **Deployment was not executed** — inspection of the repository's own governed lifecycle found that it requires a new, consequential, interactively-confirmed authority ceremony this phase's authorization explicitly reserved for Thomas.

## Baseline re-confirmed (unchanged since atomic replacement evidence, `d46c541`)

```
installed core:  42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821  (matches)
governance:      relation=consistent, anchor_seq=store_seq=1,
                  head=sha256:9d4c1bcc86507808f6dbda6c599211ddbc1ce44f2a012dc7044709bf1b8877ba  (matches)
Goals plugin:     08514f2078c0e4a7a9093e41b4a78ec72069f7053a6e5bf670ff426ec1888829  (matches Review #10)
package archive:  06e0ebaea3c0ad549d7efdf37f9894283360b36b9dfa4ccf8fa9e0a45d96cc62  (matches Review #10)
package manifest: e8894b79d8e7589cff37e98f93e31e22adb7fba885a02c19ac46156e534111d2  (matches Review #10)
```

All four full canonical digests were independently recomputed (`shasum -a 256`) against the exact files under `bootstrap-v4/artifacts/`, not read from any cached/summarized value, and match the Review-10-qualified identities exactly.

## What the repository's governed deployment lifecycle actually requires

Traced directly (`cmd/praxis/package_manager_authority.go`, `pkg/contracts/authority_model.go`, `cmd/praxis/cli_help.go`) rather than assumed from the earlier handoff summary:

`praxis install <owner/repo[@tag]>` is the mechanism that installs a package, but it requires `PRAXIS_PACKAGE_APPROVAL_ID` — a **derived package-deployment approval**, which only exists once `authority package-deploy-approve` has run. That, in turn, is the last step of a full four-stage ceremony:

1. `authority package-deploy-preview --expires-at <RFC3339>` — a read-only preview of a **new** delegation proposal: `GovernedPackageDeploy` (`"package.deploy"`) capability, `DelegationProfilePackageDeploy` (`"PACKAGE_DEPLOY"`) profile, delegated to the `PackageManagerPrincipal`, scoped from and rooted at the **current installation root** (which the just-completed re-anchor makes resolvable again — `currentInstallationRoot` succeeds now, where it would have failed before migration). **`--expires-at` is a required, owner-chosen expiry with no default** — this is a policy decision, not a technical parameter I can safely infer.
2. `authority package-deploy-proposal --preview-file <file>` — persists that exact system-produced preview as a durable proposal.
3. `authority package-deploy-review --proposal-digest <digest>` — **"Owner confirmation is required."**
4. `authority package-deploy-approve --request <digest>` — **"Owner confirmation is required; the exact intent and evidence are displayed and revalidated."** This step derives the actual package-installer binding/approval.

This is not a mechanical prerequisite check against already-existing authority — it is the **creation of a new delegated authority relationship** (package-manager deployment capability) that does not currently exist for this installation (the governed re-anchor deliberately did not, and should not, manufacture it — see `post-migration-evidence.md` §10). It requires two separate interactive owner confirmations and one owner-chosen policy value (the expiry), exactly the class of "additional consequential HUMAN decision" this phase's authorization named and told me to stop at and present rather than perform.

## Decision presented to Thomas

Before Goals-package deployment can proceed at all, **you** need to:

1. **Decide the package-manager deployment authority's expiry** (`--expires-at`) — how long should this delegated capability remain valid? (For reference: nothing in this installation currently constrains this; it is a fresh choice, not a re-derivation of any prior value — the pre-migration installation's delegated generations are not current and this document does not read them for guidance, consistent with not inferring old policy into new grants.)
2. **Perform the `package-deploy-review` interactive confirmation** yourself, after reading the system-produced proposal.
3. **Perform the `package-deploy-approve` interactive confirmation** yourself, after reading the displayed intent/evidence.

Only after that four-stage ceremony completes does a `PRAXIS_PACKAGE_APPROVAL_ID` exist for `praxis install` to consume for this Goals package.

**I did not run `package-deploy-preview` even to generate a preview file**, since doing so requires choosing an expiry value on your behalf, which is exactly the inferred policy decision the authorization forbade. If you want, tell me the expiry you'd like and I can run the read-only preview step for you to review — but the two owner-confirmation steps still need to be done by you interactively, the same way the re-anchor ceremony was.

## Disposition

`GOALS_PACKAGE_DEPLOYMENT_ACCEPTED` was **not** reached. No deployment was attempted. No authority was granted, inferred, fabricated, or backfilled. Nothing was installed via `praxis install`. The installed core, governance state, and package artifact identities are all unchanged and re-confirmed above.
