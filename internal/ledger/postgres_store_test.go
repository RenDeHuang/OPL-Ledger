package ledger

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresStoreAppendEntryCreatesPersistentLedgerRow(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	input := AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		ComputeID:     "compute_1",
		SourceEventID: "evt_1",
		AmountCents:   -390,
		Currency:      "CNY",
	}

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("evt_1", "").
		WillReturnRows(ledgerEntryRows())
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "compute_debit", "acct_1", "", "ws_1", "compute_1", "", "", "evt_1", "", int64(-390), "CNY", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("append entry: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Entry.ID == "" {
		t.Fatalf("expected generated entry id")
	}
	if result.Entry.SourceEventID != "evt_1" {
		t.Fatalf("source event = %q", result.Entry.SourceEventID)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreAppendEntryReplaysExistingSourceEvent(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("evt_1", "").
		WillReturnRows(ledgerEntryRows().AddRow("led_1", "compute_debit", "acct_1", "", "ws_1", "compute_1", "", "", "evt_1", "", int64(-390), "CNY", createdAt))
	mock.ExpectCommit()

	result, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		ComputeID:     "compute_1",
		SourceEventID: "evt_1",
		AmountCents:   -390,
		Currency:      "CNY",
	})
	if err != nil {
		t.Fatalf("append replay: %v", err)
	}
	if result.Created {
		t.Fatalf("expected replay result")
	}
	if result.Entry.ID != "led_1" {
		t.Fatalf("entry id = %q", result.Entry.ID)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreAppendEntryRejectsReplayConflict(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("evt_1", "").
		WillReturnRows(ledgerEntryRows().AddRow("led_1", "compute_debit", "acct_1", "", "ws_1", "compute_1", "", "", "evt_1", "", int64(-390), "CNY", createdAt))
	mock.ExpectRollback()

	_, err := store.AppendEntry(context.Background(), AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		ComputeID:     "compute_1",
		SourceEventID: "evt_1",
		AmountCents:   -490,
		Currency:      "CNY",
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListEntriesFiltersByAccountAndWorkspace(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE account_id = $1 AND workspace_id = $2 ORDER BY created_at, id`)).
		WithArgs("acct_1", "ws_1").
		WillReturnRows(ledgerEntryRows().AddRow("led_1", "compute_debit", "acct_1", "", "ws_1", "compute_1", "", "", "evt_1", "", int64(-390), "CNY", createdAt))

	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1", WorkspaceID: "ws_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ID != "led_1" {
		t.Fatalf("entry id = %q", entries[0].ID)
	}
	assertSQLExpectations(t, mock)
}

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("mock db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db, mock
}

func ledgerEntryRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"event_type",
		"account_id",
		"user_id",
		"workspace_id",
		"compute_id",
		"storage_id",
		"attachment_id",
		"source_event_id",
		"request_fingerprint",
		"amount_cents",
		"currency",
		"created_at",
	})
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
