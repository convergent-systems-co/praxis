ALTER TABLE package_activation_receipts
ADD COLUMN artifact_bytes BLOB NOT NULL DEFAULT X'';

ALTER TABLE package_contents
ADD COLUMN artifact_bytes BLOB NOT NULL DEFAULT X'';

UPDATE schema_meta SET value='8' WHERE key='schema_version';
