package main

import (
	"net/http"
	"sort"
	"strings"
)

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

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
