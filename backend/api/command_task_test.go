package main

import (
	"bytes"
	"testing"
)

func TestStableTaskPolicyOnlyAcceptsTrustedExecutableAndBoundedActions(t *testing.T) {
	cases := []struct {
		name string
		req  commandTaskRequest
		want bool
	}{
		{"trusted read-only diagnostic", commandTaskRequest{Class: taskReadOnly, Executable: "/usr/bin/uname", Arguments: []string{"-a"}, WorkingDir: "/var/tmp"}, true},
		{"untrusted binary with trusted basename", commandTaskRequest{Class: taskReadOnly, Executable: "/tmp/uname", Arguments: []string{"-a"}, WorkingDir: "/var/tmp"}, false},
		{"shell mode is rejected", commandTaskRequest{Class: taskReadOnly, Executable: "/bin/sh", Arguments: []string{"-c", "id"}, WorkingDir: "/var/tmp", ShellMode: true}, false},
		{"system path is rejected", commandTaskRequest{Class: taskFileInspection, Executable: "/bin/cat", Arguments: []string{"/etc/passwd"}, WorkingDir: "/var/tmp"}, false},
		{"registered test command", commandTaskRequest{Class: taskTest, Executable: "/usr/local/bin/go127", Arguments: []string{"test", "./..."}, WorkingDir: "/var/tmp"}, true},
		{"unregistered test command", commandTaskRequest{Class: taskTest, Executable: "/usr/local/bin/go127", Arguments: []string{"run", "script.go"}, WorkingDir: "/var/tmp"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stableTaskIsSafe(tc.req); got != tc.want {
				t.Fatalf("stableTaskIsSafe() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMandatoryTaskClassificationCannotDowngradeRestartOrSSHMCPOperations(t *testing.T) {
	cases := []struct {
		name string
		req  commandTaskRequest
		want systemTaskClass
	}{
		{"service restart", commandTaskRequest{Class: taskReadOnly, Executable: "/usr/sbin/service", Arguments: []string{"personal_portal_api", "restart"}}, taskRestart},
		{"process kill", commandTaskRequest{Class: taskReadOnly, Executable: "/bin/kill", Arguments: []string{"1234"}}, taskRestart},
		{"database mutation", commandTaskRequest{Class: taskReadOnly, Executable: "/usr/local/bin/psql", Arguments: []string{"-c", "select 1"}}, taskDatabaseChange},
		{"destructive delete", commandTaskRequest{Class: taskReadOnly, Executable: "/bin/rm", Arguments: []string{"file"}}, taskDestructive},
		{"SSH MCP path", commandTaskRequest{Class: taskReadOnly, Executable: "/usr/sbin/service", Arguments: []string{"host-mcp", "restart"}}, taskSSHMCP},
		{"explicit SSH MCP class", commandTaskRequest{Class: taskSSHMCP, Executable: "/usr/bin/uname"}, taskSSHMCP},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mandatoryTaskClass(tc.req); got != tc.want {
				t.Fatalf("class = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLimitedBufferRetainsOnlyBoundedTail(t *testing.T) {
	var b bytes.Buffer
	limited := &limitedBuffer{buffer: &b, limit: 5}
	if n, err := limited.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first Write = %d, %v", n, err)
	}
	if n, err := limited.Write([]byte("defgh")); err != nil || n != 5 {
		t.Fatalf("second Write = %d, %v", n, err)
	}
	if got, want := b.String(), "defgh"; got != want {
		t.Fatalf("bounded tail = %q, want %q", got, want)
	}
	if len(b.String()) > 5 {
		t.Fatalf("buffer exceeded limit: %d", b.Len())
	}
}

func TestStableApprovedTaskAllowlistRejectsArbitraryExecutables(t *testing.T) {
	if stableApprovedTaskAllowed(commandTaskRequest{Class: taskSystemAdmin, Executable: "/usr/local/bin/python3.14", Arguments: []string{"script.py"}, WorkingDir: "/var/tmp"}) {
		t.Fatal("arbitrary interpreter should not pass the stable-mode elevation allowlist")
	}
	if stableApprovedTaskAllowed(commandTaskRequest{Class: taskRestart, Executable: "/usr/sbin/service", Arguments: []string{"host-mcp", "restart"}, WorkingDir: "/var/tmp"}) {
		t.Fatal("SSH_MCP restart must remain manual-only")
	}
	if !stableApprovedTaskAllowed(commandTaskRequest{Class: taskRestart, Executable: "/usr/sbin/service", Arguments: []string{"personal_portal_api", "restart"}, WorkingDir: "/var/tmp"}) {
		t.Fatal("explicitly approved non-SSH service restart should be permitted")
	}
	if !stableApprovedTaskAllowed(commandTaskRequest{Class: taskSystemAdmin, Executable: "/usr/sbin/sysrc", Arguments: []string{"personal_portal_api_enable=YES"}, WorkingDir: "/var/tmp"}) {
		t.Fatal("explicitly approved allowlisted system administration command should be permitted")
	}
}
