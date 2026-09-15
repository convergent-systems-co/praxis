CREATE TABLE publisher_generations (
    publisher_generation_digest TEXT PRIMARY KEY,
    principal_id TEXT NOT NULL,
    principal_kind TEXT NOT NULL,
    generation TEXT NOT NULL UNIQUE,
    record_json BLOB NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('active','revoked')),
    enrollment_approval_id TEXT NOT NULL,
    enrolled_at TEXT NOT NULL,
    FOREIGN KEY(enrollment_approval_id) REFERENCES approvals(approval_id)
);

CREATE INDEX idx_publisher_generations_principal
ON publisher_generations(principal_id, principal_kind, generation);

UPDATE schema_meta SET value='14' WHERE key='schema_version';
