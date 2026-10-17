BEGIN;

-- Uploaded inline images are separate rights-managed assets. The associated
-- compliance_assets row remains the source of truth for approval/evidence.
CREATE TABLE IF NOT EXISTS portal.uploaded_image_assets (
  image_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  filename TEXT NOT NULL UNIQUE CHECK (filename ~ '^[0-9a-f-]{36}\.(png|jpg|gif|webp|avif)$'),
  uploader_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE RESTRICT,
  compliance_asset_id UUID NOT NULL UNIQUE REFERENCES portal.compliance_assets(asset_id) ON DELETE RESTRICT,
  mime_type TEXT NOT NULL CHECK (mime_type IN ('image/png','image/jpeg','image/gif','image/webp','image/avif')),
  sha256 CHAR(64) NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
  file_size_bytes BIGINT NOT NULL CHECK (file_size_bytes BETWEEN 1 AND 8388608),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS uploaded_image_assets_uploader_idx
  ON portal.uploaded_image_assets(uploader_id, created_at DESC);
CREATE INDEX IF NOT EXISTS uploaded_image_assets_sha_idx
  ON portal.uploaded_image_assets(sha256);

ALTER TABLE portal.uploaded_image_assets OWNER TO personal_portal_app;
REVOKE ALL ON portal.uploaded_image_assets FROM PUBLIC;

INSERT INTO portal.schema_migrations(version, name)
VALUES (19, 'uploaded_image_rights_gate')
ON CONFLICT (version) DO NOTHING;
COMMIT;
