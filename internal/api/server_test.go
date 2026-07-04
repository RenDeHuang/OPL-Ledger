package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	"github.com/RenDeHuang/OPL-Ledger/internal/usage"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
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

func TestManualTopUpAPISeparatesSourceEventIDFromReason(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":25000,
		"sourceEventId":"console_manual_topup_1",
		"reason":"initial launch credit",
		"operatorUserId":"usr_admin",
		"operatorAccountId":"acct_admin"
	}`)

	first := postManualTopUp(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.code, first.body)
	}
	second := postManualTopUp(t, server, body)
	if second.code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.code, second.body)
	}
	if first.result.Entry.SourceEventID != "console_manual_topup_1" {
		t.Fatalf("entry source event id = %q", first.result.Entry.SourceEventID)
	}
	if first.result.Transaction.SourceEventID != "console_manual_topup_1" {
		t.Fatalf("transaction source event id = %q", first.result.Transaction.SourceEventID)
	}
	if first.result.TopUp.SourceEventID != "console_manual_topup_1" {
		t.Fatalf("topup source event id = %q", first.result.TopUp.SourceEventID)
	}
	if first.result.TopUp.Reason != "initial launch credit" {
		t.Fatalf("topup reason = %q", first.result.TopUp.Reason)
	}
	if second.result.Wallet.BalanceCents != 25000 || second.result.Wallet.TotalRechargedCents != 25000 {
		t.Fatalf("wallet was double credited: %+v", second.result.Wallet)
	}
}

func TestListManualTopUpsFiltersByAccountAndSourceEvent(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	first := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":25000,
		"sourceEventId":"console_manual_topup_1",
		"reason":"initial launch credit"
	}`))
	if first.code != http.StatusCreated {
		t.Fatalf("first topup status = %d body=%s", first.code, first.body)
	}
	second := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_2",
		"userId":"usr_2",
		"amountCents":15000,
		"sourceEventId":"console_manual_topup_2",
		"reason":"second account credit"
	}`))
	if second.code != http.StatusCreated {
		t.Fatalf("second topup status = %d body=%s", second.code, second.body)
	}

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/topups?accountId=acct_1&sourceEventId=console_manual_topup_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list topups status = %d body=%s", rec.Code, rec.Body.String())
	}
	var topups []ledger.ManualTopUp
	if err := json.Unmarshal(rec.Body.Bytes(), &topups); err != nil {
		t.Fatalf("decode topups: %v", err)
	}
	if len(topups) != 1 {
		t.Fatalf("expected 1 topup, got %d: %+v", len(topups), topups)
	}
	if topups[0].SourceEventID != "console_manual_topup_1" || topups[0].Reason != "initial launch credit" {
		t.Fatalf("unexpected topup: %+v", topups[0])
	}
}

func TestListWalletsFiltersByAccountID(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":25000,
		"sourceEventId":"console_manual_topup_1",
		"reason":"initial launch credit"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}

	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/wallets?accountId=acct_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list wallets status = %d body=%s", rec.Code, rec.Body.String())
	}
	var wallets []wallet.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &wallets); err != nil {
		t.Fatalf("decode wallets: %v", err)
	}
	if len(wallets) != 1 {
		t.Fatalf("expected 1 wallet, got %d: %+v", len(wallets), wallets)
	}
	if wallets[0].AccountID != "acct_1" || wallets[0].BalanceCents != 25000 || wallets[0].AvailableCents != 25000 || wallets[0].TotalRechargedCents != 25000 {
		t.Fatalf("unexpected wallet snapshot: %+v", wallets[0])
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

func TestRequestUsageAPIUsesPersistedRequestQuota(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}
	quota := putRequestQuota(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"quota":{"limit":1,"used":0}
	}`))
	if quota.code != http.StatusOK {
		t.Fatalf("quota status = %d body=%s", quota.code, quota.body)
	}

	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_1",
		"amountCents":25,
		"sourceEventId":"gateway_req_1"
	}`))))
	if first.Code != http.StatusCreated {
		t.Fatalf("first request usage status = %d body=%s", first.Code, first.Body.String())
	}
	var firstResult ledger.RequestUsageResult
	if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatalf("decode first request usage: %v", err)
	}
	if firstResult.Log.Quota == nil || firstResult.Log.Quota.Used != 1 {
		t.Fatalf("expected persisted quota increment, got %+v", firstResult.Log.Quota)
	}

	second := httptest.NewRecorder()
	server.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_2",
		"amountCents":25,
		"sourceEventId":"gateway_req_2"
	}`))))
	if second.Code != http.StatusBadRequest {
		t.Fatalf("second request usage status = %d body=%s", second.Code, second.Body.String())
	}

	quotas := getRequestQuotas(t, server, "/api/v1/billing/request-quotas?accountId=acct_1&workspaceId=ws_1")
	if quotas.code != http.StatusOK {
		t.Fatalf("get quotas status = %d body=%s", quotas.code, quotas.body)
	}
	if len(quotas.records) != 1 || quotas.records[0].Quota.Used != 1 {
		t.Fatalf("quotas = %+v", quotas.records)
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
		t.Fatalf("unexpected summary after quota rejection: %+v", ledgerSummary)
	}
}

