package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
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
			toolSource, _ := raw.Details["tool_source"].(string)
			if toolSource == "Go SSH MCP" {
				operation, _ := raw.Details["operation"].(string)
				entry.Action = goSSHLogAction(operation, raw.Action, raw.Outcome)
				entry.Message = entry.Action
				if raw.Outcome != "" {
					entry.Message += " · " + raw.Outcome
				}
			}
			if raw.Outcome != "" {
				if toolSource != "Go SSH MCP" {
					entry.Message += " · " + raw.Outcome
				}
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
		entry.Time = parseServerLogTime(line)
		lower := strings.ToLower(line)
		if strings.Contains(lower, "panic") || strings.Contains(lower, "critical") || strings.Contains(lower, "fatal") || strings.Contains(lower, " error") {
			entry.Level = "error"
		} else if strings.Contains(lower, "warning") || strings.Contains(lower, " warn") || strings.Contains(lower, "failed") || strings.Contains(lower, "failure") {
			entry.Level = "warning"
		}
	}
	return entry
}

func goSSHLogAction(operation, eventType, outcome string) string {
	labels := map[string]string{
		"shell.exec":       "Shell 命令",
		"file.read":        "读取文件",
		"file.write":       "写入文件",
		"file.replace":     "修改文件",
		"file.move":        "移动路径",
		"file.delete":      "删除路径",
		"directory.list":   "列出目录",
		"system.info":      "读取系统信息",
		"service.status":   "查询服务状态",
		"logs.tail":        "读取服务日志",
		"database.query":   "数据库查询",
		"execute_parallel": "并行执行任务",
	}
	label := labels[operation]
	if label == "" {
		label = "执行工具操作"
	}
	var title string
	switch {
	case strings.Contains(eventType, "started"):
		title = "执行 " + label
	case outcome == "timeout":
		title = label + "执行超时"
	case outcome == "failed" || outcome == "blocked" || outcome == "unknown":
		title = label + "执行失败"
	case outcome == "success" && operation == "shell.exec":
		title = "Shell 命令执行结果"
	case outcome == "success":
		title = label
	case eventType == "task.queued" || eventType == "batch.accepted":
		title = "已接收任务 · " + label
	default:
		title = label
	}
	return "Go SSH MCP · " + title
}

// parseServerLogTime returns ISO timestamps so browsers do not guess the year
// (which makes syslog entries appear as 2001) or apply the viewer's timezone.
// PostgreSQL embeds an explicit UTC timestamp in its messages; prefer it over
// the syslog prefix. Other syslog records use the host's configured local zone.
func parseServerLogTime(line string) string {
	return parseServerLogTimeWithCutover(line, "/var/db/system-log-timezone-cutover")
}

func parseServerLogTimeWithCutover(line, cutoverPath string) string {
	if utcAt := strings.Index(line, " UTC"); utcAt > 0 {
		prefix := line[:utcAt]
		for i := 0; i+19 <= len(prefix); i++ {
			if i+10 >= len(prefix) || prefix[i+4] != '-' || prefix[i+7] != '-' {
				continue
			}
			stamp := strings.TrimSpace(prefix[i:])
			for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
				if parsed, err := time.Parse(layout, stamp); err == nil {
					return parsed.UTC().In(time.Local).Format(time.RFC3339)
				}
			}
		}
	}
	if len(line) >= 15 {
		if parsed, err := time.ParseInLocation("Jan _2 15:04:05", line[:15], time.Local); err == nil {
			now := time.Now()
			parsed = time.Date(now.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.Local)
			if parsed.After(now.Add(24 * time.Hour)) {
				parsed = parsed.AddDate(-1, 0, 0)
			}
			// Logs written before the host timezone was changed were stamped in UTC.
			// The cutover marker lets us convert those legacy prefixes without
			// shifting new Asia/Shanghai syslog timestamps a second time.
			if raw, readErr := os.ReadFile(cutoverPath); readErr == nil {
				if cutover, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(string(raw))); parseErr == nil && parsed.Before(cutover) {
					parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), 0, time.UTC).In(time.Local)
				}
			}
			return parsed.Format(time.RFC3339)
		}
	}
	return ""
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

func systemWarningIsRecent(entry systemLogEntry, now time.Time) bool {
	if entry.Time == "" {
		return false
	}
	var t time.Time
	var err error
	switch {
	case strings.Contains(entry.Time, "T") && strings.HasSuffix(entry.Time, "Z"):
		t, err = time.Parse(time.RFC3339, entry.Time)
	case strings.Contains(entry.Time, "T") && len(entry.Time) >= 5 && (entry.Time[len(entry.Time)-5] == '+' || entry.Time[len(entry.Time)-5] == '-') && !strings.Contains(entry.Time[len(entry.Time)-5:], ":"):
		t, err = time.Parse("2006-01-02T15:04:05-0700", entry.Time)
	case strings.Contains(entry.Time, "T"):
		t, err = time.Parse(time.RFC3339, entry.Time)
	default:
		t, err = time.ParseInLocation("Jan _2 15:04:05", entry.Time, time.Local)
		if err == nil {
			t = t.AddDate(now.Year()-t.Year(), 0, 0)
			if t.After(now.Add(5 * time.Minute)) {
				t = t.AddDate(-1, 0, 0)
			}
		}
	}
	if err != nil {
		return false
	}
	age := now.Sub(t)
	return age >= -5*time.Minute && age <= time.Hour
}

// actionableSystemWarning filters historical noise and test/diagnostic failures out of
// the warning center. Full source logs remain available on the logs page for audit.
func actionableSystemWarning(entry systemLogEntry, now time.Time) bool {
	if entry.Level != "error" && entry.Level != "warning" {
		return false
	}
	if !systemWarningIsRecent(entry, now) {
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
	return true
}
