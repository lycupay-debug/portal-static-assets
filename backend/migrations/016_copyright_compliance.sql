BEGIN;
CREATE TABLE IF NOT EXISTS portal.compliance_license_catalog (
  license_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), spdx_id TEXT NOT NULL DEFAULT '', license_name TEXT NOT NULL, license_version TEXT NOT NULL DEFAULT '',
  license_family TEXT NOT NULL DEFAULT 'other' CHECK (license_family IN ('permissive','copyleft','weak_copyleft','proprietary','public_domain','custom','other')),
  canonical_url TEXT NOT NULL DEFAULT '', full_text TEXT NOT NULL DEFAULT '', obligations JSONB NOT NULL DEFAULT '[]'::jsonb,
  commercial_use_allowed BOOLEAN, modification_allowed BOOLEAN, redistribution_allowed BOOLEAN, attribution_required BOOLEAN, source_code_offer_required BOOLEAN,
  review_status TEXT NOT NULL DEFAULT 'unreviewed' CHECK (review_status IN ('unreviewed','reviewed','restricted','needs_counsel')), reviewed_at TIMESTAMPTZ,
  notes TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT compliance_license_name_nonempty CHECK (char_length(btrim(license_name)) BETWEEN 1 AND 200), CONSTRAINT compliance_license_spdx_version_unique UNIQUE (spdx_id, license_version)
);
CREATE TABLE IF NOT EXISTS portal.compliance_assets (
  asset_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), asset_type TEXT NOT NULL CHECK (asset_type IN ('image','icon','font','audio','video','article','education_resource','software_dependency','source_file','policy_document','external_link','other')),
  asset_name TEXT NOT NULL, asset_path TEXT NOT NULL DEFAULT '', source_url TEXT NOT NULL DEFAULT '', creator_or_rightsholder TEXT NOT NULL DEFAULT '',
  ownership_basis TEXT NOT NULL DEFAULT 'unknown' CHECK (ownership_basis IN ('original','commissioned','licensed','open_license','public_domain','user_submitted','third_party_permission','unknown')),
  license_id UUID REFERENCES portal.compliance_license_catalog(license_id) ON DELETE SET NULL,
  permission_status TEXT NOT NULL DEFAULT 'unknown' CHECK (permission_status IN ('confirmed','conditional','unknown','not_permitted','not_applicable')),
  use_scope TEXT NOT NULL DEFAULT 'unknown' CHECK (use_scope IN ('commercial_web','internal_only','redistribution','modification','multiple','unknown')),
  distribution_status TEXT NOT NULL DEFAULT 'unknown' CHECK (distribution_status IN ('served_publicly','bundled_in_frontend','server_side_only','internal_only','not_distributed','unknown')),
  review_status TEXT NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending','in_review','approved','approved_with_conditions','replace_or_remove','blocked')),
  risk_level TEXT NOT NULL DEFAULT 'unknown' CHECK (risk_level IN ('low','medium','high','critical','unknown')), sha256 CHAR(64) NOT NULL DEFAULT '', file_size_bytes BIGINT,
  discovered_by TEXT NOT NULL DEFAULT 'manual' CHECK (discovered_by IN ('manual','filesystem_scan','package_lock_scan','database_scan','url_scan','imported')),
  first_discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(), last_scanned_at TIMESTAMPTZ, published_or_active BOOLEAN NOT NULL DEFAULT FALSE,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb, notes TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT compliance_asset_name_nonempty CHECK (char_length(btrim(asset_name)) BETWEEN 1 AND 500), CONSTRAINT compliance_asset_sha256_format CHECK (sha256 = '' OR sha256 ~ '^[0-9a-f]{64}$')
);
CREATE UNIQUE INDEX IF NOT EXISTS compliance_assets_path_sha_unique ON portal.compliance_assets(asset_path, sha256) WHERE asset_path <> '';
CREATE INDEX IF NOT EXISTS compliance_assets_review_idx ON portal.compliance_assets(review_status, risk_level, asset_type);
CREATE INDEX IF NOT EXISTS compliance_assets_license_idx ON portal.compliance_assets(license_id);
CREATE INDEX IF NOT EXISTS compliance_assets_name_idx ON portal.compliance_assets USING gin (to_tsvector('simple', asset_name));
CREATE TABLE IF NOT EXISTS portal.compliance_asset_evidence (
  evidence_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), asset_id UUID NOT NULL REFERENCES portal.compliance_assets(asset_id) ON DELETE CASCADE,
  evidence_type TEXT NOT NULL CHECK (evidence_type IN ('license_text','source_page','purchase_receipt','permission_email','contract','author_declaration','screenshot','hash_record','repository_metadata','legal_review','other')),
  evidence_uri TEXT NOT NULL DEFAULT '', evidence_text TEXT NOT NULL DEFAULT '', sha256 CHAR(64) NOT NULL DEFAULT '', verified BOOLEAN NOT NULL DEFAULT FALSE,
  verified_by TEXT NOT NULL DEFAULT '', verified_at TIMESTAMPTZ, captured_at TIMESTAMPTZ NOT NULL DEFAULT now(), metadata JSONB NOT NULL DEFAULT '{}'::jsonb, notes TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS compliance_asset_evidence_asset_idx ON portal.compliance_asset_evidence(asset_id, captured_at DESC);
