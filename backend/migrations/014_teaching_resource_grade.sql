BEGIN;
ALTER TABLE portal.teaching_resources ADD COLUMN IF NOT EXISTS grade TEXT;
ALTER TABLE portal.teaching_resources DROP CONSTRAINT IF EXISTS teaching_resources_grade_check;
ALTER TABLE portal.teaching_resources ADD CONSTRAINT teaching_resources_grade_check CHECK (
  grade IS NULL OR grade IN ('一年级','二年级','三年级','四年级','五年级','六年级','初一','初二','初三','高一','高二','高三','本科','硕士')
);
CREATE INDEX IF NOT EXISTS teaching_resources_owner_grade_idx ON portal.teaching_resources(owner_id, grade) WHERE deleted_at IS NULL;
INSERT INTO portal.schema_migrations(version, name) VALUES (14, 'teaching_resource_grade') ON CONFLICT (version) DO NOTHING;
COMMIT;