func TestRequestUsageAPIListsUsageLogsForOperatorReview(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/billing/request-usage", bytes.NewReader([]byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"requestId":"req_1",
		"provider":"openai",
		"model":"gpt-5",
		"amountCents":25,
		"sourceEventId":"gateway_req_1"
	}`))))
	if first.Code != http.StatusCreated {
		t.Fatalf("request usage status = %d body=%s", first.Code, first.Body.String())
	}

	logs := getRequestUsageLogs(t, server, "/api/v1/billing/request-usage?accountId=acct_1&workspaceId=ws_1&sourceEventId=gateway_req_1")
	if logs.code != http.StatusOK {
		t.Fatalf("list request usage status = %d body=%s", logs.code, logs.body)
	}
	if len(logs.logs) != 1 {
		t.Fatalf("logs = %+v", logs.logs)
	}
	if logs.logs[0].RequestID != "req_1" || logs.logs[0].AmountCents != 25 || logs.logs[0].SourceEventID != "gateway_req_1" {
		t.Fatalf("request usage log = %+v", logs.logs[0])
	}
}

func TestHoldAPIAppendsIdempotentComputeHold(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}

	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":600,
		"sourceEventId":"compute_resource:compute_1:created",
		"resourceId":"compute_1",
		"packageId":"basic"
	}`)
	first := postHold(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first hold status = %d body=%s", first.code, first.body)
	}
	second := postHold(t, server, body)
	if second.code != http.StatusOK {
		t.Fatalf("second hold status = %d body=%s", second.code, second.body)
	}
	if first.result.Entry.ID != second.result.Entry.ID {
		t.Fatalf("expected same hold entry, got %q and %q", first.result.Entry.ID, second.result.Entry.ID)
	}
	if first.result.Transaction.ID != second.result.Transaction.ID {
		t.Fatalf("expected same hold transaction, got %q and %q", first.result.Transaction.ID, second.result.Transaction.ID)
	}
	if first.result.Entry.EventType != "compute_hold" || first.result.Entry.AmountCents != 600 || first.result.Entry.ComputeID != "compute_1" {
		t.Fatalf("unexpected hold entry: %+v", first.result.Entry)
	}
	if first.result.Transaction.Type != wallet.TransactionHold || first.result.Transaction.AmountCents != 600 || first.result.Transaction.LedgerEntryID != first.result.Entry.ID {
		t.Fatalf("unexpected hold transaction: %+v", first.result.Transaction)
	}
	if second.result.Wallet.BalanceCents != 1000 || second.result.Wallet.FrozenCents != 600 || second.result.Wallet.AvailableCents != 400 || second.result.Wallet.Holds["compute"] != 600 {
		t.Fatalf("unexpected replay wallet: %+v", second.result.Wallet)
	}
}

