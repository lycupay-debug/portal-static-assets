BEGIN;

-- Normalize away all whitespace after lowercasing, so searches for "unit 5",
-- "unit5", and "unit    5" match the same values. These expression indexes
-- preserve the original display values; normalization is search-only.
CREATE INDEX IF NOT EXISTS portal_content_search_compact_trgm_idx
  ON portal.portal_content USING gin (
    regexp_replace(lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')), '[[:space:]]+', '', 'g') gin_trgm_ops
  ) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS portal_content_trash_search_compact_trgm_idx
  ON portal.portal_content USING gin (
    regexp_replace(lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(body, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')), '[[:space:]]+', '', 'g') gin_trgm_ops
  ) WHERE deleted_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_search_compact_trgm_idx
  ON portal.teaching_resources USING gin (
    regexp_replace(lower(coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(original_filename, '') || ' ' || coalesce(resource_type, '') || ' ' || coalesce(grade, '') || ' ' || coalesce(resource_url, '')), '[[:space:]]+', '', 'g') gin_trgm_ops
  ) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS teaching_resources_trash_search_compact_trgm_idx
  ON portal.teaching_resources USING gin (
    regexp_replace(lower(coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(original_filename, '') || ' ' || coalesce(resource_type, '') || ' ' || coalesce(grade, '') || ' ' || coalesce(resource_url, '')), '[[:space:]]+', '', 'g') gin_trgm_ops
  ) WHERE deleted_at IS NOT NULL;

ANALYZE portal.portal_content;
ANALYZE portal.teaching_resources;

INSERT INTO portal.schema_migrations(version, name)
VALUES (26, 'whitespace_insensitive_search_indexes')
ON CONFLICT (version) DO NOTHING;
COMMIT;
