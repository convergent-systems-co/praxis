CREATE TABLE installed_packages (
    package_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    state TEXT NOT NULL,
    source_kind TEXT NOT NULL,
    source_ref TEXT NOT NULL,
    manifest_json BLOB NOT NULL,
    installed_at TEXT NOT NULL,
    activated_at TEXT,
    PRIMARY KEY(package_id, package_version, content_digest)
);

CREATE UNIQUE INDEX idx_installed_packages_active
ON installed_packages(package_id)
WHERE state = 'active';

CREATE TABLE invocation_registry (
    entry_point_id TEXT NOT NULL,
    package_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    graph_id TEXT NOT NULL,
    graph_version TEXT NOT NULL,
    contract_json BLOB NOT NULL,
    contract_digest TEXT NOT NULL,
    active INTEGER NOT NULL CHECK(active IN (0,1)),
    registered_at TEXT NOT NULL,
    PRIMARY KEY(entry_point_id, package_version, content_digest),
    FOREIGN KEY(package_id, package_version, content_digest)
      REFERENCES installed_packages(package_id, package_version, content_digest)
      ON DELETE CASCADE
);

CREATE TABLE invocation_aliases (
    alias TEXT PRIMARY KEY,
    entry_point_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    FOREIGN KEY(entry_point_id, package_version, content_digest)
      REFERENCES invocation_registry(entry_point_id, package_version, content_digest)
      ON DELETE CASCADE
);

CREATE INDEX idx_invocation_registry_package ON invocation_registry(package_id, active);
