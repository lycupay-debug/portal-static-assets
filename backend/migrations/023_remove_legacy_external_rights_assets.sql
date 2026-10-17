BEGIN;

-- The earlier inventory also recorded uploaded user files and obsolete bundles
-- using filesystem-relative paths. Remove only their compliance metadata; do not
-- delete the uploaded files or user content themselves.
CREATE TEMP TABLE external_rights_assets_to_remove ON COMMIT DROP AS
SELECT asset_id
FROM portal.compliance_assets
WHERE asset_path LIKE 'uploads/%'
   OR asset_path LIKE 'resources/%'
   OR asset_path LIKE '/uploads/%'
   OR asset_path IN (
     'frontend/src/components/CopyrightComplaintForm.vue',
     'frontend/src/components/PublisherRightsForm.vue',
     'frontend/src/components/ComplianceResourcePreview.vue',
     'frontend/src/utils/copyrightReview.ts',
     'frontend/test/copyright-review.test.mjs'
   )
   OR (
     asset_path LIKE 'deployed/current/assets/%'
     AND (
       asset_name ILIKE '%CopyrightComplaintForm%'
       OR asset_name ILIKE '%PublisherRightsForm%'
       OR asset_name ILIKE '%ComplianceResourcePreview%'
     )
   );

DELETE FROM portal.compliance_audit_log
WHERE object_id IN (SELECT asset_id FROM external_rights_assets_to_remove);

DELETE FROM portal.compliance_assets a
USING external_rights_assets_to_remove stale
WHERE a.asset_id = stale.asset_id;

INSERT INTO portal.schema_migrations(version,name)
VALUES (23,'remove_legacy_external_rights_assets')
ON CONFLICT(version) DO NOTHING;
COMMIT;
