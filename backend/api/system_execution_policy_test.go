package main

import "testing"

func TestSystemTaskPolicyKeepsHardApprovalGatesInBothModes(t *testing.T) {
	for _, mode := range []systemExecutionMode{systemModeDeveloper, systemModeStable} {
		for _, class := range []systemTaskClass{taskRestart, taskSSHMCP} {
			got := decideSystemTaskPolicy(mode, class)
			if got.Decision != "require_approval" {
				t.Errorf("mode=%s class=%s decision=%q, want require_approval", mode, class, got.Decision)
			}
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
