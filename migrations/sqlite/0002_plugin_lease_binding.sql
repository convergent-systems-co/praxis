ALTER TABLE capability_leases ADD COLUMN bound_instance_id TEXT;
ALTER TABLE capability_leases ADD COLUMN bound_session_id TEXT;

CREATE INDEX IF NOT EXISTS idx_capability_leases_plugin_binding
    ON capability_leases(bound_instance_id, bound_session_id)
    WHERE bound_instance_id IS NOT NULL AND bound_session_id IS NOT NULL;

UPDATE schema_meta SET value='2' WHERE key='schema_version';
