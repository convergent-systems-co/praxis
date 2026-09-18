ALTER TABLE approvals ADD COLUMN authority_request_id TEXT;
ALTER TABLE approvals ADD COLUMN authority_request_version TEXT;
ALTER TABLE approvals ADD COLUMN authority_request_digest TEXT;
ALTER TABLE approvals ADD COLUMN authority_decision_ref TEXT;
ALTER TABLE approvals ADD COLUMN authority_decision_version TEXT;
ALTER TABLE approvals ADD COLUMN authority_decision_digest TEXT;
ALTER TABLE approvals ADD COLUMN authority_generation_ref TEXT;
ALTER TABLE approvals ADD COLUMN authority_generation_version TEXT;
ALTER TABLE approvals ADD COLUMN authority_generation_digest TEXT;
ALTER TABLE approvals ADD COLUMN authority_model_version TEXT;
ALTER TABLE approvals ADD COLUMN authority_model_digest TEXT;
ALTER TABLE approvals ADD COLUMN installation_digest TEXT;
ALTER TABLE approvals ADD COLUMN closure_digest TEXT;
ALTER TABLE approvals ADD COLUMN decision_authority_ref TEXT;
ALTER TABLE approvals ADD COLUMN decision_authority_version TEXT;
ALTER TABLE approvals ADD COLUMN decision_authority_generation_digest TEXT;
ALTER TABLE approvals ADD COLUMN operational_authority_ref TEXT;
ALTER TABLE approvals ADD COLUMN operational_authority_version TEXT;
ALTER TABLE approvals ADD COLUMN operational_authority_generation_digest TEXT;
ALTER TABLE approvals ADD COLUMN verification_evidence_digest TEXT;

CREATE TABLE package_verification_evidence (
    evidence_id TEXT PRIMARY KEY,
    installation_digest TEXT NOT NULL,
    closure_digest TEXT NOT NULL,
    evidence_json BLOB NOT NULL,
    evidence_digest TEXT NOT NULL UNIQUE,
    verified_at TEXT NOT NULL
);

CREATE INDEX idx_package_verification_evidence_installation
ON package_verification_evidence(installation_digest, verified_at);

UPDATE schema_meta SET value='17' WHERE key='schema_version';
