# SPEC-030: First-Party Provider Worker Adapters

- Status: Accepted
- Governing ADR: ADR-067

## Contract

`ProviderCLIWorker` SHALL:

1. receive the exact selected `WorkerRequest` from Goal-drive;
2. invoke an explicit provider argv without a shell;
3. pass only the allowlisted non-secret environment;
4. propagate context cancellation and timeout to the provider process;
5. bound stdout/stderr and keep both outside durable control-plane records;
6. redact credential-shaped process errors;
7. reject malformed explicit environment entries and credential-shaped
   overrides; and
8. return no model-derived `EndHead`, checkpoint validity, completion, or
   `USER_DECISION_REQUIRED` claim.

The controller SHALL re-read the repository after a successful
`ProviderCLIWorker` process. It SHALL classify:

- unchanged clean HEAD as `NO_PROGRESS`;
- changed clean local HEAD as validated progress and `CONTINUE`;
- uncommitted provider changes as `BLOCKED` because no checkpoint is valid;
- provider failure/cancellation as `BLOCKED` with the concrete failure; and
- remote publication only through the existing controller-owned repository
  adapter.

The adapter SHALL not forward `*_API_KEY`, `*_TOKEN`, `*_SECRET`, password,
credential, cloud-provider, or equivalent credential variables. Provider
managed OAuth/session state may be read from the explicitly allowlisted home
and provider configuration locations, but its contents SHALL not enter
`WorkerRequest`, `WorkerResult`, ledger, Goal evidence, Git, or logs.

`COMPLETE` requires an authoritative completion predicate beyond provider
prose. `USER_DECISION_REQUIRED` requires a structured durable authority or
decision request; arbitrary model uncertainty SHALL not produce it.

## Qualification

Qualification SHALL cover sanitized environment behavior, secret
non-disclosure, strict protocol preservation, transcript separation, provider
failure, timeout/cancellation, malformed provider output, repository-derived
progress, dirty-worktree blocking, and provider profile registration. Real
provider execution additionally requires an accepted WorkPlan, valid Goal
authority, and available provider-managed authentication; those are not
fabricated by adapter tests.
