package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
)

func TestAppendLedgerEntryIsIdempotent(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","sourceEventId":"evt_1","amountCents":-390,"currency":"CNY"}`)
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(body)))
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	server.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(body)))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.Code, second.Body.String())
	}
	var a ledger.Entry
	var b ledger.Entry
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a.ID != b.ID {
		t.Fatalf("expected same id, got %q and %q", a.ID, b.ID)
	}
}

func TestAppendLedgerEntryRequestFingerprintReplayReturnsOK(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","requestFingerprint":"req_1","amountCents":-390,"currency":"CNY"}`)

	first := postLedgerEntry(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.code, first.body)
	}

	second := postLedgerEntry(t, server, body)
	if second.code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.code, second.body)
	}
	if first.entry.ID != second.entry.ID {
		t.Fatalf("expected same id, got %q and %q", first.entry.ID, second.entry.ID)
	}
}

func TestAppendLedgerEntryMixedKeyReplayReturnsOKWhenBindingMissingCompanionKey(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	firstBody := []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","requestFingerprint":"req_1","amountCents":-390,"currency":"CNY"}`)
	secondBody := []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","sourceEventId":"evt_1","requestFingerprint":"req_1","amountCents":-390,"currency":"CNY"}`)

	first := postLedgerEntry(t, server, firstBody)
	if first.code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.code, first.body)
	}

	second := postLedgerEntry(t, server, secondBody)
	if second.code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.code, second.body)
	}
	if first.entry.ID != second.entry.ID {
		t.Fatalf("expected same id, got %q and %q", first.entry.ID, second.entry.ID)
	}
	if second.entry.SourceEventID != "evt_1" {
		t.Fatalf("expected source event to be bound, got %q", second.entry.SourceEventID)
	}
}

func TestAppendLedgerEntryConflictingIdempotencyKeysReturnConflict(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())

	sourceOnly := postLedgerEntry(t, server, []byte(`{"eventType":"compute_debit","accountId":"acct_1","sourceEventId":"evt_1","amountCents":-390,"currency":"CNY"}`))
	if sourceOnly.code != http.StatusCreated {
		t.Fatalf("source-only status = %d body=%s", sourceOnly.code, sourceOnly.body)
	}

	fingerprintOnly := postLedgerEntry(t, server, []byte(`{"eventType":"compute_debit","accountId":"acct_1","requestFingerprint":"req_1","amountCents":-390,"currency":"CNY"}`))
	if fingerprintOnly.code != http.StatusCreated {
		t.Fatalf("fingerprint-only status = %d body=%s", fingerprintOnly.code, fingerprintOnly.body)
	}

	conflict := postLedgerEntry(t, server, []byte(`{"eventType":"compute_debit","accountId":"acct_1","sourceEventId":"evt_1","requestFingerprint":"req_1","amountCents":-390,"currency":"CNY"}`))
	if conflict.code != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", conflict.code, conflict.body)
	}
}

func TestAppendLedgerEntryWithoutIdempotencyKeyReturnsBadRequest(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())

	response := postLedgerEntry(t, server, []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","amountCents":-390,"currency":"CNY"}`))
	if response.code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key status = %d body=%s", response.code, response.body)
	}
}

func TestAppendLedgerEntryConflictingReplayReturnsConflict(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())

	first := postLedgerEntry(t, server, []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","sourceEventId":"evt_1","amountCents":-390,"currency":"CNY"}`))
	if first.code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.code, first.body)
	}

	conflict := postLedgerEntry(t, server, []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","sourceEventId":"evt_1","amountCents":-490,"currency":"CNY"}`))
	if conflict.code != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", conflict.code, conflict.body)
	}
}

func TestAppendLedgerEntryValidationErrorsRemainBadRequest(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())

	response := postLedgerEntry(t, server, []byte(`{"accountId":"acct_1","sourceEventId":"evt_1","amountCents":-390,"currency":"CNY"}`))
	if response.code != http.StatusBadRequest {
		t.Fatalf("validation status = %d body=%s", response.code, response.body)
	}
}

