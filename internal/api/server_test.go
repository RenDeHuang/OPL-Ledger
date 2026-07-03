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

type ledgerAppendResponse struct {
	code  int
	body  string
	entry ledger.Entry
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
