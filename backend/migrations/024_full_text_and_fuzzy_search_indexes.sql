BEGIN;

-- pg_trgm supports indexed, case-insensitive substring and fuzzy matching.
-- The contrib package must be installed on the PostgreSQL host before this migration.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- array_to_string(anyarray, text) is marked STABLE because it accepts any
-- element type; this typed TEXT[] wrapper is immutable for our tag values.
CREATE OR REPLACE FUNCTION portal.text_array_join(input text[], separator text DEFAULT ' ')
RETURNS text LANGUAGE SQL IMMUTABLE PARALLEL SAFE
AS $$ SELECT array_to_string(input, separator) $$;

-- Full-text search across the entire published-content payload. The trigram
-- companion index is important for CJK text, where the built-in 'simple'
-- text parser does not perform Chinese word segmentation.
CREATE INDEX IF NOT EXISTS portal_content_search_vector_idx
  ON portal.portal_content USING gin (
    to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, ''))
  );
CREATE INDEX IF NOT EXISTS portal_content_search_trgm_idx
  ON portal.portal_content USING gin (
    lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')) gin_trgm_ops
  ) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS portal_content_title_trgm_idx
  ON portal.portal_content USING gin (lower(title) gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS portal_content_summary_trgm_idx
  ON portal.portal_content USING gin (lower(summary) gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS portal_content_slug_lower_idx
  ON portal.portal_content (lower(slug)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS portal_content_trash_search_trgm_idx
  ON portal.portal_content USING gin (
    lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')) gin_trgm_ops
  ) WHERE deleted_at IS NOT NULL;

-- Search resource name, description, original filename, type, grade and URL.
-- Separate title/description/filename indexes support targeted searches;
-- the combined index covers the public directory's multi-field search box.
CREATE INDEX IF NOT EXISTS teaching_resources_search_trgm_idx
  ON portal.teaching_resources USING gin (
    lower(coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(original_filename, '') || ' ' || coalesce(resource_type, '') || ' ' || coalesce(grade, '') || ' ' || coalesce(resource_url, '')) gin_trgm_ops
  ) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_title_trgm_idx
  ON portal.teaching_resources USING gin (lower(title) gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_description_trgm_idx
  ON portal.teaching_resources USING gin (lower(description) gin_trgm_ops)
  WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_filename_trgm_idx
  ON portal.teaching_resources USING gin (lower(original_filename) gin_trgm_ops)
  WHERE deleted_at IS NULL AND original_filename IS NOT NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_trash_search_trgm_idx
  ON portal.teaching_resources USING gin (
    lower(coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(original_filename, '') || ' ' || coalesce(resource_type, '') || ' ' || coalesce(grade, '') || ' ' || coalesce(resource_url, '')) gin_trgm_ops
  ) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_title_lower_idx
  ON portal.teaching_resources (lower(title)) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_filename_lower_idx
  ON portal.teaching_resources (lower(original_filename))
  WHERE deleted_at IS NULL AND original_filename IS NOT NULL;

ANALYZE portal.portal_content;
ANALYZE portal.teaching_resources;

INSERT INTO portal.schema_migrations(version, name)
VALUES (24, 'full_text_and_fuzzy_search_indexes')
ON CONFLICT (version) DO NOTHING;
COMMIT;
