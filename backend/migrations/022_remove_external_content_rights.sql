BEGIN;

-- Keep ordinary image-upload ownership/access metadata, but remove the old
-- table's dependency on the external-content copyright ledger.
CREATE TABLE IF NOT EXISTS portal.uploaded_images (
  image_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  filename TEXT NOT NULL UNIQUE CHECK (filename ~ '^[0-9a-f-]{36}\.(png|jpg|gif|webp|avif)$'),
  uploader_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE RESTRICT,
  mime_type TEXT NOT NULL CHECK (mime_type IN ('image/png','image/jpeg','image/gif','image/webp','image/avif')),
  sha256 CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes BETWEEN 1 AND 8388608),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO portal.uploaded_images(filename,uploader_id,mime_type,sha256,file_size_bytes,created_at)
SELECT filename,uploader_id,mime_type,sha256,file_size_bytes,created_at
FROM portal.uploaded_image_assets
ON CONFLICT(filename) DO NOTHING;
CREATE INDEX IF NOT EXISTS uploaded_images_uploader_idx ON portal.uploaded_images(uploader_id,created_at DESC);
CREATE INDEX IF NOT EXISTS uploaded_images_sha_idx ON portal.uploaded_images(sha256);
ALTER TABLE portal.uploaded_images OWNER TO personal_portal_app;
REVOKE ALL ON portal.uploaded_images FROM PUBLIC;
DROP TABLE IF EXISTS portal.uploaded_image_assets;

-- Delete external user-content rights records only. Software/dependency and
-- portal-policy assets remain available in /account/compliance.
WITH external_assets AS (
  SELECT asset_id FROM portal.compliance_assets
  WHERE asset_path LIKE 'db://portal.portal_content/%'
     OR asset_path LIKE 'db://portal.teaching_resources/%'
     OR asset_path LIKE '/uploads/%'
     OR asset_type IN ('article','education_resource','external_link')
     OR metadata->>'source' IN ('content_api_publish_gate','resource_api_publish_gate')
), removed AS (
  DELETE FROM portal.compliance_assets a USING external_assets e
  WHERE a.asset_id=e.asset_id
  RETURNING a.asset_id
)
DELETE FROM portal.compliance_audit_log l
WHERE l.object_id IN (SELECT asset_id FROM external_assets)
   OR l.action IN ('content.permanently_deleted','resource.permanently_deleted','publisher.rights_submitted')
   OR l.object_type='copyright_complaint';

DELETE FROM portal.compliance_scan_runs WHERE scan_type='database_content';
DROP TABLE IF EXISTS portal.compliance_copyright_complaints;
DELETE FROM portal.compliance_legal_documents WHERE document_type IN ('copyright_notice','copyright_complaints') OR slug IN ('copyright-notice','copyright-complaints');
ALTER TABLE portal.compliance_assets DROP CONSTRAINT IF EXISTS compliance_assets_asset_type_check;
ALTER TABLE portal.compliance_assets ADD CONSTRAINT compliance_assets_asset_type_check CHECK (asset_type IN ('image','icon','font','audio','video','software_dependency','source_file','policy_document','other'));
ALTER TABLE portal.compliance_assets DROP CONSTRAINT IF EXISTS compliance_assets_discovered_by_check;
ALTER TABLE portal.compliance_assets ADD CONSTRAINT compliance_assets_discovered_by_check CHECK (discovered_by IN ('manual','filesystem_scan','package_lock_scan','imported'));
UPDATE portal.compliance_asset_reviews SET review_type='license' WHERE review_type='copyright';
ALTER TABLE portal.compliance_asset_reviews DROP CONSTRAINT IF EXISTS compliance_asset_reviews_review_type_check;
ALTER TABLE portal.compliance_asset_reviews ADD CONSTRAINT compliance_asset_reviews_review_type_check CHECK (review_type IN ('license','privacy','content','distribution','legal_basis','recheck'));
ALTER TABLE portal.compliance_asset_reviews ALTER COLUMN review_type SET DEFAULT 'license';
ALTER TABLE portal.compliance_scan_runs DROP CONSTRAINT IF EXISTS compliance_scan_runs_scan_type_check;
ALTER TABLE portal.compliance_scan_runs ADD CONSTRAINT compliance_scan_runs_scan_type_check CHECK (scan_type IN ('filesystem','dependencies','license_texts','full_audit','manual'));
ALTER TABLE portal.compliance_legal_documents DROP CONSTRAINT IF EXISTS compliance_legal_documents_document_type_check;
ALTER TABLE portal.compliance_legal_documents ADD CONSTRAINT compliance_legal_documents_document_type_check CHECK (document_type IN ('privacy_policy','terms_of_use','third_party_notices','other'));

INSERT INTO portal.schema_migrations(version,name)
VALUES (22,'remove_external_content_rights')
ON CONFLICT(version) DO NOTHING;
COMMIT;
