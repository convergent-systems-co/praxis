CREATE TABLE plugin_supervisor_snapshots (
    instance_id TEXT PRIMARY KEY,
    snapshot_json BLOB NOT NULL,
    updated_at TEXT NOT NULL
);

UPDATE schema_meta SET value='10' WHERE key='schema_version';
