CREATE TABLE package_rollback_receipts (
    rollback_id TEXT PRIMARY KEY,
    root_package_id TEXT NOT NULL,
    request_json BLOB NOT NULL,
    intent_digest TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    authority_id TEXT NOT NULL,
    authority_kind TEXT NOT NULL,
    rolled_back_at TEXT NOT NULL,
    UNIQUE(intent_digest, approval_id),
    FOREIGN KEY(approval_id) REFERENCES approvals(approval_id)
);

CREATE INDEX idx_package_rollback_receipts_root
ON package_rollback_receipts(root_package_id, rolled_back_at);

UPDATE schema_meta SET value='9' WHERE key='schema_version';
