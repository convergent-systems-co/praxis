ALTER TABLE invocation_runtime_bindings
    ADD COLUMN executable_binding_json BLOB;

UPDATE schema_meta SET value='13' WHERE key='schema_version';
