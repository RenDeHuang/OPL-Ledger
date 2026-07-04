package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
)

func TestEvidenceRecordAPIPostsAndQueriesWorkspaceLifecycleEvidence(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"type":"workspace.created",
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"sourceEventId":"workspace_create_1",
		"actor":{"type":"user","id":"usr_1"},
		"plan":{"workspaceName":"Lab","packageId":"basic","computeProfile":"basic","storageGb":50},
		"approval":{"status":"implicit_console_policy"},
		"environment":{"runtimeProvider":"tencent-tke","workspaceImage":"opl/workspace:latest"},
		"resourceRefs":{"serverId":"ins_1","storageId":"disk_1"},
		"billingRefs":[{"id":"hold_1","type":"compute_hold","amountCents":1200,"currency":"CNY"}],
		"continuation":{"type":"open_workspace_url"}
	}`)
	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/evidence-records", bytes.NewReader(body)))
	if post.Code != http.StatusCreated {
		t.Fatalf("post evidence status = %d body=%s", post.Code, post.Body.String())
	}
	var record ledger.EvidenceRecord
	if err := json.Unmarshal(post.Body.Bytes(), &record); err != nil {
		t.Fatalf("decode evidence record: %v", err)
	}
	if record.ID == "" || record.Type != "workspace.created" || record.SourceEventID != "workspace_create_1" {
		t.Fatalf("record = %+v", record)
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/evidence-records?accountId=acct_1&workspaceId=ws_1&type=workspace.created&sourceEventId=workspace_create_1", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get evidence status = %d body=%s", get.Code, get.Body.String())
	}
	var records []ledger.EvidenceRecord
	if err := json.Unmarshal(get.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode evidence records: %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("records = %+v", records)
	}
}

func TestEvidenceRecordAPIDoesNotAppendBillingLedgerEntry(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"type":"workspace.storage_backup_created",
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"sourceEventId":"backup_1",
		"plan":{"backup":"daily"},
		"approval":{"status":"approved"},
		"environment":{"runtimeProvider":"tencent-tke"}
	}`)
	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/evidence-records", bytes.NewReader(body)))
	if post.Code != http.StatusCreated {
		t.Fatalf("post evidence status = %d body=%s", post.Code, post.Body.String())
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/entries?accountId=acct_1&workspaceId=ws_1&sourceEventId=backup_1", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get ledger entries status = %d body=%s", get.Code, get.Body.String())
	}
	var entries []ledger.Entry
	if err := json.Unmarshal(get.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode ledger entries: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("evidence should not create billing ledger entries: %+v", entries)
	}
}
