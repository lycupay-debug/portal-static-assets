BEGIN;

-- GIN trigram indexes accelerate substring LIKE searches. GiST trigram indexes
-- additionally accelerate the similarity (%) operator used for typo-tolerant
-- fuzzy matches. These indexes include active and trashed records.
CREATE INDEX IF NOT EXISTS portal_content_fuzzy_gist_idx
  ON portal.portal_content USING gist (
    lower(coalesce(title, '') || ' ' || coalesce(summary, '') || ' ' || coalesce(slug, '') || ' ' || coalesce(portal.text_array_join(tags, ' '), '') || ' ' || coalesce(parent_category, '') || ' ' || coalesce(child_category, '')) gist_trgm_ops
  );
CREATE INDEX IF NOT EXISTS teaching_resources_fuzzy_gist_idx
  ON portal.teaching_resources USING gist (
    lower(coalesce(title, '') || ' ' || coalesce(description, '') || ' ' || coalesce(original_filename, '') || ' ' || coalesce(resource_type, '') || ' ' || coalesce(grade, '') || ' ' || coalesce(resource_url, '')) gist_trgm_ops
  );

ANALYZE portal.portal_content;
ANALYZE portal.teaching_resources;

INSERT INTO portal.schema_migrations(version, name)
VALUES (25, 'gist_fuzzy_search_indexes')
ON CONFLICT (version) DO NOTHING;
COMMIT;
