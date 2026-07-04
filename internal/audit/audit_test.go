package audit

import "testing"

func TestStoreAppendAuditEvent(t *testing.T) {
	store := NewMemoryStore()

	event, err := store.Append(EventInput{
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		ActorID:       "usr_1",
		Action:        "billing.settled",
		TargetKind:    "workspace",
		TargetID:      "ws_1",
		SourceEventID: "billing_tick_1",
		Payload:       map[string]any{"amountCents": int64(47)},
	})
	if err != nil {
		t.Fatalf("append audit event: %v", err)
	}
	if event.ID == "" {
		t.Fatalf("expected generated id")
	}
	if event.Action != "billing.settled" || event.TargetKind != "workspace" {
		t.Fatalf("event = %+v", event)
	}
	if event.Payload["amountCents"] != int64(47) {
		t.Fatalf("payload = %+v", event.Payload)
	}
}

func TestStoreListAuditEventsByFilters(t *testing.T) {
	store := NewMemoryStore()
	events := []EventInput{
		{AccountID: "acct_1", WorkspaceID: "ws_1", Action: "billing.settled", TargetKind: "workspace", SourceEventID: "billing_tick_1"},
		{AccountID: "acct_1", WorkspaceID: "ws_2", Action: "billing.settled", TargetKind: "workspace", SourceEventID: "billing_tick_2"},
		{AccountID: "acct_2", WorkspaceID: "ws_1", Action: "account.credit_granted", TargetKind: "manual_topup", SourceEventID: "topup_1"},
	}
	for _, input := range events {
		if _, err := store.Append(input); err != nil {
			t.Fatalf("append audit event: %v", err)
		}
	}

	byAccount := store.List(EventFilter{AccountID: "acct_1"})
	if len(byAccount) != 2 {
		t.Fatalf("by account = %+v", byAccount)
	}
	byWorkspace := store.List(EventFilter{WorkspaceID: "ws_1"})
	if len(byWorkspace) != 2 {
		t.Fatalf("by workspace = %+v", byWorkspace)
	}
	byAction := store.List(EventFilter{Action: "account.credit_granted"})
	if len(byAction) != 1 || byAction[0].AccountID != "acct_2" {
		t.Fatalf("by action = %+v", byAction)
	}
	bySource := store.List(EventFilter{SourceEventID: "billing_tick_2"})
	if len(bySource) != 1 || bySource[0].WorkspaceID != "ws_2" {
		t.Fatalf("by source = %+v", bySource)
	}
}

func TestAppendAuditEventRequiresActionAndTargetKind(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Append(EventInput{TargetKind: "workspace"}); err == nil {
		t.Fatalf("expected action error")
	}
	if _, err := store.Append(EventInput{Action: "billing.settled"}); err == nil {
		t.Fatalf("expected target kind error")
	}
}
