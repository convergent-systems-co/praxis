CREATE TABLE invocation_runtime_bindings (
    entry_point_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    package_id TEXT NOT NULL,
    contract_digest TEXT NOT NULL,
    runtime_id TEXT NOT NULL,
    runtime_version TEXT NOT NULL,
    runtime_digest TEXT NOT NULL,
    registered_at TEXT NOT NULL,
    PRIMARY KEY (entry_point_id, package_version, content_digest),
    FOREIGN KEY (entry_point_id, package_version, content_digest)
      REFERENCES invocation_registry(entry_point_id, package_version, content_digest)
      ON DELETE CASCADE
);

CREATE INDEX idx_invocation_runtime_bindings_package
ON invocation_runtime_bindings(package_id, content_digest);

UPDATE schema_meta SET value='12' WHERE key='schema_version';
