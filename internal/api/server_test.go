package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
