CREATE TABLE package_activation_receipts (
    activation_id TEXT PRIMARY KEY,
    package_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    verification_id TEXT NOT NULL,
    verification_json BLOB NOT NULL,
    manifest_bytes BLOB NOT NULL,
    signature_json BLOB NOT NULL,
    activation_intent_json BLOB NOT NULL,
    activation_intent_digest TEXT NOT NULL,
    approval_id TEXT NOT NULL,
    authority_id TEXT NOT NULL,
    authority_kind TEXT NOT NULL,
    activated_at TEXT NOT NULL,
    UNIQUE(verification_id, approval_id),
    FOREIGN KEY(package_id, package_version, content_digest)
      REFERENCES installed_packages(package_id, package_version, content_digest),
    FOREIGN KEY(approval_id) REFERENCES approvals(approval_id)
);

CREATE INDEX idx_package_activation_receipts_package
ON package_activation_receipts(package_id, activated_at);

UPDATE schema_meta SET value='6' WHERE key='schema_version';
