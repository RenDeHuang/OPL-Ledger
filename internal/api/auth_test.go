package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/auth"
	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
)

func TestAuthRejectsMissingTokenForMutatingEndpoint(t *testing.T) {
	server := NewServerWithOptions(ledger.NewMemoryStore(), Options{Auth: auth.Config{ServiceToken: "svc_1"}})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"amountCents":25000,
		"reason":"owner_credit"
	}`))))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthServiceTokenAllowsMutatingEndpoint(t *testing.T) {
	server := NewServerWithOptions(ledger.NewMemoryStore(), Options{Auth: auth.Config{ServiceToken: "svc_1"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"amountCents":25000,
		"reason":"owner_credit"
	}`)))
	req.Header.Set("Authorization", "Bearer svc_1")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthAdminTokenAllowsOperatorEvidenceRead(t *testing.T) {
	store := ledger.NewMemoryStore()
	if _, err := store.AppendAuditEvent(nil, ledger.AuditEventInput{
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		Action:        "workspace.created",
		TargetKind:    "workspace",
		TargetID:      "ws_1",
		SourceEventID: "workspace_create_1",
	}); err != nil {
		t.Fatalf("seed audit event: %v", err)
	}
	server := NewServerWithOptions(store, Options{Auth: auth.Config{AdminToken: "admin_1"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?accountId=acct_1", nil)
	req.Header.Set("Authorization", "Bearer admin_1")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var events []ledger.AuditEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v", events)
	}
}

func TestAuthRejectsMissingAdminTokenForBillingWalletReads(t *testing.T) {
	server := NewServerWithOptions(ledger.NewMemoryStore(), Options{Auth: auth.Config{AdminToken: "admin_1"}})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/wallets?accountId=acct_1", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthRejectsMissingAdminTokenForRequestUsageReads(t *testing.T) {
	server := NewServerWithOptions(ledger.NewMemoryStore(), Options{Auth: auth.Config{AdminToken: "admin_1"}})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/request-usage?accountId=acct_1", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthRejectsMissingAdminTokenForResourceUsageReads(t *testing.T) {
	server := NewServerWithOptions(ledger.NewMemoryStore(), Options{Auth: auth.Config{AdminToken: "admin_1"}})
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/resource-usage?accountId=acct_1", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthAdminTokenAllowsBillingTopUpReads(t *testing.T) {
	store := ledger.NewMemoryStore()
	if _, err := store.ManualTopUp(nil, ledger.ManualTopUpInput{
		AccountID:     "acct_1",
		UserID:        "usr_1",
		AmountCents:   25000,
		SourceEventID: "console_manual_topup_1",
		Reason:        "initial launch credit",
	}); err != nil {
		t.Fatalf("seed manual topup: %v", err)
	}
	server := NewServerWithOptions(store, Options{Auth: auth.Config{AdminToken: "admin_1"}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/billing/topups?accountId=acct_1", nil)
	req.Header.Set("Authorization", "Bearer admin_1")
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var topups []ledger.ManualTopUp
	if err := json.Unmarshal(rec.Body.Bytes(), &topups); err != nil {
		t.Fatalf("decode topups: %v", err)
	}
	if len(topups) != 1 {
		t.Fatalf("topups = %+v", topups)
	}
}
