BEGIN;

-- Mark replaced content hashes as historical revisions without deleting evidence.
UPDATE portal.compliance_assets
SET metadata = COALESCE(metadata, '{}'::jsonb) || jsonb_build_object('superseded', TRUE)
WHERE notes LIKE '%Superseded by a new content revision%'
   OR notes LIKE '%Superseded by a new resource revision%';

INSERT INTO portal.schema_migrations(version, name)
VALUES (21, 'current_compliance_asset_revision')
ON CONFLICT (version) DO NOTHING;

COMMIT;
