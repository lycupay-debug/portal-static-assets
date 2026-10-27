package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

type rolePermissionsRequest struct {
	Permissions []string `json:"permissions"`
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
