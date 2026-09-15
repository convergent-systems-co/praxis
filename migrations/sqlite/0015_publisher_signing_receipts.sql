CREATE TABLE publisher_signing_receipts (
    provenance_digest TEXT PRIMARY KEY,
    publisher_generation_digest TEXT NOT NULL,
    package_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    envelope_json BLOB NOT NULL,
    provenance_json BLOB NOT NULL,
    signed_at TEXT NOT NULL,
    FOREIGN KEY(publisher_generation_digest) REFERENCES publisher_generations(publisher_generation_digest)
);

CREATE INDEX idx_publisher_signing_receipts_package
ON publisher_signing_receipts(package_id, package_version, signed_at);

UPDATE schema_meta SET value='15' WHERE key='schema_version';