CREATE TABLE IF NOT EXISTS portal.compliance_asset_reviews (
  review_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), asset_id UUID NOT NULL REFERENCES portal.compliance_assets(asset_id) ON DELETE CASCADE,
  reviewer TEXT NOT NULL DEFAULT 'system', review_type TEXT NOT NULL DEFAULT 'copyright' CHECK (review_type IN ('copyright','license','privacy','content','distribution','legal_basis','recheck')),
  decision TEXT NOT NULL CHECK (decision IN ('approved','approved_with_conditions','pending_evidence','replace','remove','blocked','escalate')),
  risk_level TEXT NOT NULL CHECK (risk_level IN ('low','medium','high','critical','unknown')), findings TEXT NOT NULL DEFAULT '', required_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb, reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now(), follow_up_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS compliance_asset_reviews_asset_idx ON portal.compliance_asset_reviews(asset_id, reviewed_at DESC);
CREATE TABLE IF NOT EXISTS portal.compliance_scan_runs (
  scan_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), scan_type TEXT NOT NULL CHECK (scan_type IN ('filesystem','dependencies','database_content','license_texts','full_audit','manual')),
  scan_status TEXT NOT NULL DEFAULT 'running' CHECK (scan_status IN ('running','completed','completed_with_warnings','failed')), started_at TIMESTAMPTZ NOT NULL DEFAULT now(), completed_at TIMESTAMPTZ,
  source_revision TEXT NOT NULL DEFAULT '', assets_seen INTEGER NOT NULL DEFAULT 0, assets_created INTEGER NOT NULL DEFAULT 0, assets_updated INTEGER NOT NULL DEFAULT 0,
  findings_count INTEGER NOT NULL DEFAULT 0, report_path TEXT NOT NULL DEFAULT '', summary JSONB NOT NULL DEFAULT '{}'::jsonb, error_message TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS portal.compliance_legal_documents (
  document_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), slug TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(?:-[a-z0-9]+)*$'), title TEXT NOT NULL,
  document_type TEXT NOT NULL CHECK (document_type IN ('copyright_notice','privacy_policy','terms_of_use','copyright_complaints','third_party_notices','other')),
  jurisdiction TEXT NOT NULL DEFAULT 'CN', body_markdown TEXT NOT NULL DEFAULT '', body_html TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','in_review','published','archived')), version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  effective_at TIMESTAMPTZ, last_legal_review_at TIMESTAMPTZ, legal_basis JSONB NOT NULL DEFAULT '[]'::jsonb, factual_assumptions JSONB NOT NULL DEFAULT '[]'::jsonb,
  missing_information JSONB NOT NULL DEFAULT '[]'::jsonb, approved_by TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compliance_legal_documents_public_idx ON portal.compliance_legal_documents(slug) WHERE status='published';
CREATE TABLE IF NOT EXISTS portal.compliance_legal_document_versions (
  version_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), document_id UUID NOT NULL REFERENCES portal.compliance_legal_documents(document_id) ON DELETE CASCADE,
  version INTEGER NOT NULL, body_markdown TEXT NOT NULL, status TEXT NOT NULL, changed_by TEXT NOT NULL DEFAULT 'system', change_note TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT now(), UNIQUE(document_id, version)
);
CREATE TABLE IF NOT EXISTS portal.compliance_copyright_complaints (
  complaint_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), complainant_name TEXT NOT NULL, complainant_email TEXT NOT NULL,
  complainant_role TEXT NOT NULL DEFAULT 'rightsholder' CHECK (complainant_role IN ('rightsholder','authorized_agent','other')),
  work_description TEXT NOT NULL, proof_of_rights TEXT NOT NULL DEFAULT '', infringing_urls JSONB NOT NULL DEFAULT '[]'::jsonb,
  requested_action TEXT NOT NULL DEFAULT 'review_and_remove' CHECK (requested_action IN ('review_and_remove','disable_access','credit_or_license','other')),
  statement_truthful BOOLEAN NOT NULL DEFAULT FALSE, status TEXT NOT NULL DEFAULT 'received' CHECK (status IN ('received','under_review','need_more_information','action_taken','rejected','closed')),
  resolution_notes TEXT NOT NULL DEFAULT '', assigned_to TEXT NOT NULL DEFAULT '', received_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(), resolved_at TIMESTAMPTZ, privacy_retention_until TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS compliance_complaints_status_idx ON portal.compliance_copyright_complaints(status, received_at DESC);
CREATE TABLE IF NOT EXISTS portal.compliance_audit_log (
  audit_id UUID PRIMARY KEY DEFAULT gen_random_uuid(), actor TEXT NOT NULL DEFAULT 'system', action TEXT NOT NULL, object_type TEXT NOT NULL, object_id UUID,
  before_state JSONB NOT NULL DEFAULT '{}'::jsonb, after_state JSONB NOT NULL DEFAULT '{}'::jsonb, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS compliance_audit_log_object_idx ON portal.compliance_audit_log(object_type, object_id, created_at DESC);
ALTER TABLE portal.compliance_license_catalog OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_assets OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_asset_evidence OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_asset_reviews OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_scan_runs OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_legal_documents OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_legal_document_versions OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_copyright_complaints OWNER TO personal_portal_app;
ALTER TABLE portal.compliance_audit_log OWNER TO personal_portal_app;
REVOKE ALL ON ALL TABLES IN SCHEMA portal FROM PUBLIC;
INSERT INTO portal.schema_migrations(version, name) VALUES (16, 'copyright_compliance') ON CONFLICT (version) DO NOTHING;
COMMIT;