func TestAppendLedgerEntryConcurrentDuplicateSourceEventReturnsOneCreatedAndRestOK(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","sourceEventId":"evt_1","amountCents":-390,"currency":"CNY"}`)

	const requestCount = 64
	start := make(chan struct{})
	responses := make(chan ledgerAppendResponse, requestCount)
	var wg sync.WaitGroup
	for range requestCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			responses <- postLedgerEntry(t, server, body)
		}()
	}
	close(start)
	wg.Wait()
	close(responses)

	created := 0
	replayed := 0
	ids := map[string]bool{}
	for response := range responses {
		switch response.code {
		case http.StatusCreated:
			created++
		case http.StatusOK:
			replayed++
		default:
			t.Fatalf("unexpected status = %d body=%s", response.code, response.body)
		}
		ids[response.entry.ID] = true
	}
	if created != 1 {
		t.Fatalf("expected exactly one 201 response, got %d", created)
	}
	if replayed != requestCount-1 {
		t.Fatalf("expected %d replay responses, got %d", requestCount-1, replayed)
	}
	if len(ids) != 1 {
		t.Fatalf("expected one ledger entry ID, got %d", len(ids))
	}
}

func TestLedgerSummary(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	events := [][]byte{
		[]byte(`{"eventType":"manual_topup","accountId":"acct_1","sourceEventId":"topup_1","amountCents":1000,"currency":"CNY"}`),
		[]byte(`{"eventType":"compute_debit","accountId":"acct_1","sourceEventId":"debit_1","amountCents":-390,"currency":"CNY"}`),
	}
	for _, event := range events {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(event)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("append status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/summary?accountId=acct_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("summary status = %d body=%s", rec.Code, rec.Body.String())
	}
	var summary ledger.Summary
	_ = json.Unmarshal(rec.Body.Bytes(), &summary)
	if summary.BalanceCents != 610 {
		t.Fatalf("expected balance 610, got %d", summary.BalanceCents)
	}
}

func TestManualTopUpAPIAppendsCreditLedgerEntry(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"amountCents":25000,
		"reason":"owner_credit",
		"operatorUserId":"usr_admin"
	}`))))
	if rec.Code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result ledger.ManualTopUpResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode topup response: %v", err)
	}
	entry := result.Entry
	if entry.EventType != "credit" || entry.AmountCents != 25000 || entry.SourceEventID != "owner_credit" {
		t.Fatalf("unexpected topup entry: %+v", entry)
	}
}

func TestManualTopUpAPIWritesWalletLedgerTransactionTopupAndAudit(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":25000,
		"reason":"owner_credit_1",
		"operatorUserId":"usr_admin",
		"operatorAccountId":"acct_admin"
	}`))))
	if rec.Code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result ledger.ManualTopUpResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode topup result: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Wallet.BalanceCents != 25000 || result.Wallet.AvailableCents != 25000 || result.Wallet.TotalRechargedCents != 25000 {
		t.Fatalf("wallet snapshot = %+v", result.Wallet)
	}
	if result.Entry.EventType != "credit" || result.Entry.AmountCents != 25000 || result.Entry.SourceEventID != "owner_credit_1" {
		t.Fatalf("ledger entry = %+v", result.Entry)
	}
	if result.Transaction.Type != "credit" || result.Transaction.AmountCents != 25000 || result.Transaction.LedgerEntryID != result.Entry.ID {
		t.Fatalf("wallet transaction = %+v", result.Transaction)
	}
	if result.TopUp.TargetAccountID != "acct_1" || result.TopUp.WalletTransactionID != result.Transaction.ID || result.TopUp.LedgerEntryID != result.Entry.ID {
		t.Fatalf("manual topup = %+v", result.TopUp)
	}
	if result.AuditEvent.Action != "account.credit_granted" || result.AuditEvent.SourceEventID != result.TopUp.ID {
		t.Fatalf("audit event = %+v", result.AuditEvent)
	}
}

