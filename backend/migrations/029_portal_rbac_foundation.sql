BEGIN;

CREATE TABLE IF NOT EXISTS portal.roles (
  role_key TEXT PRIMARY KEY CHECK (role_key ~ '^[a-z][a-z0-9_.-]{1,63}$'),
  display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 80),
  description TEXT NOT NULL DEFAULT '',
  is_system BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS portal.permissions (
  permission_key TEXT PRIMARY KEY CHECK (permission_key ~ '^[a-z][a-z0-9_.-]{1,95}$'),
  description TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS portal.role_permissions (
  role_key TEXT NOT NULL REFERENCES portal.roles(role_key) ON DELETE CASCADE,
  permission_key TEXT NOT NULL REFERENCES portal.permissions(permission_key) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (role_key, permission_key)
);
CREATE TABLE IF NOT EXISTS portal.portal_user_roles (
  user_id UUID NOT NULL REFERENCES portal.portal_users(user_id) ON DELETE CASCADE,
  role_key TEXT NOT NULL REFERENCES portal.roles(role_key) ON DELETE RESTRICT,
  assigned_by TEXT NOT NULL DEFAULT 'system',
  assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS portal_user_roles_single_super_admin
  ON portal.portal_user_roles(role_key) WHERE role_key = 'super_admin';
CREATE UNIQUE INDEX IF NOT EXISTS portal_users_single_admin
  ON portal.portal_users ((is_admin)) WHERE is_admin = TRUE;

INSERT INTO portal.roles(role_key, display_name, description, is_system) VALUES
  ('super_admin', '超级管理员', '拥有全站管理和权限分配能力；系统最多允许一个超级管理员。', TRUE),
  ('user', '普通用户', '默认注册角色，不具备后台管理权限。', TRUE),
  ('copyright_reviewer', '版权合规审核员', '仅处理内部资源合规与审核工作。', TRUE)
ON CONFLICT (role_key) DO UPDATE SET display_name=EXCLUDED.display_name, description=EXCLUDED.description;

INSERT INTO portal.permissions(permission_key, description) VALUES
  ('portal.users.read', '查看账户与角色'),
  ('portal.users.roles.manage', '分配和撤销账户角色'),
  ('portal.roles.read', '查看角色和权限'),
  ('portal.roles.permissions.manage', '配置角色权限'),
  ('portal.system_ops.read', '查看系统运维信息'),
  ('portal.system_ops.manage', '管理系统运维策略和任务'),
  ('portal.compliance.read', '查看内部合规资料'),
  ('portal.compliance.manage', '维护内部合规资料与审核权限')
ON CONFLICT (permission_key) DO UPDATE SET description=EXCLUDED.description;

INSERT INTO portal.role_permissions(role_key, permission_key)
SELECT 'super_admin', permission_key FROM portal.permissions
ON CONFLICT DO NOTHING;
INSERT INTO portal.role_permissions(role_key, permission_key)
VALUES ('copyright_reviewer','portal.compliance.read'),
       ('copyright_reviewer','portal.compliance.manage')
ON CONFLICT DO NOTHING;

INSERT INTO portal.portal_user_roles(user_id, role_key, assigned_by)
SELECT user_id, 'super_admin', 'migration-029-legacy-admin'
FROM portal.portal_users WHERE is_admin=TRUE
ON CONFLICT DO NOTHING;
INSERT INTO portal.portal_user_roles(user_id, role_key, assigned_by)
SELECT user_id, 'copyright_reviewer', 'migration-029-legacy-reviewer'
FROM portal.portal_users WHERE is_copyright_reviewer=TRUE
ON CONFLICT DO NOTHING;
INSERT INTO portal.portal_user_roles(user_id, role_key, assigned_by)
SELECT user_id, 'user', 'migration-029-default'
FROM portal.portal_users u
WHERE NOT EXISTS (
  SELECT 1 FROM portal.portal_user_roles ur
  WHERE ur.user_id=u.user_id AND ur.role_key IN ('super_admin','copyright_reviewer')
)
ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION portal.assign_default_portal_role()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  INSERT INTO portal.portal_user_roles(user_id, role_key, assigned_by)
  VALUES (NEW.user_id, 'user', 'registration')
  ON CONFLICT DO NOTHING;
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS portal_users_assign_default_role ON portal.portal_users;
CREATE TRIGGER portal_users_assign_default_role
AFTER INSERT ON portal.portal_users
FOR EACH ROW EXECUTE FUNCTION portal.assign_default_portal_role();
REVOKE ALL ON FUNCTION portal.assign_default_portal_role() FROM PUBLIC;

ALTER TABLE portal.roles OWNER TO personal_portal_app;
ALTER TABLE portal.permissions OWNER TO personal_portal_app;
ALTER TABLE portal.role_permissions OWNER TO personal_portal_app;
ALTER TABLE portal.portal_user_roles OWNER TO personal_portal_app;
REVOKE ALL ON TABLE portal.roles, portal.permissions, portal.role_permissions, portal.portal_user_roles FROM PUBLIC;

INSERT INTO portal.schema_migrations(version, name)
VALUES (29, 'portal_rbac_foundation')
ON CONFLICT (version) DO NOTHING;

COMMIT;
