package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const dummyLoginPasswordHash = "$2a$12$93GixgSd39NVDadst0KoHOgSJjg7jLOIBi3/9fG2OiP.ubz58RJXy"

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func readSessionToken(r *http.Request) string {
	c, err := r.Cookie(portalCookie)
	if err != nil || len(c.Value) > 128 {
		return ""
	}
	return c.Value
}
func setSessionCookie(w http.ResponseWriter, token string, age int) {
	w.Header().Add("Set-Cookie", portalCookie+"=; Path=/api/auth; HttpOnly; Secure; SameSite=Strict; Max-Age=0")
	w.Header().Add("Set-Cookie", fmt.Sprintf("%s=%s; Path=/api; HttpOnly; Secure; SameSite=Strict; Max-Age=%d", portalCookie, token, age))
}
func clearSessionCookie(w http.ResponseWriter) {
	w.Header().Add("Set-Cookie", portalCookie+"=; Path=/api; HttpOnly; Secure; SameSite=Strict; Max-Age=0")
	w.Header().Add("Set-Cookie", portalCookie+"=; Path=/api/auth; HttpOnly; Secure; SameSite=Strict; Max-Age=0")
}
func (a *portalAPI) session(r *http.Request) (*portalUser, error) {
	token := readSessionToken(r)
	if token == "" {
		return nil, nil
	}
	var u portalUser
	var expires time.Time
	err := a.db.QueryRowContext(r.Context(), `SELECT u.user_id::text,u.email,u.display_name,u.created_at,u.is_admin,s.expires_at FROM portal.portal_auth_sessions s JOIN portal.portal_users u ON u.user_id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash(token)).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.IsAdmin, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.ExpiresAt = &expires
	return &u, nil
}
func (a *portalAPI) requireUser(w http.ResponseWriter, r *http.Request) (*portalUser, bool) {
	u, err := a.session(r)
	if err != nil {
		writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
		return nil, false
	}
	if u == nil {
		writeError(w, 401, "请先登录后再管理内容。", nil)
		return nil, false
	}
	return u, true
}
func (a *portalAPI) createSession(ctx context.Context, userID string, w http.ResponseWriter) error {
	_, err := a.db.ExecContext(ctx, "DELETE FROM portal.portal_auth_sessions WHERE expires_at<=now()")
	if err != nil {
		return err
	}
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	_, err = a.db.ExecContext(ctx, `INSERT INTO portal.portal_auth_sessions(user_id,token_hash,expires_at) VALUES($1,$2,now()+($3*interval '1 day'))`, userID, tokenHash(token), portalSessionDays)
	if err == nil {
		setSessionCookie(w, token, portalSessionDays*86400)
	}
	return err
}
func portalRegistrationEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PERSONAL_PORTAL_REGISTRATION_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (a *portalAPI) auth(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/auth/register" && r.Method == http.MethodPost:
		if !portalRegistrationEnabled() {
			writeError(w, http.StatusForbidden, "当前暂不开放注册。", nil)
			return
		}
		if a.rateLimited(w, r) {
			return
		}
		var b struct {
			Email       string `json:"email"`
			DisplayName string `json:"displayName"`
			Password    string `json:"password"`
		}
		if err := decodeJSON(w, r, 16*1024, &b); err != nil {
			writeError(w, 400, "请求格式不正确。", nil)
			return
		}
		b.Email = strings.ToLower(strings.TrimSpace(b.Email))
		b.DisplayName = strings.TrimSpace(b.DisplayName)
		if !portalEmail.MatchString(b.Email) || len(b.Email) > 254 {
			writeError(w, 400, "请输入有效的邮箱地址。", nil)
			return
		}
		if len(b.Password) < 10 || len(b.Password) > 72 || !utf8.ValidString(b.Password) || len([]byte(b.Password)) > 72 {
			writeError(w, 400, "密码须为 10 至 72 个字符，且 UTF-8 编码不超过 72 字节。", nil)
			return
		}
		if runeLen(b.DisplayName) > 60 {
			writeError(w, 400, "昵称不能超过 60 个字符。", nil)
			return
		}
		if b.DisplayName == "" {
			b.DisplayName = strings.Split(b.Email, "@")[0]
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(b.Password), 12)
		if err != nil {
			writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
			return
		}
		var u portalUser
		err = a.db.QueryRowContext(r.Context(), `INSERT INTO portal.portal_users(email,display_name,password_hash) VALUES($1,$2,$3) RETURNING user_id::text,email,display_name,created_at,is_admin`, b.Email, b.DisplayName, string(hash)).Scan(&u.ID, &u.Email, &u.DisplayName, &u.CreatedAt, &u.IsAdmin)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "23505") {
				writeError(w, 409, "该邮箱已注册，请直接登录。", nil)
			} else {
				writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
			}
			return
		}
		if err = a.createSession(r.Context(), u.ID, w); err != nil {
			writeError(w, 500, "账户已创建，但会话建立失败，请重新登录。", nil)
			return
		}
		writeJSON(w, 201, map[string]any{"user": u})
	case path == "/auth/login" && r.Method == http.MethodPost:
		if a.rateLimited(w, r) {
			return
		}
		var b struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := decodeJSON(w, r, 16*1024, &b); err != nil {
			writeError(w, 400, "请求格式不正确。", nil)
			return
		}
		b.Email = strings.ToLower(strings.TrimSpace(b.Email))
		if !portalEmail.MatchString(b.Email) || b.Password == "" || len(b.Password) > 72 {
			writeError(w, 400, "请输入正确的邮箱和密码。", nil)
			return
		}
		var u portalUser
		var hash string
		err := a.db.QueryRowContext(r.Context(), `SELECT user_id::text,email,display_name,password_hash,created_at,is_admin FROM portal.portal_users WHERE email=$1`, b.Email).Scan(&u.ID, &u.Email, &u.DisplayName, &hash, &u.CreatedAt, &u.IsAdmin)
		if errors.Is(err, sql.ErrNoRows) {
			// Keep the missing-account path close to the wrong-password path.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyLoginPasswordHash), []byte(b.Password))
			writeError(w, 401, "邮箱或密码不正确。", nil)
			return
		}
		if err != nil {
			a.dbError(w, err, "登录账户查询失败。")
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(b.Password)) != nil {
			writeError(w, 401, "邮箱或密码不正确。", nil)
			return
		}
		if err = a.createSession(r.Context(), u.ID, w); err != nil {
			writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
			return
		}
		writeJSON(w, 200, map[string]any{"user": u})
	case path == "/auth/me" && r.Method == http.MethodGet:
		u, err := a.session(r)
		if err != nil {
			writeError(w, 500, "服务暂时不可用，请稍后再试。", nil)
			return
		}
		if u == nil {
			writeError(w, 401, "尚未登录。", nil)
			return
		}
		age := int(time.Until(*u.ExpiresAt).Seconds())
		if age < 1 {
			age = 1
		}
		setSessionCookie(w, readSessionToken(r), age)
		u.ExpiresAt = nil
		writeJSON(w, 200, map[string]any{"user": u})
	case path == "/auth/logout" && r.Method == http.MethodPost:
		if token := readSessionToken(r); token != "" {
			if _, err := a.db.ExecContext(r.Context(), "DELETE FROM portal.portal_auth_sessions WHERE token_hash=$1", tokenHash(token)); err != nil {
				fmt.Fprintf(os.Stderr, "portal_api_error operation=%q error=%v\n", "logout session revocation", err)
				clearSessionCookie(w)
				writeError(w, http.StatusServiceUnavailable, "退出登录状态未能确认，请稍后重试。", nil)
				return
			}
		}
		clearSessionCookie(w)
		writeJSON(w, 200, map[string]any{"ok": true})
	default:
		writeError(w, 405, "不支持的请求方式。", nil)
	}
}
