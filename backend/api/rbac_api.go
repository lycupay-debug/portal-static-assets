package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

type roleAssignmentRequest struct {
	Roles []string `json:"roles"`
}

type rolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
}

func (a *portalAPI) requireSuperAdmin(w http.ResponseWriter, r *http.Request) (*portalUser, bool) {
	u, ok := a.requireUser(w, r)
	if !ok {
		return nil, false
	}
	var allowed bool
	err := a.db.QueryRowContext(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM portal.portal_user_roles
			WHERE user_id=$1::uuid AND role_key='super_admin'
		) AND EXISTS (
			SELECT 1 FROM portal.portal_users
			WHERE user_id=$1::uuid AND is_admin=TRUE AND lower(email)='lycupay@gmail.com'
		)`, u.ID).Scan(&allowed)
	if err != nil {
		a.dbError(w, err, "无法验证超级管理员权限。")
		return nil, false
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "只有指定的超级管理员可以管理账户权限。", nil)
		return nil, false
	}
	return u, true
}

func (a *portalAPI) rbac(w http.ResponseWriter, r *http.Request) {
	u, ok := a.requireSuperAdmin(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin")
	switch {
	case path == "/users" && r.Method == http.MethodGet:
		items, err := queryJSONRows(r.Context(), a.db, `
			SELECT to_jsonb(x) FROM (
				SELECT u.user_id::text AS id, u.email, u.display_name AS "displayName",
				       u.is_admin AS "isAdmin", u.is_copyright_reviewer AS "isCopyrightReviewer",
				       u.created_at AS "createdAt",
				       COALESCE((SELECT jsonb_agg(ur.role_key ORDER BY ur.role_key)
				                 FROM portal.portal_user_roles ur WHERE ur.user_id=u.user_id), '[]'::jsonb) AS roles
				FROM portal.portal_users u ORDER BY u.created_at DESC
			) x`)
		if err != nil {
			a.dbError(w, err, "无法读取账户权限列表。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	case path == "/roles" && r.Method == http.MethodGet:
		items, err := queryJSONRows(r.Context(), a.db, `
			SELECT to_jsonb(x) FROM (
				SELECT r.role_key AS "roleKey", r.display_name AS "displayName",
				       r.description, r.is_system AS "isSystem",
				       COALESCE((SELECT jsonb_agg(rp.permission_key ORDER BY rp.permission_key)
				                 FROM portal.role_permissions rp WHERE rp.role_key=r.role_key), '[]'::jsonb) AS permissions
				FROM portal.roles r ORDER BY r.role_key
			) x`)
		if err != nil {
			a.dbError(w, err, "无法读取角色和权限定义。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	case path == "/permissions" && r.Method == http.MethodGet:
		items, err := queryJSONRows(r.Context(), a.db, `
			SELECT to_jsonb(x) FROM (
				SELECT permission_key AS "permissionKey", description
				FROM portal.permissions ORDER BY permission_key
			) x`)
		if err != nil {
			a.dbError(w, err, "无法读取权限定义。")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
		return
	case strings.HasPrefix(path, "/users/") && strings.HasSuffix(path, "/roles") && r.Method == http.MethodPatch:
		a.updateUserRoles(w, r, u, strings.TrimSuffix(strings.TrimPrefix(path, "/users/"), "/roles"))
		return
	case strings.HasPrefix(path, "/roles/") && strings.HasSuffix(path, "/permissions") && r.Method == http.MethodPatch:
		a.updateRolePermissions(w, r, u, strings.TrimSuffix(strings.TrimPrefix(path, "/roles/"), "/permissions"))
		return
	default:
		writeError(w, http.StatusNotFound, "未找到权限管理接口。", nil)
	}
}

func (a *portalAPI) updateUserRoles(w http.ResponseWriter, r *http.Request, actor *portalUser, id string) {
	if !portalUUID.MatchString(id) {
		writeError(w, http.StatusBadRequest, "账户标识无效。", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body roleAssignmentRequest
	if err := decoder.Decode(&body); err != nil || body.Roles == nil {
		writeError(w, http.StatusBadRequest, "请提供 roles 字符串数组。", nil)
		return
	}
	roles := make(map[string]bool, len(body.Roles))
	for _, role := range body.Roles {
		role = strings.TrimSpace(role)
		if role == "" || roles[role] {
			writeError(w, http.StatusBadRequest, "角色列表包含空值或重复项。", nil)
			return
		}
		roles[role] = true
	}
	for role := range roles {
		var exists bool
		if err := a.db.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM portal.roles WHERE role_key=$1)", role).Scan(&exists); err != nil {
			a.dbError(w, err, "无法验证角色定义。")
			return
		}
		if !exists {
			writeError(w, http.StatusBadRequest, "包含不存在的角色。", nil)
			return
		}
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		a.dbError(w, err, "无法开始权限变更事务。")
		return
	}
	defer tx.Rollback()
	var email string
	var beforeRoles []byte
	err = tx.QueryRowContext(r.Context(), `
		SELECT email, COALESCE((SELECT jsonb_agg(role_key ORDER BY role_key)
		 FROM portal.portal_user_roles WHERE user_id=u.user_id), '[]'::jsonb)
		FROM portal.portal_users u WHERE user_id=$1::uuid FOR UPDATE`, id).Scan(&email, &beforeRoles)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "账户不存在。", nil)
		return
	}
	if err != nil {
		a.dbError(w, err, "无法读取账户当前权限。")
		return
	}
	isDesignatedAdmin := strings.EqualFold(email, "lycupay@gmail.com")
	if isDesignatedAdmin && !roles["super_admin"] {
		writeError(w, http.StatusConflict, "指定超级管理员必须保留 super_admin 角色。", nil)
		return
	}
	if !isDesignatedAdmin && roles["super_admin"] {
		writeError(w, http.StatusForbidden, "当前策略只允许 lycupay@gmail.com 担任超级管理员。", nil)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM portal.portal_user_roles WHERE user_id=$1::uuid", id); err != nil {
		a.dbError(w, err, "无法更新账户角色。")
		return
	}
	for role := range roles {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal.portal_user_roles(user_id,role_key,assigned_by) VALUES($1::uuid,$2,$3)", id, role, actor.Email); err != nil {
			a.dbError(w, err, "无法保存账户角色。")
			return
		}
	}
	_, err = tx.ExecContext(r.Context(), `
		UPDATE portal.portal_users SET is_admin=$1, is_copyright_reviewer=$2, updated_at=now()
		WHERE user_id=$3::uuid`, roles["super_admin"], roles["copyright_reviewer"], id)
	if err != nil {
		a.dbError(w, err, "无法同步兼容权限字段。")
		return
	}
	afterRoles, _ := json.Marshal(sortedKeys(roles))
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state)
		VALUES($1,'update_user_roles','portal_user',$2::uuid,
		       jsonb_build_object('email',$3::text,'roles',$4::jsonb),
		       jsonb_build_object('email',$3::text,'roles',$5::jsonb))`,
		actor.Email, id, email, string(beforeRoles), string(afterRoles))
	if err != nil {
		a.dbError(w, err, "无法记录权限变更审计。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "权限变更事务提交失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "email": email, "roles": sortedKeys(roles), "isAdmin": roles["super_admin"], "isCopyrightReviewer": roles["copyright_reviewer"]})
}

