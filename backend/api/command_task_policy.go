package main

import (
	"path/filepath"
	"strings"
)

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validTaskClass(class systemTaskClass) bool {
	switch class {
	case taskReadOnly, taskFileInspection, taskTest, taskBuild, taskCodeChange, taskSystemAdmin, taskDeploy, taskRestart, taskDatabaseChange, taskDestructive, taskSSHMCP:
		return true
	default:
		return false
	}
}

func allowedTaskWorkingDirectory(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return false
	}
	for _, root := range []string{"/opt/personal-portal", "/var/tmp", "/var/log/personal-portal"} {
		rel, err := filepath.Rel(root, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func stableTaskIsSafe(req commandTaskRequest) bool {
	if req.ShellMode || !allowedTaskWorkingDirectory(req.WorkingDir) {
		return false
	}
	base := filepath.Base(req.Executable)
	trustedExecutable := map[string][]string{
		"uname": {"/usr/bin/uname"}, "df": {"/bin/df"}, "freebsd-version": {"/bin/freebsd-version"},
		"uptime": {"/usr/bin/uptime"}, "hostname": {"/bin/hostname"}, "id": {"/usr/bin/id"}, "whoami": {"/usr/bin/whoami"},
		"git": {"/usr/local/bin/git"}, "go127": {"/usr/local/bin/go127"}, "npm": {"/usr/local/bin/npm"}, "make": {"/usr/bin/make"},
		"ls": {"/bin/ls"}, "cat": {"/bin/cat"}, "head": {"/usr/bin/head"}, "tail": {"/usr/bin/tail"},
		"grep": {"/usr/bin/grep"}, "find": {"/usr/bin/find"}, "stat": {"/usr/bin/stat"}, "file": {"/usr/bin/file"},
		"wc": {"/usr/bin/wc"},
	}
	trusted := false
	for _, path := range trustedExecutable[base] {
		if req.Executable == path {
			trusted = true
			break
		}
	}
	if !trusted {
		return false
	}
	for _, arg := range req.Arguments {
		if strings.ContainsRune(arg, 0) || strings.Contains(arg, ".."+string(filepath.Separator)) || strings.Contains(arg, "/etc/") || strings.Contains(arg, "/root/") || strings.Contains(arg, "/var/backups/") || strings.Contains(arg, "/usr/local/etc/") {
			return false
		}
	}
	switch req.Class {
	case taskReadOnly:
		switch base {
		case "uname", "df", "freebsd-version", "uptime", "hostname", "id", "whoami", "pwd", "git", "go127", "npm", "make":
			if base == "git" {
				return len(req.Arguments) > 0 && (req.Arguments[0] == "status" || req.Arguments[0] == "log" || req.Arguments[0] == "diff")
			}
			if base == "go127" {
				return len(req.Arguments) > 0 && req.Arguments[0] == "version"
			}
			if base == "npm" {
				return len(req.Arguments) > 0 && (req.Arguments[0] == "--version" || req.Arguments[0] == "version")
			}
			if base == "make" {
				return len(req.Arguments) > 0 && req.Arguments[0] == "-n"
			}
			return true
		default:
			return false
		}
	case taskFileInspection:
		switch base {
		case "ls", "cat", "head", "tail", "grep", "stat", "file", "wc":
			return true
		case "find":
			for _, arg := range req.Arguments {
				if arg == "-delete" || arg == "-exec" || arg == "-execdir" || arg == "-ok" || arg == "-okdir" || strings.HasPrefix(arg, "-fprint") {
					return false
				}
			}
			return true
		default:
			return false
		}
	case taskTest:
		return (base == "go127" && len(req.Arguments) >= 2 && req.Arguments[0] == "test" && req.Arguments[1] == "./...") ||
			(base == "npm" && len(req.Arguments) == 1 && req.Arguments[0] == "test") ||
			(base == "make" && len(req.Arguments) == 1 && req.Arguments[0] == "test")
	case taskBuild:
		if base == "npm" {
			return len(req.Arguments) == 2 && req.Arguments[0] == "run" && req.Arguments[1] == "build"
		}
		if base == "make" {
			hasTarget, hasAll := false, false
			for _, arg := range req.Arguments {
				if strings.HasPrefix(arg, "TARGET=/var/tmp/personal-portal-task-build-") {
					hasTarget = true
				}
				if arg == "all" {
					hasAll = true
				}
			}
			return hasTarget && hasAll
		}
		if base == "go127" && len(req.Arguments) >= 3 && req.Arguments[0] == "build" {
			for i, arg := range req.Arguments {
				if arg == "-o" && i+1 < len(req.Arguments) && strings.HasPrefix(req.Arguments[i+1], "/var/tmp/personal-portal-task-build-") {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func isSSHMCPCommand(executable string, args []string) bool {
	value := strings.ToLower(executable + " " + strings.Join(args, " "))
	for _, marker := range []string{"ssh_mcp", "ssh-mcp", "host-mcp", "host_mcp", "ssh mcp"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func mandatoryTaskClass(req commandTaskRequest) systemTaskClass {
	if req.Class == taskSSHMCP || isSSHMCPCommand(req.Executable, req.Arguments) {
		return taskSSHMCP
	}
	base := strings.ToLower(filepath.Base(req.Executable))
	if base == "rm" || base == "unlink" || base == "rmdir" || base == "shred" {
		return taskDestructive
	}
	if base == "psql" || base == "createdb" || base == "dropdb" || base == "pg_restore" || base == "pg_dump" || base == "createuser" || base == "dropuser" {
		return taskDatabaseChange
	}
	if base == "chmod" || base == "chown" || base == "sysrc" || base == "zfs" || base == "zpool" || base == "pkg" || base == "mount" || base == "umount" {
		return taskSystemAdmin
	}
	if base == "git" && len(req.Arguments) > 0 {
		switch strings.ToLower(req.Arguments[0]) {
		case "clean", "reset":
			return taskDestructive
		case "apply", "checkout", "switch", "restore", "commit", "push", "merge", "rebase":
			return taskCodeChange
		}
	}
	if base == "npm" && len(req.Arguments) > 0 && (req.Arguments[0] == "install" || req.Arguments[0] == "ci" || req.Arguments[0] == "uninstall" || req.Arguments[0] == "publish") {
		return taskCodeChange
	}
	if base == "make" {
		for _, arg := range req.Arguments {
			if arg == "clean" {
				return taskDestructive
			}
		}
	}
	if base == "service" || base == "rcctl" || base == "systemctl" || base == "launchctl" || base == "shutdown" || base == "reboot" || base == "kill" || base == "pkill" || base == "killall" || strings.HasPrefix(req.Executable, "/usr/local/etc/rc.d/") {
		for _, arg := range req.Arguments {
			lower := strings.ToLower(arg)
			switch lower {
			case "start", "stop", "restart", "reload", "force-reload", "reboot", "shutdown", "-9", "-kill", "-term":
				return taskRestart
			}
			if strings.HasSuffix(lower, "start") || strings.HasSuffix(lower, "stop") || strings.HasSuffix(lower, "restart") || strings.HasSuffix(lower, "reload") {
				return taskRestart
			}
		}
		if base == "shutdown" || base == "reboot" || base == "kill" || base == "pkill" || base == "killall" {
			return taskRestart
		}
	}
	for _, arg := range req.Arguments {
		if strings.Contains(strings.ToLower(arg), "ssh_mcp") || strings.Contains(strings.ToLower(arg), "ssh-mcp") || strings.Contains(strings.ToLower(arg), "host-mcp") || strings.Contains(strings.ToLower(arg), "host_mcp") {
			return taskSSHMCP
		}
	}
	return req.Class
}

func stableApprovedTaskAllowed(req commandTaskRequest) bool {
	if req.ShellMode || !allowedTaskWorkingDirectory(req.WorkingDir) {
		return false
	}
	switch req.Class {
	case taskCodeChange:
		if req.Executable != "/usr/local/bin/git" || req.WorkingDir != "/opt/personal-portal" || len(req.Arguments) < 2 || req.Arguments[0] != "apply" {
			return false
		}
		for _, arg := range req.Arguments {
			if arg == "--unsafe-paths" || strings.Contains(arg, ".."+string(filepath.Separator)) || strings.Contains(arg, "/etc/") || strings.Contains(arg, "/root/") || strings.Contains(arg, "/var/backups/") || strings.Contains(arg, "/usr/local/etc/") {
				return false
			}
		}
		return true
	case taskSystemAdmin:
		switch req.Executable {
		case "/usr/sbin/service", "/usr/sbin/sysrc", "/sbin/zfs", "/sbin/zpool", "/usr/sbin/pkg", "/sbin/mount", "/sbin/umount", "/usr/sbin/chown", "/bin/chmod", "/usr/bin/chmod", "/usr/sbin/pw":
			return true
		}
		return false
	case taskDeploy:
		return req.Executable == "/opt/personal-portal/deploy/freebsd/deploy.sh" && req.WorkingDir == "/opt/personal-portal" && len(req.Arguments) == 0
	case taskRestart:
		return req.Executable == "/usr/sbin/service" && len(req.Arguments) == 2 && (req.Arguments[1] == "restart" || req.Arguments[1] == "start" || req.Arguments[1] == "stop" || req.Arguments[1] == "reload") && !isSSHMCPCommand(req.Executable, req.Arguments)
	case taskDatabaseChange:
		switch req.Executable {
		case "/usr/local/bin/psql", "/usr/local/bin/createdb", "/usr/local/bin/dropdb", "/usr/local/bin/pg_restore", "/usr/local/bin/pg_dump", "/usr/local/bin/createuser", "/usr/local/bin/dropuser":
			return true
		}
		return false
	case taskDestructive:
		switch req.Executable {
		case "/bin/rm", "/bin/rmdir", "/usr/bin/unlink", "/bin/unlink", "/usr/bin/chflags":
			return true
		}
		return false
	default:
		return false
	}
}
