BEGIN;
-- Migration 017 was generated from a shell-safe template and stored literal \n
-- markers. Normalize them to actual Markdown line breaks without changing text.
UPDATE portal.compliance_legal_documents
SET body_markdown=replace(body_markdown, chr(92) || 'n', chr(10)), updated_at=now()
WHERE slug IN ('copyright-notice','privacy-policy','terms-of-use','copyright-complaints')
  AND position(chr(92) || 'n' in body_markdown) > 0;

UPDATE portal.compliance_legal_document_versions
SET body_markdown=replace(body_markdown, chr(92) || 'n', chr(10))
WHERE document_id IN (
  SELECT document_id FROM portal.compliance_legal_documents
  WHERE slug IN ('copyright-notice','privacy-policy','terms-of-use','copyright-complaints')
)
AND position(chr(92) || 'n' in body_markdown) > 0;

UPDATE portal.compliance_legal_documents
SET missing_information = missing_information || jsonb_build_array('核实网站备案/许可状态及首页展示信息（如适用）')
WHERE slug='terms-of-use'
  AND NOT (missing_information @> jsonb_build_array('核实网站备案/许可状态及首页展示信息（如适用）'));

INSERT INTO portal.schema_migrations(version,name)
VALUES (18,'normalize_policy_drafts')
ON CONFLICT (version) DO NOTHING;
COMMIT;