func (a *portalAPI) updateRolePermissions(w http.ResponseWriter, r *http.Request, actor *portalUser, roleKey string) {
	if roleKey == "" || strings.Contains(roleKey, "/") {
		writeError(w, http.StatusBadRequest, "角色标识无效。", nil)
		return
	}
	if roleKey == "super_admin" {
		writeError(w, http.StatusConflict, "超级管理员角色的基础权限不可移除。", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var body rolePermissionsRequest
	if err := decoder.Decode(&body); err != nil || body.Permissions == nil {
		writeError(w, http.StatusBadRequest, "请提供 permissions 字符串数组。", nil)
		return
	}
	permissions := make(map[string]bool, len(body.Permissions))
	for _, permission := range body.Permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" || permissions[permission] {
			writeError(w, http.StatusBadRequest, "权限列表包含空值或重复项。", nil)
			return
		}
		permissions[permission] = true
	}
	tx, err := a.db.BeginTx(r.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		a.dbError(w, err, "无法开始权限变更事务。")
		return
	}
	defer tx.Rollback()
	var before []byte
	err = tx.QueryRowContext(r.Context(), `
		SELECT COALESCE(jsonb_agg(permission_key ORDER BY permission_key),'[]'::jsonb)
		FROM portal.role_permissions WHERE role_key=$1`, roleKey).Scan(&before)
	if err != nil {
		a.dbError(w, err, "无法读取角色权限。")
		return
	}
	var roleExists bool
	if err = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM portal.roles WHERE role_key=$1)", roleKey).Scan(&roleExists); err != nil {
		a.dbError(w, err, "无法验证角色。")
		return
	}
	if !roleExists {
		writeError(w, http.StatusNotFound, "角色不存在。", nil)
		return
	}
	for permission := range permissions {
		var exists bool
		if err = tx.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM portal.permissions WHERE permission_key=$1)", permission).Scan(&exists); err != nil {
			a.dbError(w, err, "无法验证权限定义。")
			return
		}
		if !exists {
			writeError(w, http.StatusBadRequest, "包含不存在的权限标识。", nil)
			return
		}
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM portal.role_permissions WHERE role_key=$1", roleKey); err != nil {
		a.dbError(w, err, "无法更新角色权限。")
		return
	}
	for permission := range permissions {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal.role_permissions(role_key,permission_key) VALUES($1,$2)", roleKey, permission); err != nil {
			a.dbError(w, err, "无法保存角色权限。")
			return
		}
	}
	after, _ := json.Marshal(sortedKeys(permissions))
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO portal.compliance_audit_log(actor,action,object_type,object_id,before_state,after_state)
		VALUES($1,'update_role_permissions','portal_role',NULL,
		       jsonb_build_object('roleKey',$2::text,'permissions',$3::jsonb),
		       jsonb_build_object('roleKey',$2::text,'permissions',$4::jsonb))`,
		actor.Email, roleKey, string(before), string(after))
	if err != nil {
		a.dbError(w, err, "无法记录权限变更审计。")
		return
	}
	if err = tx.Commit(); err != nil {
		a.dbError(w, err, "权限变更事务提交失败。")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"roleKey": roleKey, "permissions": sortedKeys(permissions)})
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	// Role and permission identifiers are stable strings; sort for deterministic API output.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
