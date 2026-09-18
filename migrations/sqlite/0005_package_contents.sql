UPDATE schema_meta SET value='5' WHERE key='schema_version';

CREATE TABLE package_contents (
    package_id TEXT NOT NULL,
    package_version TEXT NOT NULL,
    content_digest TEXT NOT NULL,
    kind TEXT NOT NULL,
    content_id TEXT NOT NULL,
    content_version TEXT NOT NULL,
    artifact_digest TEXT NOT NULL,
    artifact_ref TEXT NOT NULL,
    compatibility TEXT,
    active INTEGER NOT NULL CHECK(active IN (0,1)),
    registered_at TEXT NOT NULL,
    PRIMARY KEY(package_id, package_version, content_digest, kind, content_id, content_version),
    FOREIGN KEY(package_id, package_version, content_digest)
      REFERENCES installed_packages(package_id, package_version, content_digest)
      ON DELETE CASCADE
);

CREATE UNIQUE INDEX idx_package_contents_active_identity
ON package_contents(kind, content_id, content_version)
WHERE active = 1;

CREATE INDEX idx_package_contents_package
ON package_contents(package_id, active, kind);
