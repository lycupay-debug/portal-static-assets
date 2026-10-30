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

func (a *portalAPI) updateUserRoles(w http.ResponseWriter, r *http.Request, actor *portalUser, id string) {
	if !portalUUID.MatchString(id) {
		writeError(w, http.StatusBadRequest, "账户标识无效。", nil)
		return
	}
	var body roleAssignmentRequest
	if err := decodeJSON(w, r, 16*1024, &body); err != nil || body.Roles == nil {
		writeError(w, http.StatusBadRequest, "请提供 roles 字符串数组。", nil)
		return
	}
	if len(body.Roles) == 0 {
		writeError(w, http.StatusBadRequest, "账户至少必须保留一个角色。", nil)
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
