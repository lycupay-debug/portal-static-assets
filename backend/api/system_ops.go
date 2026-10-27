package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *portalAPI) requireSystemOpsAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := a.requireUser(w, r)
	if !ok {
		return false
	}
	permission := "portal.system_ops.manage"
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		permission = "portal.system_ops.read"
	}
	var isAdmin bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT EXISTS (SELECT 1 FROM portal.portal_user_roles ur JOIN portal.role_permissions rp ON rp.role_key=ur.role_key WHERE ur.user_id=$1::uuid AND rp.permission_key=$2)`, u.ID, permission).Scan(&isAdmin); err != nil {
		a.dbError(w, err, "无法验证系统运维权限。")
		return false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "只有管理员可以查看系统运维信息。", nil)
		return false
	}
	return true
}

func (a *portalAPI) systemOps(w http.ResponseWriter, r *http.Request) {
	if !a.requireSystemOpsAdmin(w, r) {
		return
	}
	switch {
	case r.URL.Path == "/system-ops/policy":
		a.systemExecutionPolicy(w, r)
	case r.URL.Path == "/system-ops/tasks":
		switch r.Method {
		case http.MethodGet:
			a.listCommandTasks(w, r)
		case http.MethodPost:
			a.createCommandTask(w, r)
		default:
			w.Header().Set("Allow", "GET, POST")
			writeError(w, http.StatusMethodNotAllowed, "任务队列只支持 GET 和 POST。", nil)
		}
	case strings.HasPrefix(r.URL.Path, "/system-ops/tasks/") && strings.HasSuffix(r.URL.Path, "/approve") && r.Method == http.MethodPost:
		a.approveCommandTask(w, r)
	case strings.HasPrefix(r.URL.Path, "/system-ops/tasks/") && strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost:
		a.cancelCommandTask(w, r)
	case r.URL.Path == "/system-ops/logs" && r.Method == http.MethodGet:
		source := r.URL.Query().Get("source")
		path, ok := systemLogPath(source)
		if !ok {
			writeError(w, http.StatusBadRequest, "日志来源不正确。", nil)
			return
		}
		after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 100
		}
		result, err := readSystemLog(path, source, after, limit)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "当前日志源暂不可读取。", nil)
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.URL.Path == "/system-ops/warnings" && r.Method == http.MethodGet:
		now := time.Now()
		items := make([]systemLogEntry, 0, 40)
		serverPath, _ := systemLogPath("server")
		server, err := readSystemLog(serverPath, "server", 0, 200)
		if err == nil {
			seen := make(map[string]struct{})
			for i := len(server.Items) - 1; i >= 0 && len(items) < 40; i-- {
				entry := server.Items[i]
				if !actionableSystemWarning(entry, now) {
					continue
				}
				key := strings.ToLower(strings.TrimSpace(entry.Message))
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				items = append(items, entry)
			}
		}
		sshPath, _ := systemLogPath("ssh")
		ssh, err := readSystemLog(sshPath, "ssh", 0, 100)
		if err == nil {
			for i := len(ssh.Items) - 1; i >= 0 && len(items) < 50; i-- {
				if actionableSystemWarning(ssh.Items[i], now) {
					items = append(items, ssh.Items[i])
				}
			}
		}
		var failedCleanup int
		if err := a.db.QueryRowContext(r.Context(), `SELECT count(*)::int FROM portal.event_inbox WHERE event_type='portal.file.cleanup' AND status='failed'`).Scan(&failedCleanup); err != nil && !errors.Is(err, sql.ErrNoRows) {
			a.dbError(w, err, "无法读取系统事件状态。")
			return
		}
		// Surface every failed event family, not just file cleanup. Group by type so
		// one broken event does not flood the warning list with duplicate rows.
		rows, err := a.db.QueryContext(r.Context(), `SELECT event_type, count(*)::int FROM portal.event_inbox WHERE status='failed' GROUP BY event_type ORDER BY count(*) DESC, event_type LIMIT 20`)
		if err != nil {
			a.dbError(w, err, "无法读取系统事件状态。")
			return
		}
		for rows.Next() {
			var eventType string
			var count int
			if err := rows.Scan(&eventType, &count); err != nil {
				rows.Close()
				a.dbError(w, err, "无法读取系统事件状态。")
				return
			}
			items = append(items, systemLogEntry{Source: "server", Level: "error", Time: now.Format(time.RFC3339), Action: "event_dead_lettered", Message: fmt.Sprintf("事件类型 %s 有 %d 个处理失败的事件，需要核验。", eventType, count), Details: map[string]any{"eventType": eventType, "count": count}})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			a.dbError(w, err, "无法读取系统事件状态。")
			return
		}
		rows.Close()
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "failedFileCleanupEvents": failedCleanup, "checkedAt": now.Format(time.RFC3339)})
	case r.URL.Path == "/system-ops/stream" && r.Method == http.MethodGet:
		a.streamSystemLog(w, r)
	default:
		writeError(w, http.StatusNotFound, "未找到系统运维接口。", nil)
	}
}

func (a *portalAPI) streamSystemLog(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	path, ok := systemLogPath(source)
	if !ok {
		writeError(w, http.StatusBadRequest, "日志来源不正确。", nil)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "当前服务不支持实时日志流。", nil)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		if parsed, err := strconv.ParseInt(lastID, 10, 64); err == nil {
			after = parsed
		}
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	flusher.Flush()
	if after <= 0 {
		initial, err := readSystemLog(path, source, 0, 100)
		if err == nil {
			for _, item := range initial.Items {
				if err := writeSSE(w, item); err != nil {
					return
				}
			}
			after = initial.Cursor
			flusher.Flush()
		}
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			result, err := readSystemLog(path, source, after, 100)
			if err != nil {
				continue
			}
			for _, item := range result.Items {
				if err := writeSSE(w, item); err != nil {
					return
				}
				after = item.ID
			}
			if result.Cursor > after || len(result.Items) == 0 {
				after = result.Cursor
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeSSE(w io.Writer, item systemLogEntry) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(w, "id: %d\nevent: log\ndata: %s\n\n", item.ID, data); err != nil {
		return err
	}
	return nil
}
