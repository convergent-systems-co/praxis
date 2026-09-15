# SPEC-033: Turn-Owned Provider Workspaces

- Status: Accepted
- Governing ADR: ADR-070

## Contract

For a repository-backed bounded turn, the controller SHALL:

1. require a clean authoritative checkout at the exact selected start HEAD;
2. create a unique managed Git worktree from that HEAD;
3. persist a validated immutable workspace record before provider execution;
4. bind the record to the exact Goal/version, accepted WorkPlan, child,
   invocation, turn, provider, repository, path, and start HEAD;
5. pass only the managed workspace path to the provider;
6. recover the record and inspect the same workspace after interruption or
   provider failure;
7. classify dirty workspace state as recoverable but not checkpoint progress;
8. require a clean provider-created commit before checkpoint validation;
9. validate ancestry, repository identity, bounded scope, evidence, and remote
   publication independently of provider claims; and
10. clean a workspace only after a durable terminal state and a clean recovery.

`ProviderWorkspaceRecord` lifecycle transitions are immutable encrypted
snapshots. A record SHALL include workspace identity/path, repository, Goal and
WorkPlan bindings, child, invocation/turn/provider identity, start HEAD,
optional end HEAD, lifecycle state, and creation time. A specifically
authorized pre-isolation migration MAY additionally bind a source label and
cryptographic migration-input digest; those fields describe admitted input,
not original-turn provenance or checkpoint validity. Missing, conflicting,
stale, or ambiguous records fail closed.

The managed path SHALL remain below the configured workspace root. Recovery
SHALL verify that it is the recorded Git worktree for the recorded repository,
not merely a directory with a plausible name. Cleanup SHALL never force-remove
a dirty workspace or a workspace in a non-terminal state.

The authoritative checkout SHALL remain clean throughout provider execution.
An unrelated dirty authoritative checkout blocks workspace creation. A dirty
workspace with valid durable ownership may be recovered without treating its
contents as validated progress. Provider push, protected-ref rewrite,
symlink/path escape, secrets, generated files, and out-of-scope changes remain
checkpoint validation failures.

The current pre-isolation dirty checkout cannot be assigned this provenance
after the fact. It remains unchanged pending explicit migration/reconciliation
authority.

## Qualification

Qualification SHALL cover clean isolation, dirty provider recovery, terminal
cleanup, authoritative-dirty rejection, path escape, exact start-HEAD binding,
workspace identity, and immutable lifecycle records. Future integration SHALL
cover provider commits, out-of-scope changes, push/ref rewrite attempts,
stale workspaces, duplicate recovery, crash before/after commit/publication,
and concurrent workspace non-contamination.
