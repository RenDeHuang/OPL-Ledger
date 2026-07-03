package ledger

import (
	"context"
	"testing"
)

func TestMemoryStoreAppendIsIdempotentBySourceEvent(t *testing.T) {
	store := NewMemoryStore()
	input := AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		ComputeID:     "compute_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	}
	first, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected idempotent ID %q, got %q", first.ID, second.ID)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreAppendIsIdempotentByRequestFingerprint(t *testing.T) {
	store := NewMemoryStore()
	input := AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		WorkspaceID:        "ws_1",
		ComputeID:          "compute_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	}
	first, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected idempotent ID %q, got %q", first.ID, second.ID)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreAppendBindsRequestFingerprintToExistingSourceEvent(t *testing.T) {
	store := NewMemoryStore()
	first, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("mixed retry append: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected mixed retry ID %q, got %q", first.ID, second.ID)
	}
	third, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("request fingerprint retry append: %v", err)
	}
	if first.ID != third.ID {
		t.Fatalf("expected bound request fingerprint ID %q, got %q", first.ID, third.ID)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreAppendBindsSourceEventToExistingRequestFingerprint(t *testing.T) {
	store := NewMemoryStore()
	first, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("mixed retry append: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected mixed retry ID %q, got %q", first.ID, second.ID)
	}
	third, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	})
	if err != nil {
		t.Fatalf("source event retry append: %v", err)
	}
	if first.ID != third.ID {
		t.Fatalf("expected bound source event ID %q, got %q", first.ID, third.ID)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreAppendRejectsDifferentFingerprintForExistingSourceEvent(t *testing.T) {
	store := NewMemoryStore()
	first, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_1",
		RequestFingerprint: "fp_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	_, err = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_1",
		RequestFingerprint: "fp_2",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err == nil {
		t.Fatalf("expected conflicting request fingerprint error")
	}
	if _, ok := store.byRequestFingerprint["fp_2"]; ok {
		t.Fatalf("conflicting request fingerprint was bound")
	}
	sourceRetry, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	})
	if err != nil {
		t.Fatalf("source event retry append: %v", err)
	}
	if sourceRetry.ID != first.ID {
		t.Fatalf("expected source event retry ID %q, got %q", first.ID, sourceRetry.ID)
	}
	if sourceRetry.RequestFingerprint != "fp_1" {
		t.Fatalf("expected original request fingerprint fp_1, got %q", sourceRetry.RequestFingerprint)
	}
	fingerprintRetry, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		RequestFingerprint: "fp_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("request fingerprint retry append: %v", err)
	}
	if fingerprintRetry.ID != first.ID {
		t.Fatalf("expected request fingerprint retry ID %q, got %q", first.ID, fingerprintRetry.ID)
	}
	if fingerprintRetry.SourceEventID != "evt_1" {
		t.Fatalf("expected original source event evt_1, got %q", fingerprintRetry.SourceEventID)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreAppendRejectsDifferentSourceForExistingRequestFingerprint(t *testing.T) {
	store := NewMemoryStore()
	first, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_1",
		RequestFingerprint: "fp_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	_, err = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_2",
		RequestFingerprint: "fp_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err == nil {
		t.Fatalf("expected conflicting source event error")
	}
	if _, ok := store.bySourceEvent["evt_2"]; ok {
		t.Fatalf("conflicting source event was bound")
	}
	fingerprintRetry, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		RequestFingerprint: "fp_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("request fingerprint retry append: %v", err)
	}
	if fingerprintRetry.ID != first.ID {
		t.Fatalf("expected request fingerprint retry ID %q, got %q", first.ID, fingerprintRetry.ID)
	}
	if fingerprintRetry.SourceEventID != "evt_1" {
		t.Fatalf("expected original source event evt_1, got %q", fingerprintRetry.SourceEventID)
	}
	sourceRetry, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	})
	if err != nil {
		t.Fatalf("source event retry append: %v", err)
	}
	if sourceRetry.ID != first.ID {
		t.Fatalf("expected source event retry ID %q, got %q", first.ID, sourceRetry.ID)
	}
	if sourceRetry.RequestFingerprint != "fp_1" {
		t.Fatalf("expected original request fingerprint fp_1, got %q", sourceRetry.RequestFingerprint)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreAppendRejectsConflictingIdempotencyKeys(t *testing.T) {
	store := NewMemoryStore()
	sourceEntry, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	})
	if err != nil {
		t.Fatalf("source event append: %v", err)
	}
	fingerprintEntry, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err != nil {
		t.Fatalf("request fingerprint append: %v", err)
	}
	if sourceEntry.ID == fingerprintEntry.ID {
		t.Fatalf("expected setup entries to be distinct")
	}
	_, err = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:          "compute_debit",
		AccountID:          "acct_1",
		SourceEventID:      "evt_1",
		RequestFingerprint: "req_1",
		AmountCents:        390,
		Currency:           "CNY",
	})
	if err == nil {
		t.Fatalf("expected conflicting idempotency keys error")
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected no conflicting append, got %d entries", len(entries))
	}
}

func TestMemoryStoreSummaryTotalsAccountBalance(t *testing.T) {
	store := NewMemoryStore()
	_, _ = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType: "manual_topup", AccountID: "acct_1", SourceEventID: "topup_1", AmountCents: 1000, Currency: "CNY",
	})
	_, _ = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType: "compute_debit", AccountID: "acct_1", SourceEventID: "debit_1", AmountCents: -390, Currency: "CNY",
	})
	summary, err := store.Summary(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.BalanceCents != 610 {
		t.Fatalf("expected balance 610, got %d", summary.BalanceCents)
	}
}