func TestFabricResourcePreflightAliasCreatesIdempotentHold(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}

	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":600,
		"sourceEventId":"fabric:compute:compute_1:create_requested",
		"resourceId":"compute_1",
		"packageId":"basic"
	}`)
	first := postHoldTo(t, server, "/api/v1/fabric/resource-preflight", body)
	if first.code != http.StatusCreated {
		t.Fatalf("first fabric preflight status = %d body=%s", first.code, first.body)
	}
	second := postHoldTo(t, server, "/api/v1/fabric/resource-preflight", body)
	if second.code != http.StatusOK {
		t.Fatalf("second fabric preflight status = %d body=%s", second.code, second.body)
	}
	if first.result.Entry.EventType != "compute_hold" || first.result.Transaction.Type != wallet.TransactionHold {
		t.Fatalf("unexpected fabric preflight result: entry=%+v transaction=%+v", first.result.Entry, first.result.Transaction)
	}
	if first.result.Entry.ID != second.result.Entry.ID || first.result.Transaction.ID != second.result.Transaction.ID {
		t.Fatalf("expected replayed fabric preflight records")
	}
}

func TestHoldAPIInsufficientAvailableBalanceDoesNotMutateBillingState(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":500,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}

	rejected := postHold(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":600,
		"sourceEventId":"compute_resource:compute_1:created",
		"resourceId":"compute_1"
	}`))
	if rejected.code != http.StatusBadRequest {
		t.Fatalf("rejected hold status = %d body=%s", rejected.code, rejected.body)
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
	if ledgerSummary.BalanceCents != 500 || ledgerSummary.EntryCount != 1 {
		t.Fatalf("unexpected summary after rejected hold: %+v", ledgerSummary)
	}
}

func TestHoldReleaseAPIReleasesExistingComputeHold(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}
	hold := postHold(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":600,
		"sourceEventId":"compute_resource:compute_1:created",
		"resourceId":"compute_1"
	}`))
	if hold.code != http.StatusCreated {
		t.Fatalf("hold status = %d body=%s", hold.code, hold.body)
	}

	body := []byte(`{
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"holdTypes":["compute"],
		"sourceEventId":"compute_resource:compute_1:stopped",
		"computeId":"compute_1",
		"reason":"stop_compute"
	}`)
	released := postHoldRelease(t, server, body)
	if released.code != http.StatusCreated {
		t.Fatalf("release status = %d body=%s", released.code, released.body)
	}
	replayed := postHoldRelease(t, server, body)
	if replayed.code != http.StatusOK {
		t.Fatalf("release replay status = %d body=%s", replayed.code, replayed.body)
	}
	if len(released.result.Entries) != 1 || released.result.Entries[0].EventType != "compute_hold_released" || released.result.Entries[0].AmountCents != -600 {
		t.Fatalf("unexpected release entries: %+v", released.result.Entries)
	}
	if len(released.result.Transactions) != 1 || released.result.Transactions[0].Type != wallet.TransactionHoldRelease || released.result.Transactions[0].LedgerEntryID != released.result.Entries[0].ID {
		t.Fatalf("unexpected release transactions: %+v", released.result.Transactions)
	}
	if replayed.result.Wallet.BalanceCents != 1000 || replayed.result.Wallet.FrozenCents != 0 || replayed.result.Wallet.AvailableCents != 1000 || replayed.result.Wallet.Holds["compute"] != 0 {
		t.Fatalf("unexpected release replay wallet: %+v", replayed.result.Wallet)
	}
}

func TestFabricLifecycleAliasesRecordUsageSettlementReleaseAndEvidence(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}
	hold := postHold(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":700,
		"sourceEventId":"fabric:compute:compute_1:create_requested",
		"resourceId":"compute_1"
	}`))
	if hold.code != http.StatusCreated {
		t.Fatalf("hold status = %d body=%s", hold.code, hold.body)
	}

	evidence := postEvidenceTo(t, server, "/api/v1/fabric/resource-created", []byte(`{
		"type":"fabric.compute.created",
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"targetKind":"compute",
		"targetId":"compute_1",
		"sourceEventId":"fabric:compute:compute_1:created",
		"plan":{"packageId":"basic"},
		"approval":{"status":"ledger_hold_created"},
		"environment":{"runtimeProvider":"tencent"},
		"payload":{"provider":"tencent","packageId":"basic"}
	}`))
	if evidence.code != http.StatusCreated {
		t.Fatalf("fabric created evidence status = %d body=%s", evidence.code, evidence.body)
	}
	if evidence.record.Type != "fabric.compute.created" || evidence.record.SourceEventID != "fabric:compute:compute_1:created" {
		t.Fatalf("fabric evidence = %+v", evidence.record)
	}

	usage := postResourceUsageTo(t, server, "/api/v1/fabric/resource-usage-tick", []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"computeId":"compute_1",
		"resourceKind":"compute",
		"quantity":1,
		"unit":"hour",
		"unitPriceCents":120,
		"amountCents":120,
		"sourceEventId":"fabric:compute:compute_1:usage:2026070412"
	}`))
	if usage.code != http.StatusCreated {
		t.Fatalf("fabric usage status = %d body=%s", usage.code, usage.body)
	}
	if usage.result.Log.ComputeID != "compute_1" || usage.result.Log.AmountCents != 120 {
		t.Fatalf("fabric usage log = %+v", usage.result.Log)
	}

	settlement := postSettlementTo(t, server, "/api/v1/fabric/resource-settlement", []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"computeId":"compute_1",
		"sourceEventId":"fabric:compute:compute_1:settlement:2026070412",
		"hours":1,
		"computeActive":true,
		"computeHourlyCents":120
	}`))
	if settlement.code != http.StatusCreated {
		t.Fatalf("fabric settlement status = %d body=%s", settlement.code, settlement.body)
	}
	if len(settlement.result.Transactions) != 1 || settlement.result.Transactions[0].Type != wallet.TransactionDebit {
		t.Fatalf("fabric settlement transactions = %+v", settlement.result.Transactions)
	}

	release := postHoldReleaseTo(t, server, "/api/v1/fabric/resource-destroyed", []byte(`{
		"accountId":"acct_1",
		"workspaceId":"ws_1",
		"holdTypes":["compute"],
		"sourceEventId":"fabric:compute:compute_1:destroyed",
		"computeId":"compute_1",
		"reason":"resource destroyed"
	}`))
	if release.code != http.StatusCreated {
		t.Fatalf("fabric destroyed release status = %d body=%s", release.code, release.body)
	}
	if len(release.result.Transactions) != 1 || release.result.Transactions[0].Type != wallet.TransactionHoldRelease {
		t.Fatalf("fabric release transactions = %+v", release.result.Transactions)
	}
}

