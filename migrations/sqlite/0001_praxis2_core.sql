PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS schema_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO schema_meta(key, value) VALUES ('schema_version', '1');

CREATE TABLE IF NOT EXISTS commands (
    command_id TEXT PRIMARY KEY,
    command_type TEXT NOT NULL,
    command_version TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    actor_kind TEXT NOT NULL,
    scope TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    causation_id TEXT,
    payload BLOB NOT NULL,
    status TEXT NOT NULL,
    result BLOB,
    created_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    event_id TEXT NOT NULL UNIQUE,
    aggregate_id TEXT NOT NULL,
    aggregate_type TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    event_version TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    actor_kind TEXT NOT NULL,
    command_id TEXT NOT NULL,
    correlation_id TEXT NOT NULL,
    causation_id TEXT,
    provenance_json BLOB,
    trust_class TEXT,
    payload BLOB NOT NULL,
    created_at TEXT NOT NULL,
    integrity_json BLOB,
    UNIQUE(aggregate_id, aggregate_version),
    FOREIGN KEY(command_id) REFERENCES commands(command_id)
);

CREATE INDEX IF NOT EXISTS idx_events_aggregate ON events(aggregate_id, aggregate_version);
CREATE INDEX IF NOT EXISTS idx_events_correlation ON events(correlation_id, sequence);

CREATE TABLE IF NOT EXISTS capability_leases (
    lease_id TEXT PRIMARY KEY,
    principal_id TEXT NOT NULL,
    principal_kind TEXT NOT NULL,
    capability TEXT NOT NULL,
    operations_json BLOB NOT NULL,
    scope TEXT NOT NULL,
    constraints_json BLOB,
    required_enforcement_json BLOB,
    issued_at TEXT NOT NULL,
    expires_at TEXT,
    revoked_at TEXT,
    remaining_uses INTEGER,
    version INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_leases_principal ON capability_leases(principal_id, capability);

CREATE TABLE IF NOT EXISTS approvals (
    approval_id TEXT PRIMARY KEY,
    approver_id TEXT NOT NULL,
    approver_kind TEXT NOT NULL,
    intent_digest TEXT,
    policy_ref TEXT,
    issued_at TEXT NOT NULL,
    expires_at TEXT,
    revoked_at TEXT,
    remaining_uses INTEGER NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    CHECK ((intent_digest IS NOT NULL AND policy_ref IS NULL) OR (intent_digest IS NULL AND policy_ref IS NOT NULL)),
    CHECK (remaining_uses >= 0)
);

CREATE TABLE IF NOT EXISTS effects (
    effect_id TEXT PRIMARY KEY,
    command_id TEXT NOT NULL,
    action_intent_digest TEXT NOT NULL,
    target_adapter TEXT NOT NULL,
    target_principal TEXT,
    capability_lease_id TEXT,
    approval_id TEXT,
    idempotency_key TEXT,
    preconditions_json BLOB,
    crypto_profile TEXT,
    state TEXT NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    request_payload BLOB NOT NULL,
    observed_result BLOB,
    reconciliation_evidence BLOB,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(command_id) REFERENCES commands(command_id),
    FOREIGN KEY(capability_lease_id) REFERENCES capability_leases(lease_id),
    FOREIGN KEY(approval_id) REFERENCES approvals(approval_id)
);

CREATE INDEX IF NOT EXISTS idx_effects_state ON effects(state, updated_at);

CREATE TABLE IF NOT EXISTS aggregate_versions (
    aggregate_id TEXT PRIMARY KEY,
    aggregate_type TEXT NOT NULL,
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS projection_checkpoints (
    projection_name TEXT PRIMARY KEY,
    projection_version TEXT NOT NULL,
    consistency_class TEXT NOT NULL,
    last_sequence INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS crypto_key_refs (
    key_id TEXT NOT NULL,
    key_version INTEGER NOT NULL,
    principal_id TEXT NOT NULL,
    purpose TEXT NOT NULL,
    suite_family TEXT NOT NULL,
    provider_ref TEXT NOT NULL,
    lifecycle_state TEXT NOT NULL,
    exportable INTEGER NOT NULL DEFAULT 0,
    properties_json BLOB,
    created_at TEXT NOT NULL,
    not_before TEXT,
    expires_at TEXT,
    PRIMARY KEY(key_id, key_version)
);
