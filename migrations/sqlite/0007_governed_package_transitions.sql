CREATE TABLE package_transition_receipts (
    transition_id TEXT PRIMARY KEY,
    package_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    operation TEXT NOT NULL,
    intent_json BLOB NOT NULL,
    intent_digest TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    authority_id TEXT NOT NULL,
    authority_kind TEXT NOT NULL,
    transitioned_at TEXT NOT NULL,
    UNIQUE(intent_digest, approval_id),
    FOREIGN KEY(approval_id) REFERENCES approvals(approval_id)
);

CREATE INDEX idx_package_transition_receipts_package
ON package_transition_receipts(package_id, transitioned_at);

UPDATE schema_meta SET value='7' WHERE key='schema_version';
