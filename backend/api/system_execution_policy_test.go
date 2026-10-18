package main

import "testing"

func TestSystemTaskPolicyKeepsRestartApprovalAndSSHMCPManualOnlyInBothModes(t *testing.T) {
	for _, mode := range []systemExecutionMode{systemModeDeveloper, systemModeStable} {
		if got := decideSystemTaskPolicy(mode, taskRestart); got.Decision != "require_approval" {
			t.Errorf("mode=%s restart decision=%q, want require_approval", mode, got.Decision)
		}
		if got := decideSystemTaskPolicy(mode, taskSSHMCP); got.Decision != "manual_only" {
			t.Errorf("mode=%s SSH_MCP decision=%q, want manual_only", mode, got.Decision)
		}
	}
}

func TestSystemTaskPolicyDeveloperModeAllowsAuthorizedScopeForOtherKnownTasks(t *testing.T) {
	classes := []systemTaskClass{taskReadOnly, taskFileInspection, taskTest, taskBuild, taskCodeChange, taskSystemAdmin, taskDeploy, taskDatabaseChange, taskDestructive}
	for _, class := range classes {
		if got := decideSystemTaskPolicy(systemModeDeveloper, class); got.Decision != "allow_authorized_scope" {
			t.Errorf("class=%s decision=%q, want allow_authorized_scope", class, got.Decision)
		}
	}
}

func TestSystemTaskPolicyStableModeAppliesRiskRules(t *testing.T) {
	cases := map[systemTaskClass]string{
		taskReadOnly: "allow_read_only", taskFileInspection: "require_path_scope", taskTest: "require_registered_task",
		taskBuild: "require_workspace_scope", taskCodeChange: "require_change_scope", taskSystemAdmin: "require_operation_scope",
		taskDeploy: "require_approval", taskDatabaseChange: "require_approval", taskDestructive: "require_approval",
	}
	for class, want := range cases {
		if got := decideSystemTaskPolicy(systemModeStable, class); got.Decision != want {
			t.Errorf("class=%s decision=%q, want %q", class, got.Decision, want)
		}
	}
}

func TestSystemTaskPolicyDeniesUnknownClasses(t *testing.T) {
	if got := decideSystemTaskPolicy(systemModeStable, "unclassified"); got.Decision != "deny_unknown" {
		t.Fatalf("unknown class decision=%q, want deny_unknown", got.Decision)
	}
}