func TestManualTopUpAPIReplayDoesNotDoubleCredit(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":25000,
		"reason":"owner_credit_1",
		"operatorUserId":"usr_admin"
	}`)

	first := postManualTopUp(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.code, first.body)
	}
	second := postManualTopUp(t, server, body)
	if second.code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.code, second.body)
	}
	if second.result.Created {
		t.Fatalf("expected replay result")
	}
	if first.result.Entry.ID != second.result.Entry.ID {
		t.Fatalf("expected same ledger entry, got %q and %q", first.result.Entry.ID, second.result.Entry.ID)
	}
	if first.result.Transaction.ID != second.result.Transaction.ID {
		t.Fatalf("expected same wallet transaction, got %q and %q", first.result.Transaction.ID, second.result.Transaction.ID)
	}
	if second.result.Wallet.BalanceCents != 25000 || second.result.Wallet.TotalRechargedCents != 25000 {
		t.Fatalf("wallet was double credited: %+v", second.result.Wallet)
	}
}

func TestRequestUsageAPIAppendsIdempotentRequestDebit(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := httptest.NewRecorder()
	server.ServeHTTP(topup, httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":25000,
		"reason":"owner_credit_1"
	}`))))
	if topup.Code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.Code, topup.Body.String())
	}
	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_1",
		"provider":"openai",
		"model":"gpt-5",
		"inputTokens":1000,
		"outputTokens":500,
		"amountCents":25,
		"sourceEventId":"gateway_req_1"
	}`)
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader(body)))
	if first.Code != http.StatusCreated {
		t.Fatalf("first request usage status = %d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	server.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader(body)))
	if second.Code != http.StatusOK {
		t.Fatalf("second request usage status = %d body=%s", second.Code, second.Body.String())
	}
	var firstResult ledger.RequestUsageResult
	var secondResult ledger.RequestUsageResult
	_ = json.Unmarshal(first.Body.Bytes(), &firstResult)
	_ = json.Unmarshal(second.Body.Bytes(), &secondResult)
	if firstResult.Entry.ID != secondResult.Entry.ID {
		t.Fatalf("expected idempotent request usage entry, got %q and %q", firstResult.Entry.ID, secondResult.Entry.ID)
	}
	if firstResult.Log.ID != secondResult.Log.ID {
		t.Fatalf("expected same request usage log, got %q and %q", firstResult.Log.ID, secondResult.Log.ID)
	}
	if firstResult.Entry.EventType != "request_debit" || firstResult.Entry.AmountCents != -25 {
		t.Fatalf("unexpected request usage entry: %+v", firstResult.Entry)
	}
	if firstResult.Transaction.AmountCents != -25 || firstResult.Transaction.LedgerEntryID != firstResult.Entry.ID || firstResult.Transaction.UsageLogID != firstResult.Log.ID {
		t.Fatalf("unexpected request wallet transaction: %+v", firstResult.Transaction)
	}
	if firstResult.Log.AmountCents != 25 || firstResult.Log.UnpaidCents != 0 || firstResult.Log.LedgerEntryID != firstResult.Entry.ID {
		t.Fatalf("unexpected request usage log: %+v", firstResult.Log)
	}
	if secondResult.Wallet.BalanceCents != 24975 || secondResult.Wallet.AvailableCents != 24975 {
		t.Fatalf("wallet was double debited: %+v", secondResult.Wallet)
	}
	if firstResult.AuditEvent.Action != "billing.request_usage_recorded" {
		t.Fatalf("unexpected audit event: %+v", firstResult.AuditEvent)
	}
}

func TestRequestUsageAPIConflictingReplayReturnsConflict(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_1",
		"provider":"openai",
		"model":"gpt-5",
		"inputTokens":1000,
		"outputTokens":500,
		"amountCents":25,
		"sourceEventId":"gateway_req_1"
	}`)
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader(body)))
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	conflictBody := bytes.Replace(body, []byte(`"amountCents":25`), []byte(`"amountCents":35`), 1)
	conflict := httptest.NewRecorder()
	server.ServeHTTP(conflict, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader(conflictBody)))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestRequestUsageAPIQuotaExceededDoesNotMutateBillingState(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := httptest.NewRecorder()
	server.ServeHTTP(topup, httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))))
	if topup.Code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.Code, topup.Body.String())
	}

	rejected := httptest.NewRecorder()
	server.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_1",
		"amountCents":25,
		"sourceEventId":"gateway_req_1",
		"requestQuota":{"limit":0,"used":0}
	}`))))
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("rejected status = %d body=%s", rejected.Code, rejected.Body.String())
	}

	allowed := httptest.NewRecorder()
	server.ServeHTTP(allowed, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_1",
		"amountCents":25,
		"sourceEventId":"gateway_req_1",
		"requestQuota":{"limit":2,"used":0}
	}`))))
	if allowed.Code != http.StatusCreated {
		t.Fatalf("allowed status = %d body=%s", allowed.Code, allowed.Body.String())
	}
	var result ledger.RequestUsageResult
	if err := json.Unmarshal(allowed.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode allowed result: %v", err)
	}
	if result.Wallet.BalanceCents != 975 {
		t.Fatalf("wallet balance = %d", result.Wallet.BalanceCents)
	}
	if result.Log.Quota == nil || result.Log.Quota.Used != 1 {
		t.Fatalf("quota result = %+v", result.Log.Quota)
	}

	summary := httptest.NewRecorder()
	server.ServeHTTP(summary, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/summary?accountId=acct_1", nil))
	if summary.Code != http.StatusOK {
		t.Fatalf("summary status = %d body=%s", summary.Code, summary.Body.String())
	}
	var ledgerSummary ledger.Summary
	if err := json.Unmarshal(summary.Body.Bytes(), &ledgerSummary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}
	if ledgerSummary.BalanceCents != 975 || ledgerSummary.EntryCount != 2 {
		t.Fatalf("unexpected summary after rejection and one success: %+v", ledgerSummary)
	}
}