func TestSettlementAPIChargesAvailableBeforeComputeHoldAndReplays(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}
	hold := postHold(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":700,
		"sourceEventId":"compute_resource:compute_1:created",
		"resourceId":"compute_1"
	}`))
	if hold.code != http.StatusCreated {
		t.Fatalf("hold status = %d body=%s", hold.code, hold.body)
	}

	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"computeId":"compute_1",
		"sourceEventId":"billing_tick_1",
		"hours":1,
		"computeActive":true,
		"computeHourlyCents":500
	}`)
	first := postSettlement(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first settlement status = %d body=%s", first.code, first.body)
	}
	second := postSettlement(t, server, body)
	if second.code != http.StatusOK {
		t.Fatalf("second settlement status = %d body=%s", second.code, second.body)
	}
	if len(first.result.Entries) != 2 {
		t.Fatalf("entries = %+v", first.result.Entries)
	}
	if first.result.Entries[0].EventType != "compute_debit" || first.result.Entries[0].AmountCents != -300 {
		t.Fatalf("available entry = %+v", first.result.Entries[0])
	}
	if first.result.Entries[1].EventType != "compute_debit" || first.result.Entries[1].AmountCents != -200 {
		t.Fatalf("hold entry = %+v", first.result.Entries[1])
	}
	if len(first.result.Transactions) != 2 || first.result.Transactions[0].FundingSource != "available_balance" || first.result.Transactions[1].FundingSource != "compute_hold" {
		t.Fatalf("transactions = %+v", first.result.Transactions)
	}
	if second.result.Wallet.BalanceCents != 500 || second.result.Wallet.FrozenCents != 500 || second.result.Wallet.AvailableCents != 0 || second.result.Wallet.Holds["compute"] != 500 {
		t.Fatalf("wallet was double settled or wrong: %+v", second.result.Wallet)
	}
	if first.result.Entries[0].ID != second.result.Entries[0].ID || first.result.Transactions[0].ID != second.result.Transactions[0].ID {
		t.Fatalf("expected replayed settlement records")
	}
}

