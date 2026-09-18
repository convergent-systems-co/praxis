CREATE TABLE authority_model_active (
    singleton_id TEXT PRIMARY KEY,
    object_namespace TEXT NOT NULL,
    object_id TEXT NOT NULL,
    object_version TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

INSERT INTO authority_model_active(singleton_id, object_namespace, object_id, object_version, updated_at)
VALUES ('authority-model-current', 'publisher_governance', 'active-authority-model', '1', CURRENT_TIMESTAMP);

UPDATE schema_meta SET value='18' WHERE key='schema_version';
