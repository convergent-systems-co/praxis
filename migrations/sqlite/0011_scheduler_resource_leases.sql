CREATE TABLE scheduler_resources (
    resource_key TEXT PRIMARY KEY,
    capacity INTEGER NOT NULL CHECK(capacity > 0),
    exclusive INTEGER NOT NULL DEFAULT 0 CHECK(exclusive IN (0,1))
);

CREATE TABLE scheduler_resource_leases (
    lease_id TEXT PRIMARY KEY,
    slice_id TEXT NOT NULL,
    attempt_id TEXT NOT NULL,
    resource_key TEXT NOT NULL,
    capacity INTEGER NOT NULL CHECK(capacity > 0),
    acquired_at TEXT NOT NULL,
    expires_at TEXT,
    released_at TEXT,
    FOREIGN KEY(resource_key) REFERENCES scheduler_resources(resource_key)
);

CREATE INDEX scheduler_resource_leases_active
ON scheduler_resource_leases(resource_key, released_at, expires_at);

UPDATE schema_meta SET value='11' WHERE key='schema_version';
