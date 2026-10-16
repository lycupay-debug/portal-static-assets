BEGIN;

ALTER TABLE portal.portal_content
  ADD COLUMN IF NOT EXISTS parent_category TEXT,
  ADD COLUMN IF NOT EXISTS child_category TEXT;

ALTER TABLE portal.portal_content
  DROP CONSTRAINT IF EXISTS portal_content_parent_category_check,
  DROP CONSTRAINT IF EXISTS portal_content_child_category_check,
  DROP CONSTRAINT IF EXISTS portal_content_taxonomy_pair_check,
  DROP CONSTRAINT IF EXISTS portal_content_taxonomy_presence_check;

ALTER TABLE portal.portal_content
  ADD CONSTRAINT portal_content_parent_category_check CHECK (
    parent_category IS NULL OR parent_category IN ('教育','年级','教育人力资源','旅游','网络','系统','编程','软件工具')
  ),
  ADD CONSTRAINT portal_content_child_category_check CHECK (
    child_category IS NULL OR child_category IN (
      '学前教育','小学教育','中学教育','高等教育','职业教育','家庭教育','教学资源',
      '一年级','二年级','三年级','四年级','五年级','六年级','初一','初二','初三','高一','高二','高三','本科','硕士',
      '事业编','国企','省考','国考',
      '住宿','交通','景点攻略','旅行规划','美食体验',
      '网络安全','网络维护','网络基础知识','网络诊断','云与服务器',
      '操作系统','系统配置','系统维护','部署运维','故障排查',
      '前端开发','后端开发','Python','JavaScript / TypeScript','数据库','API 与集成','代码实践',
      '效率工具','开发工具','文档工具','AI 工具','自动化工具'
    )
  ),
  ADD CONSTRAINT portal_content_taxonomy_presence_check CHECK ((parent_category IS NULL AND child_category IS NULL) OR (parent_category IS NOT NULL AND child_category IS NOT NULL)),
  ADD CONSTRAINT portal_content_taxonomy_pair_check CHECK (
    child_category IS NULL OR
    (parent_category = '教育' AND child_category IN ('学前教育','小学教育','中学教育','高等教育','职业教育','家庭教育','教学资源')) OR
    (parent_category = '年级' AND child_category IN ('一年级','二年级','三年级','四年级','五年级','六年级','初一','初二','初三','高一','高二','高三','本科','硕士')) OR
    (parent_category = '教育人力资源' AND child_category IN ('事业编','国企','省考','国考')) OR
    (parent_category = '旅游' AND child_category IN ('住宿','交通','景点攻略','旅行规划','美食体验')) OR
    (parent_category = '网络' AND child_category IN ('网络安全','网络维护','网络基础知识','网络诊断','云与服务器')) OR
    (parent_category = '系统' AND child_category IN ('操作系统','系统配置','系统维护','部署运维','故障排查')) OR
    (parent_category = '编程' AND child_category IN ('前端开发','后端开发','Python','JavaScript / TypeScript','数据库','API 与集成','代码实践')) OR
    (parent_category = '软件工具' AND child_category IN ('效率工具','开发工具','文档工具','AI 工具','自动化工具'))
  );

CREATE INDEX IF NOT EXISTS portal_content_taxonomy_idx
  ON portal.portal_content(parent_category, child_category)
  WHERE deleted_at IS NULL;

INSERT INTO portal.schema_migrations(version, name)
VALUES (13, 'content_taxonomy_fields')
ON CONFLICT (version) DO NOTHING;
COMMIT;
