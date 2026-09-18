CREATE TABLE IF NOT EXISTS exact_dispatch_grants (
    intent_digest TEXT PRIMARY KEY,
    intent_json BLOB NOT NULL,
    approval_id TEXT NOT NULL UNIQUE,
    lease_id TEXT NOT NULL UNIQUE,
    approver_id TEXT NOT NULL,
    approver_kind TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    FOREIGN KEY(approval_id) REFERENCES approvals(approval_id),
    FOREIGN KEY(lease_id) REFERENCES capability_leases(lease_id)
);
