package evidence

import "testing"

func TestNewRecordRequiresWorkspaceLifecycleFields(t *testing.T) {
	_, err := NewRecord(RecordInput{
		Type:        "workspace.created",
		AccountID:   "acct_1",
		WorkspaceID: "ws_1",
		Actor:       map[string]any{"type": "user", "id": "usr_1"},
		Plan:        map[string]any{"workspaceName": "Lab"},
		Approval:    map[string]any{"status": "implicit_console_policy"},
		Environment: map[string]any{"runtimeProvider": "tencent-tke"},
		ResourceRefs: map[string]any{
			"serverId":  "ins_1",
			"storageId": "disk_1",
		},
		BillingRefs: []map[string]any{
			{"id": "hold_1", "type": "compute_hold"},
		},
	})
	if err != nil {
		t.Fatalf("NewRecord returned error: %v", err)
	}
}

func TestNewRecordRejectsMissingRequiredFields(t *testing.T) {
	tests := []struct {
		name  string
		input RecordInput
		err   string
	}{
		{
			name: "type",
			input: RecordInput{
				AccountID:   "acct_1",
				WorkspaceID: "ws_1",
				Plan:        map[string]any{"workspaceName": "Lab"},
				Approval:    map[string]any{"status": "implicit_console_policy"},
				Environment: map[string]any{"runtimeProvider": "tencent-tke"},
			},
			err: "evidence_type_required",
		},
		{
			name: "account",
			input: RecordInput{
				Type:        "workspace.created",
				WorkspaceID: "ws_1",
				Plan:        map[string]any{"workspaceName": "Lab"},
				Approval:    map[string]any{"status": "implicit_console_policy"},
				Environment: map[string]any{"runtimeProvider": "tencent-tke"},
			},
			err: "evidence_account_required",
		},
		{
			name: "workspace",
			input: RecordInput{
				Type:        "workspace.created",
				AccountID:   "acct_1",
				Plan:        map[string]any{"workspaceName": "Lab"},
				Approval:    map[string]any{"status": "implicit_console_policy"},
				Environment: map[string]any{"runtimeProvider": "tencent-tke"},
			},
			err: "evidence_workspace_required",
		},
		{
			name: "plan",
			input: RecordInput{
				Type:        "workspace.created",
				AccountID:   "acct_1",
				WorkspaceID: "ws_1",
				Approval:    map[string]any{"status": "implicit_console_policy"},
				Environment: map[string]any{"runtimeProvider": "tencent-tke"},
			},
			err: "evidence_plan_required",
		},
		{
			name: "approval",
			input: RecordInput{
				Type:        "workspace.created",
				AccountID:   "acct_1",
				WorkspaceID: "ws_1",
				Plan:        map[string]any{"workspaceName": "Lab"},
				Environment: map[string]any{"runtimeProvider": "tencent-tke"},
			},
			err: "evidence_approval_required",
		},
		{
			name: "environment",
			input: RecordInput{
				Type:        "workspace.created",
				AccountID:   "acct_1",
				WorkspaceID: "ws_1",
				Plan:        map[string]any{"workspaceName": "Lab"},
				Approval:    map[string]any{"status": "implicit_console_policy"},
			},
			err: "evidence_environment_required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRecord(tt.input)
			if err == nil || err.Error() != tt.err {
				t.Fatalf("error = %v, want %s", err, tt.err)
			}
		})
	}
}

func TestMatchesFiltersByAccountWorkspaceTypeAndSourceEvent(t *testing.T) {
	record, err := NewRecord(RecordInput{
		Type:          "workspace.storage_backup_created",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		SourceEventID: "backup_1",
		Plan:          map[string]any{"backup": "daily"},
		Approval:      map[string]any{"status": "approved"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
	})
	if err != nil {
		t.Fatalf("NewRecord returned error: %v", err)
	}

	if !Matches(record, RecordFilter{AccountID: "acct_1", WorkspaceID: "ws_1", Type: "workspace.storage_backup_created", SourceEventID: "backup_1"}) {
		t.Fatalf("expected filter to match record")
	}
	if Matches(record, RecordFilter{WorkspaceID: "ws_2"}) {
		t.Fatalf("expected workspace mismatch not to match")
	}
}
