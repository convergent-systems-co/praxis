# Troubleshooting and Operations

## Diagnostic commands

```bash
praxis version
praxis doctor
praxis executors
praxis status RUN_ID --db "$PRAXIS_DB"
python -m praxis_dashboard --graph GRAPH.json --run-dir RUN_DIR --replay-only
```

Keep the command output, exact binary version, database path (not credentials), run directory, package digest, and provider version when reporting an incident.

## Failed initialization or missing database

Run `praxis doctor` with `PRAXIS_DB` unset to verify the executable. Then choose a writable path and run it again. An existing path that cannot be opened is not repaired by deleting it; copy it first and inspect permissions, filesystem availability, and migration errors.

## Provider unavailable or authentication failure

Run `praxis executors` and `praxis doctor`. A missing provider executable, stopped Ollama/MLX service, failed subscription login, or ambiguous Copilot authentication remains unavailable/degraded. Install or authenticate the provider using its own documented command, then repeat the read-only checks. Do not paste credentials into issue reports.

## Invalid graph, package, or version

Use `praxis doctor --graph GRAPH.json` for document validation. For a package, preserve the signed manifest and artifact and verify the source tag, content digest, contract version, dependency lock, and trusted publisher key. Do not edit a package in place to make validation pass. Publish a new version instead.

## Interrupted run or restart

Preserve the entire Python run directory, including `run-state.json`, `events/events.jsonl`, and any evidence files. Replay it with the dashboard. For Go runs, preserve the SQLite database and use `praxis status`; resume requires the exact persisted wait reference and actor identity. A stale provider session or lease is expected to be rejected after restart.

## Corrupted or rejected state

Stop writes. Make a byte-for-byte copy of the database/run directory, record the error, and work only on the copy. Rejected schema, digest, sequence, event-gap, or authority records are fail-closed safety behavior. Do not hand-edit authoritative state or delete events.

## Failed upgrade or rollback

Keep the prior binary and database. Verify the new binary with `praxis version` and `praxis doctor` before package mutation. Package rollback is an explicit, approval-bound operation; it restores package registrations, not consumed approvals, leases, plugin sessions, or agent identities.

## Backup and restore

Back up the stopped `PRAXIS_DB` file together with release metadata and trusted publisher configuration. Back up Python run directories as complete directories. Restore into a separate path first, run read-only doctor/status/replay checks, and only then point a new process at the restored state.

## Uninstall

Remove the binary or Python virtual environment separately from durable state. To remove a package, use the governed `praxis uninstall PACKAGE_ID` flow; it preserves historical receipts and replay lineage. Do not use `rm -rf` on `$HOME/.praxis` unless you have separately backed up and explicitly intend to remove all local state.