func TestResourceUsageAPIRecordsIdempotentComputeUsage(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"computeId":"compute_1",
		"resourceKind":"compute",
		"quantity":1,
		"unit":"hour",
		"unitPriceCents":47,
		"amountCents":47,
		"sourceEventId":"resource_usage:compute_1:billing_tick_1"
	}`)
	first := postResourceUsage(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first resource usage status = %d body=%s", first.code, first.body)
	}
	second := postResourceUsage(t, server, body)
	if second.code != http.StatusOK {
		t.Fatalf("second resource usage status = %d body=%s", second.code, second.body)
	}
	if first.result.Log.ID != second.result.Log.ID {
		t.Fatalf("expected replayed resource usage log")
	}
	if first.result.Log.ResourceKind != usage.ResourceKindCompute || first.result.Log.ComputeID != "compute_1" || first.result.Log.AmountCents != 47 {
		t.Fatalf("unexpected resource usage log: %+v", first.result.Log)
	}
}

func TestResourceUsageAPIListsUsageLogsForOperatorReview(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"computeId":"compute_1",
		"resourceKind":"compute",
		"quantity":1,
		"unit":"hour",
		"unitPriceCents":47,
		"amountCents":47,
		"sourceEventId":"resource_usage:compute_1:billing_tick_1"
	}`)
	first := postResourceUsage(t, server, body)
	if first.code != http.StatusCreated {
		t.Fatalf("first resource usage status = %d body=%s", first.code, first.body)
	}

	logs := getResourceUsageLogs(t, server, "/api/v1/billing/resource-usage?accountId=acct_1&workspaceId=ws_1&sourceEventId=resource_usage:compute_1:billing_tick_1")
	if logs.code != http.StatusOK {
		t.Fatalf("list resource usage status = %d body=%s", logs.code, logs.body)
	}
	if len(logs.logs) != 1 {
		t.Fatalf("logs = %+v", logs.logs)
	}
	if logs.logs[0].ComputeID != "compute_1" || logs.logs[0].AmountCents != 47 || logs.logs[0].SourceEventID != "resource_usage:compute_1:billing_tick_1" {
		t.Fatalf("resource usage log = %+v", logs.logs[0])
	}
}