func TestAuditEventAPIPostsAndQueriesEvents(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"actorId":"usr_1",
		"action":"billing.settled",
		"targetKind":"workspace",
		"targetId":"ws_1",
		"sourceEventId":"billing_tick_1",
		"payload":{"amountCents":47}
	}`)
	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/audit/events", bytes.NewReader(body)))
	if post.Code != http.StatusCreated {
		t.Fatalf("post audit status = %d body=%s", post.Code, post.Body.String())
	}
	var event ledger.AuditEvent
	if err := json.Unmarshal(post.Body.Bytes(), &event); err != nil {
		t.Fatalf("decode audit event: %v", err)
	}
	if event.ID == "" || event.Action != "billing.settled" {
		t.Fatalf("event = %+v", event)
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/audit/events?accountId=acct_1&workspaceId=ws_1&action=billing.settled&sourceEventId=billing_tick_1", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get audit status = %d body=%s", get.Code, get.Body.String())
	}
	var events []ledger.AuditEvent
	if err := json.Unmarshal(get.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode audit events: %v", err)
	}
	if len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("events = %+v", events)
	}
}

func TestTaskReceiptAPIPostsAndQueriesReceipts(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"taskId":"task_1",
		"actor":{"type":"user","id":"usr_1"},
		"plan":{"goal":"run analysis"},
		"approval":{"status":"approved"},
		"environment":{"runtimeProvider":"tencent-tke"},
		"executionRefs":[{"type":"run","uri":"opl://run/1"}]
	}`)
	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/task-receipts", bytes.NewReader(body)))
	if post.Code != http.StatusCreated {
		t.Fatalf("post receipt status = %d body=%s", post.Code, post.Body.String())
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/task-receipts?accountId=acct_1&workspaceId=ws_1&taskId=task_1", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get receipt status = %d body=%s", get.Code, get.Body.String())
	}
	var receipts []ledger.TaskReceipt
	if err := json.Unmarshal(get.Body.Bytes(), &receipts); err != nil {
		t.Fatalf("decode receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].TaskID != "task_1" {
		t.Fatalf("unexpected receipts: %+v", receipts)
	}
}

func TestReconciliationAPIStoresLatestReport(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"provider":"tencent",
		"markupRate":0.2,
		"ledgerRows":[{"workspaceId":"ws_1","resourceType":"compute","amountCents":-1200}],
		"tencentRows":[{"workspaceId":"ws_1","resourceType":"compute","amountCents":1000}]
	}`)
	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/billing/reconciliation", bytes.NewReader(body)))
	if post.Code != http.StatusCreated {
		t.Fatalf("post reconciliation status = %d body=%s", post.Code, post.Body.String())
	}
	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/billing/reconciliation/latest", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get reconciliation status = %d body=%s", get.Code, get.Body.String())
	}
	var report ledger.ReconciliationReport
	if err := json.Unmarshal(get.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode reconciliation report: %v", err)
	}
	if report.Provider != "tencent" || report.Status != "pass" {
		t.Fatalf("unexpected reconciliation report: %+v", report)
	}
}

type ledgerAppendResponse struct {
	code  int
	body  string
	entry ledger.Entry
}

type manualTopUpResponse struct {
	code   int
	body   string
	result ledger.ManualTopUpResult
}

func postLedgerEntry(t *testing.T, server http.Handler, body []byte) ledgerAppendResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(body)))
	response := ledgerAppendResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.entry); err != nil {
			t.Fatalf("decode append response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func postManualTopUp(t *testing.T, server http.Handler, body []byte) manualTopUpResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/billing/topups", bytes.NewReader(body)))
	response := manualTopUpResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.result); err != nil {
			t.Fatalf("decode topup response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}
