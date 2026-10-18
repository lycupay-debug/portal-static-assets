package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type systemLogEntry struct {
	ID      int64          `json:"id"`
	Time    string         `json:"time"`
	Source  string         `json:"source"`
	Level   string         `json:"level"`
	Action  string         `json:"action,omitempty"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type logReadResult struct {
	Items  []systemLogEntry `json:"items"`
	Cursor int64            `json:"cursor"`
}

func (a *portalAPI) requireSystemOpsAdmin(w http.ResponseWriter, r *http.Request) bool {
	u, ok := a.requireUser(w, r)
	if !ok {
		return false
	}
	var isAdmin bool
	if err := a.db.QueryRowContext(r.Context(), `SELECT is_admin FROM portal.portal_users WHERE user_id=$1::uuid`, u.ID).Scan(&isAdmin); err != nil {
		a.dbError(w, err, "无法验证系统运维权限。")
		return false
	}
	if !isAdmin {
		writeError(w, http.StatusForbidden, "只有管理员可以查看系统运维信息。", nil)
		return false
	}
	return true
}

func systemLogPath(source string) (string, bool) {
	switch source {
	case "server":
		return "/var/log/messages", true
	case "ssh":
		return "/var/log/host-mcp-admin-audit.log", true
	default:
		return "", false
	}
}

func parseSystemLogLine(source string, id int64, line string) systemLogEntry {
	entry := systemLogEntry{ID: id, Source: source, Level: "info", Message: strings.TrimSpace(line)}
	if source == "ssh" {
		var raw struct {
			Time    string         `json:"time"`
			Action  string         `json:"action"`
			Outcome string         `json:"outcome"`
			Details map[string]any `json:"details"`
		}
		if json.Unmarshal([]byte(line), &raw) == nil {
			entry.Time, entry.Action, entry.Details = raw.Time, raw.Action, raw.Details
			entry.Message = raw.Action
			if raw.Outcome != "" {
				entry.Message += " · " + raw.Outcome
				switch raw.Outcome {
				case "failed":
					entry.Level = "error"
				case "timeout":
					entry.Level = "warning"
				}
			}
			if raw.Details != nil {
				if rc, ok := raw.Details["returncode"].(float64); ok && rc != 0 {
					entry.Level = "error"
				}
				if timedOut, ok := raw.Details["timed_out"].(bool); ok && timedOut {
					entry.Level = "warning"
				}
				if stage, ok := raw.Details["stage"].(string); ok && stage == "started" {
					entry.Level = "info"
				}
			}
		}
	} else {
		if len(line) >= 15 {
			entry.Time = line[:15]
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "panic") || strings.Contains(lower, "critical") || strings.Contains(lower, "fatal") || strings.Contains(lower, " error") {
			entry.Level = "error"
		} else if strings.Contains(lower, "warning") || strings.Contains(lower, " warn") || strings.Contains(lower, "failed") || strings.Contains(lower, "failure") {
			entry.Level = "warning"
		}
	}
	return entry
}

// readSystemLog reads only a bounded tail for a new viewer and only appended bytes for a live cursor.
func readSystemLog(path, source string, after int64, limit int) (logReadResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return logReadResult{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return logReadResult{}, err
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	size := st.Size()
	initial := after <= 0 || after > size
	if initial {
		start := size - 256*1024
		if start < 0 {
			start = 0
		}
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			return logReadResult{}, err
		}
		cursor := start
		reader := bufio.NewReaderSize(f, 32*1024)
		if start > 0 {
			partial, e := reader.ReadString('\n')
			cursor += int64(len(partial))
			if e != nil && e != io.EOF {
				return logReadResult{}, e
			}
		}
		data, err := io.ReadAll(io.LimitReader(reader, 256*1024))
		if err != nil {
			return logReadResult{}, err
		}
		items := make([]systemLogEntry, 0, limit)
		for len(data) > 0 {
			newline := strings.IndexByte(string(data), '\n')
			if newline < 0 {
				break
			} // Ignore an unfinished final line until its newline arrives.
			line := string(data[:newline])
			data = data[newline+1:]
			cursor += int64(newline + 1)
			items = append(items, parseSystemLogLine(source, cursor, line))
			if len(items) > limit {
				items = items[1:]
			}
		}
		return logReadResult{Items: items, Cursor: cursor}, nil
	}
	if _, err := f.Seek(after, io.SeekStart); err != nil {
		return logReadResult{}, err
	}
	reader := bufio.NewReaderSize(f, 32*1024)
	cursor := after
	items := make([]systemLogEntry, 0, limit)
	for len(items) < limit {
		line, e := reader.ReadString('\n')
		if e != nil && e != io.EOF {
			return logReadResult{}, e
		}
		if e == io.EOF {
			break
		}
		cursor += int64(len(line))
		items = append(items, parseSystemLogLine(source, cursor, strings.TrimSuffix(line, "\n")))
	}
	return logReadResult{Items: items, Cursor: cursor}, nil
}

// actionableSystemWarning filters historical noise and test/diagnostic failures out of
// the warning center. Full source logs remain available on the logs page for audit.
func actionableSystemWarning(entry systemLogEntry, now time.Time) bool {
	if entry.Level != "error" && entry.Level != "warning" {
		return false
	}
	if entry.Source == "ssh" {
		// Read-only shell queries and blocked reads are useful audit entries, but are
		// not service-health incidents. Only failed state-changing operations alert.
		switch entry.Action {
		case "write_file", "replace_in_file", "move_path", "delete_path":
			return entry.Level == "error"
		default:
			return false
		}
	}
	if entry.Source != "server" {
		return false
	}
	lower := strings.ToLower(entry.Message)
	// These are retired schema references, test fixtures, diagnostic probes, or
	// optional host services unrelated to portal availability. Keep them in logs.
	if strings.Contains(lower, "uploaded_image_assets") && strings.Contains(lower, "does not exist") {
		return false
	}
	for _, marker := range []string{
		"permission denied for function pg_ls_dir",
		"go api integration",
		"test.processor.",
		"mit test license text",
		"rc.conf(5)",
	} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	// A warning should represent a current incident, not an old log line replayed
	// from the bounded tail on every page refresh.
	if entry.Time != "" {
		t, err := time.ParseInLocation("Jan _2 15:04:05", entry.Time, time.Local)
		if err == nil {
			t = t.AddDate(now.Year()-t.Year(), 0, 0)
			if t.After(now.Add(5 * time.Minute)) {
				t = t.AddDate(-1, 0, 0)
			}
			if now.Sub(t) > time.Hour || t.After(now.Add(5*time.Minute)) {
				return false
			}
		}
	}
	return true
}

func (a *portalAPI) systemOps(w http.ResponseWriter, r *http.Request) {
	if !a.requireSystemOpsAdmin(w, r) {
		return
	}
	switch {
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
