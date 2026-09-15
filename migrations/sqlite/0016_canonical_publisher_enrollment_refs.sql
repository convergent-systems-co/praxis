-- Canonical publisher enrollment approvals are encrypted governance records,
-- not rows in the generic one-use approval table. The existing column is
-- retained as the immutable approval reference, but its foreign key is
-- removed so the repository-owned transaction can bind the canonical record
-- and its digest without synthesizing a second approval.
PRAGMA foreign_keys=OFF;

CREATE TABLE publisher_generations_v2 (
    publisher_generation_digest TEXT PRIMARY KEY,
    principal_id TEXT NOT NULL,
    principal_kind TEXT NOT NULL,
    generation TEXT NOT NULL UNIQUE,
    record_json BLOB NOT NULL,
    state TEXT NOT NULL CHECK(state IN ('active','revoked')),
    enrollment_approval_id TEXT NOT NULL,
    enrolled_at TEXT NOT NULL
);

INSERT INTO publisher_generations_v2(publisher_generation_digest,principal_id,principal_kind,generation,record_json,state,enrollment_approval_id,enrolled_at)
SELECT publisher_generation_digest,principal_id,principal_kind,generation,record_json,state,enrollment_approval_id,enrolled_at
FROM publisher_generations;

DROP TABLE publisher_generations;
ALTER TABLE publisher_generations_v2 RENAME TO publisher_generations;

CREATE INDEX idx_publisher_generations_principal
ON publisher_generations(principal_id, principal_kind, generation);

PRAGMA foreign_keys=ON;
UPDATE schema_meta SET value='16' WHERE key='schema_version';
