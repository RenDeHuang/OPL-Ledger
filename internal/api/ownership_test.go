package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	"github.com/RenDeHuang/OPL-Ledger/internal/ownership"
)

type workspaceOwners map[string]string

func (w workspaceOwners) WorkspaceOwnerAccountID(_ context.Context, workspaceID string) (string, error) {
	accountID, ok := w[workspaceID]
	if !ok {
		return "", ownership.ErrWorkspaceNotFound
	}
	return accountID, nil
}

func TestTaskReceiptAPIRejectsWorkspaceOwnedByAnotherAccount(t *testing.T) {
	server := NewServerWithOwnership(ledger.NewMemoryStore(), workspaceOwners{"ws_1": "acct_2"})
	body := []byte(`{
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"taskId":"task_1",
		"sourceEventId":"task_source_1",
		"plan":{"goal":"run analysis"},
		"approval":{"status":"approved"},
		"environment":{"runtimeProvider":"tencent-tke"}
	}`)

	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/task-receipts", bytes.NewReader(body)))
	if post.Code != http.StatusNotFound {
		t.Fatalf("post receipt status = %d body=%s", post.Code, post.Body.String())
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/task-receipts?accountId=acct_1&workspaceId=ws_1&taskId=task_1", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get receipt status = %d body=%s", get.Code, get.Body.String())
	}
	if get.Body.String() != "null\n" {
		t.Fatalf("expected no receipt to be recorded, got %s", get.Body.String())
	}
}
