CREATE TABLE secure_blobs (
    namespace TEXT NOT NULL,
    object_id TEXT NOT NULL,
    object_version TEXT NOT NULL,
    object_digest TEXT NOT NULL,
    sensitivity TEXT NOT NULL,
    crypto_profile TEXT NOT NULL,
    envelope_json BLOB NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT,
    PRIMARY KEY(namespace, object_id, object_version)
);

CREATE INDEX idx_secure_blobs_lookup
    ON secure_blobs(namespace, object_id, created_at DESC);

UPDATE schema_meta SET value='3' WHERE key='schema_version';