func TestWalletTransactionsAPIListsAndFiltersTransactions(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	topup := postManualTopUp(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"amountCents":1000,
		"reason":"owner_credit_1"
	}`))
	if topup.code != http.StatusCreated {
		t.Fatalf("topup status = %d body=%s", topup.code, topup.body)
	}
	hold := postHold(t, server, []byte(`{
		"accountId":"acct_1",
		"userId":"usr_1",
		"workspaceId":"ws_1",
		"holdType":"compute",
		"amountCents":600,
		"sourceEventId":"compute_resource:compute_1:created",
		"resourceId":"compute_1"
	}`))
	if hold.code != http.StatusCreated {
		t.Fatalf("hold status = %d body=%s", hold.code, hold.body)
	}

	all := getWalletTransactions(t, server, "/api/v1/billing/wallet-transactions?accountId=acct_1")
	if all.code != http.StatusOK {
		t.Fatalf("wallet transactions status = %d body=%s", all.code, all.body)
	}
	if len(all.transactions) != 2 {
		t.Fatalf("transactions = %+v", all.transactions)
	}
	if all.transactions[0].Type != wallet.TransactionCredit || all.transactions[1].Type != wallet.TransactionHold {
		t.Fatalf("transaction order/types = %+v", all.transactions)
	}

	holds := getWalletTransactions(t, server, "/api/v1/billing/wallet-transactions?accountId=acct_1&type=hold")
	if holds.code != http.StatusOK {
		t.Fatalf("hold transactions status = %d body=%s", holds.code, holds.body)
	}
	if len(holds.transactions) != 1 || holds.transactions[0].Type != wallet.TransactionHold || holds.transactions[0].SourceEventID != "compute_resource:compute_1:created" {
		t.Fatalf("hold transactions = %+v", holds.transactions)
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

func TestKubernetesEvidenceSnapshotAPIPostsAndQueriesSnapshots(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{
		"clusterId":"cluster_1",
		"namespace":"opl-cloud",
		"objectKind":"Deployment",
		"objectName":"opl-ws-1",
		"workspaceId":"ws_1",
		"resourceVersion":"42",
		"observedGeneration":7,
		"readinessStatus":"ready",
		"redactedObject":{
			"kind":"Deployment",
			"name":"opl-ws-1",
			"readyReplicas":1
		}
	}`)
	post := httptest.NewRecorder()
	server.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/kubernetes-evidence-snapshots", bytes.NewReader(body)))
	if post.Code != http.StatusCreated {
		t.Fatalf("post kubernetes snapshot status = %d body=%s", post.Code, post.Body.String())
	}
	var posted ledger.KubernetesEvidenceSnapshot
	if err := json.Unmarshal(post.Body.Bytes(), &posted); err != nil {
		t.Fatalf("decode posted snapshot: %v", err)
	}
	if posted.CollectedAt.IsZero() {
		t.Fatalf("expected collectedAt to be set")
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/kubernetes-evidence-snapshots?workspaceId=ws_1&objectKind=Deployment", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("get kubernetes snapshots status = %d body=%s", get.Code, get.Body.String())
	}
	var snapshots []ledger.KubernetesEvidenceSnapshot
	if err := json.Unmarshal(get.Body.Bytes(), &snapshots); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].ObjectName != "opl-ws-1" || snapshots[0].ReadinessStatus != "ready" {
		t.Fatalf("snapshots = %+v", snapshots)
	}
	payload := string(mustJSON(t, snapshots[0].RedactedObject))
	if bytes.Contains([]byte(payload), []byte("secret-value")) {
		t.Fatalf("snapshot leaked secret value: %s", payload)
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

func TestReconciliationAPIListsReportsByProviderAndStatus(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	passBody := []byte(`{
		"provider":"tencent",
		"markupRate":0.2,
		"ledgerRows":[{"workspaceId":"ws_1","resourceType":"compute","amountCents":-1200}],
		"tencentRows":[{"workspaceId":"ws_1","resourceType":"compute","amountCents":1000}]
	}`)
	postPass := httptest.NewRecorder()
	server.ServeHTTP(postPass, httptest.NewRequest(http.MethodPost, "/api/v1/billing/reconciliation", bytes.NewReader(passBody)))
	if postPass.Code != http.StatusCreated {
		t.Fatalf("post pass reconciliation status = %d body=%s", postPass.Code, postPass.Body.String())
	}
	failBody := []byte(`{
		"provider":"tencent",
		"markupRate":0.2,
		"ledgerRows":[{"workspaceId":"ws_1","resourceType":"compute","amountCents":-1100}],
		"tencentRows":[{"workspaceId":"ws_1","resourceType":"compute","amountCents":1000}]
	}`)
	postFail := httptest.NewRecorder()
	server.ServeHTTP(postFail, httptest.NewRequest(http.MethodPost, "/api/v1/billing/reconciliation", bytes.NewReader(failBody)))
	if postFail.Code != http.StatusCreated {
		t.Fatalf("post fail reconciliation status = %d body=%s", postFail.Code, postFail.Body.String())
	}

	get := httptest.NewRecorder()
	server.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/billing/reconciliation?provider=tencent&status=fail", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("list reconciliation status = %d body=%s", get.Code, get.Body.String())
	}
	var reports []ledger.ReconciliationReport
	if err := json.Unmarshal(get.Body.Bytes(), &reports); err != nil {
		t.Fatalf("decode reconciliation reports: %v", err)
	}
	if len(reports) != 1 || reports[0].Provider != "tencent" || reports[0].Status != "fail" {
		t.Fatalf("reports = %+v", reports)
	}
}

func TestReconciliationGuardBlocksWhenReportMissing(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/reconciliation/guard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("guard status = %d body=%s", rec.Code, rec.Body.String())
	}
	var guard ledger.ReconciliationGuard
	if err := json.Unmarshal(rec.Body.Bytes(), &guard); err != nil {
		t.Fatalf("decode guard: %v", err)
	}
	if guard.Status != "blocked" || !guard.BlockNewWorkspaces || guard.Reason != "billing_reconciliation_report_missing" {
		t.Fatalf("guard = %+v", guard)
	}
}

func TestReconciliationGuardBlocksWhenReportStale(t *testing.T) {
	store := ledger.NewMemoryStore()
	_, err := store.AppendReconciliationReport(nil, ledger.ReconciliationReport{
		Provider:  "tencent",
		Status:    "pass",
		CreatedAt: time.Now().UTC().Add(-48 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}
	server := NewServer(store)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/reconciliation/guard?maxAgeHours=30", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("guard status = %d body=%s", rec.Code, rec.Body.String())
	}
	var guard ledger.ReconciliationGuard
	if err := json.Unmarshal(rec.Body.Bytes(), &guard); err != nil {
		t.Fatalf("decode guard: %v", err)
	}
	if guard.Status != "blocked" || guard.Reason != "billing_reconciliation_report_stale" {
		t.Fatalf("guard = %+v", guard)
	}
}

func TestReconciliationGuardBlocksWhenReportFailed(t *testing.T) {
	store := ledger.NewMemoryStore()
	_, err := store.AppendReconciliationReport(nil, ledger.ReconciliationReport{
		Provider:  "tencent",
		Status:    "fail",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
		Payload: map[string]any{
			"lines": []any{map[string]any{"workspaceId": "ws_1", "status": "fail"}},
		},
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}
	server := NewServer(store)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/reconciliation/guard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("guard status = %d body=%s", rec.Code, rec.Body.String())
	}
	var guard ledger.ReconciliationGuard
	if err := json.Unmarshal(rec.Body.Bytes(), &guard); err != nil {
		t.Fatalf("decode guard: %v", err)
	}
	if guard.Status != "blocked" || guard.Reason != "tencent_bill_reconciliation_failed" {
		t.Fatalf("guard = %+v", guard)
	}
}

func TestReconciliationGuardAllowsWhenReportPassedRecently(t *testing.T) {
	store := ledger.NewMemoryStore()
	_, err := store.AppendReconciliationReport(nil, ledger.ReconciliationReport{
		Provider:  "tencent",
		Status:    "pass",
		CreatedAt: time.Now().UTC().Add(-1 * time.Hour),
	})
	if err != nil {
		t.Fatalf("seed report: %v", err)
	}
	server := NewServer(store)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/billing/reconciliation/guard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("guard status = %d body=%s", rec.Code, rec.Body.String())
	}
	var guard ledger.ReconciliationGuard
	if err := json.Unmarshal(rec.Body.Bytes(), &guard); err != nil {
		t.Fatalf("decode guard: %v", err)
	}
	if guard.Status != "ok" || guard.BlockNewWorkspaces || guard.Reason != "billing_reconciliation_ok" {
		t.Fatalf("guard = %+v", guard)
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

type holdAPIResponse struct {
	code   int
	body   string
	result struct {
		Wallet      wallet.Snapshot    `json:"wallet"`
		Entry       ledger.Entry       `json:"entry"`
		Transaction wallet.Transaction `json:"transaction"`
		Created     bool               `json:"created"`
	}
}

type holdReleaseAPIResponse struct {
	code   int
	body   string
	result struct {
		Wallet       wallet.Snapshot      `json:"wallet"`
		Entries      []ledger.Entry       `json:"entries"`
		Transactions []wallet.Transaction `json:"transactions"`
		Created      bool                 `json:"created"`
	}
}

type settlementAPIResponse struct {
	code   int
	body   string
	result struct {
		Wallet       wallet.Snapshot      `json:"wallet"`
		Entries      []ledger.Entry       `json:"entries"`
		Transactions []wallet.Transaction `json:"transactions"`
		UnpaidCents  int64                `json:"unpaidCents"`
		Created      bool                 `json:"created"`
	}
}

type resourceUsageAPIResponse struct {
	code   int
	body   string
	result struct {
		Log     usage.ResourceUsageLog `json:"log"`
		Created bool                   `json:"created"`
	}
}

type requestQuotaAPIResponse struct {
	code   int
	body   string
	record ledger.RequestQuotaRecord
}

type requestQuotasAPIResponse struct {
	code    int
	body    string
	records []ledger.RequestQuotaRecord
}

type requestUsageLogsAPIResponse struct {
	code int
	body string
	logs []ledger.RequestUsageLog
}

type resourceUsageLogsAPIResponse struct {
	code int
	body string
	logs []usage.ResourceUsageLog
}

type evidenceAPIResponse struct {
	code   int
	body   string
	record ledger.EvidenceRecord
}

type walletTransactionsAPIResponse struct {
	code         int
	body         string
	transactions []wallet.Transaction
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

func putRequestQuota(t *testing.T, server http.Handler, body []byte) requestQuotaAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/v1/billing/request-quotas", bytes.NewReader(body)))
	response := requestQuotaAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.record); err != nil {
			t.Fatalf("decode request quota response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func getRequestQuotas(t *testing.T, server http.Handler, target string) requestQuotasAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	response := requestQuotasAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.records); err != nil {
			t.Fatalf("decode request quotas response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func getRequestUsageLogs(t *testing.T, server http.Handler, target string) requestUsageLogsAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	response := requestUsageLogsAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.logs); err != nil {
			t.Fatalf("decode request usage logs response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func getResourceUsageLogs(t *testing.T, server http.Handler, target string) resourceUsageLogsAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	response := resourceUsageLogsAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.logs); err != nil {
			t.Fatalf("decode resource usage logs response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func getWalletTransactions(t *testing.T, server http.Handler, target string) walletTransactionsAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	response := walletTransactionsAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.transactions); err != nil {
			t.Fatalf("decode wallet transactions response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	return payload
}

func postResourceUsage(t *testing.T, server http.Handler, body []byte) resourceUsageAPIResponse {
	return postResourceUsageTo(t, server, "/api/v1/billing/resource-usage", body)
}

func postResourceUsageTo(t *testing.T, server http.Handler, target string, body []byte) resourceUsageAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)))
	response := resourceUsageAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.result); err != nil {
			t.Fatalf("decode resource usage response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func postSettlement(t *testing.T, server http.Handler, body []byte) settlementAPIResponse {
	return postSettlementTo(t, server, "/api/v1/billing/settlements", body)
}

func postSettlementTo(t *testing.T, server http.Handler, target string, body []byte) settlementAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)))
	response := settlementAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.result); err != nil {
			t.Fatalf("decode settlement response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func postHold(t *testing.T, server http.Handler, body []byte) holdAPIResponse {
	return postHoldTo(t, server, "/api/v1/billing/holds", body)
}

func postHoldTo(t *testing.T, server http.Handler, target string, body []byte) holdAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)))
	response := holdAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.result); err != nil {
			t.Fatalf("decode hold response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func postHoldRelease(t *testing.T, server http.Handler, body []byte) holdReleaseAPIResponse {
	return postHoldReleaseTo(t, server, "/api/v1/billing/holds/release", body)
}

func postHoldReleaseTo(t *testing.T, server http.Handler, target string, body []byte) holdReleaseAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)))
	response := holdReleaseAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.result); err != nil {
			t.Fatalf("decode hold release response: %v body=%s", err, rec.Body.String())
		}
	}
	return response
}

func postEvidenceTo(t *testing.T, server http.Handler, target string, body []byte) evidenceAPIResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body)))
	response := evidenceAPIResponse{code: rec.Code, body: rec.Body.String()}
	if rec.Code == http.StatusCreated || rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &response.record); err != nil {
			t.Fatalf("decode evidence response: %v body=%s", err, rec.Body.String())
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
