package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/RenDeHuang/OPL-Ledger/internal/usage"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
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

func TestPostgresStoreManualTopUpWritesAccountingLoopInOneTransaction(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("owner_credit_1", "").
		WillReturnRows(ledgerEntryRows())
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows())
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "credit", "acct_1", "usr_1", "account", "", "", "", "owner_credit_1", "", int64(25000), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "account", "credit", int64(25000), "CNY", "owner_credit_1", sqlmock.AnyArg(), "", "", int64(0), int64(25000), int64(0), int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_events`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "", "usr_admin", "account.credit_granted", "manual_topup", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO manual_topups`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "usr_admin", "acct_admin", "usr_1", "acct_1", "owner_credit_1", int64(25000), "CNY", "completed", int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(25000), int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.ManualTopUp(context.Background(), ManualTopUpInput{
		AccountID:         "acct_1",
		UserID:            "usr_1",
		AmountCents:       25000,
		Reason:            "owner_credit_1",
		OperatorUserID:    "usr_admin",
		OperatorAccountID: "acct_admin",
	})
	if err != nil {
		t.Fatalf("manual topup: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Wallet.BalanceCents != 25000 || result.Transaction.LedgerEntryID != result.Entry.ID || result.TopUp.AuditEventID != result.AuditEvent.ID {
		t.Fatalf("unexpected accounting result: %+v", result)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreManualTopUpSeparatesSourceEventIDFromReason(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("console_manual_topup_1", "").
		WillReturnRows(ledgerEntryRows())
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows())
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "credit", "acct_1", "usr_1", "account", "", "", "", "console_manual_topup_1", "", int64(25000), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "account", "credit", int64(25000), "CNY", "console_manual_topup_1", sqlmock.AnyArg(), "", "", int64(0), int64(25000), int64(0), int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_events`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "", "usr_admin", "account.credit_granted", "manual_topup", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO manual_topups`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "usr_admin", "acct_admin", "usr_1", "acct_1", "console_manual_topup_1", int64(25000), "CNY", "completed", int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(25000), int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.ManualTopUp(context.Background(), ManualTopUpInput{
		AccountID:         "acct_1",
		UserID:            "usr_1",
		AmountCents:       25000,
		SourceEventID:     "console_manual_topup_1",
		Reason:            "initial launch credit",
		OperatorUserID:    "usr_admin",
		OperatorAccountID: "acct_admin",
	})
	if err != nil {
		t.Fatalf("manual topup: %v", err)
	}
	if result.Entry.SourceEventID != "console_manual_topup_1" || result.Transaction.SourceEventID != "console_manual_topup_1" {
		t.Fatalf("unexpected source event ids: %+v", result)
	}
	if result.TopUp.SourceEventID != "console_manual_topup_1" || result.TopUp.Reason != "initial launch credit" {
		t.Fatalf("unexpected topup fields: %+v", result.TopUp)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreManualTopUpReplayReturnsExistingAccountingLoop(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	transaction := walletTransactionFixture(t, "wtx_1", "led_1", "owner_credit_1", createdAt)
	topup := manualTopUpFixture("topup_1", "led_1", transaction.ID, "aud_1", createdAt)
	audit := auditFixture("aud_1", topup.ID, createdAt)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("owner_credit_1", "").
		WillReturnRows(ledgerEntryRows().AddRow("led_1", "credit", "acct_1", "usr_1", "account", "", "", "", "owner_credit_1", "", int64(25000), "CNY", createdAt))
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(25000), int64(0), int64(25000), []byte(`{}`), createdAt, createdAt))
	mock.ExpectQuery(`SELECT payload\s+FROM wallet_transactions`).
		WithArgs("owner_credit_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, transaction)))
	mock.ExpectQuery(`SELECT payload\s+FROM manual_topups`).
		WithArgs("owner_credit_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, topup)))
	mock.ExpectQuery(`SELECT payload\s+FROM audit_events`).
		WithArgs(topup.ID).
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, audit)))
	mock.ExpectCommit()

	result, err := store.ManualTopUp(context.Background(), ManualTopUpInput{
		AccountID:         "acct_1",
		UserID:            "usr_1",
		AmountCents:       25000,
		Reason:            "owner_credit_1",
		OperatorUserID:    "usr_admin",
		OperatorAccountID: "acct_admin",
	})
	if err != nil {
		t.Fatalf("manual topup replay: %v", err)
	}
	if result.Created {
		t.Fatalf("expected replay result")
	}
	if result.Wallet.BalanceCents != 25000 || result.Entry.ID != "led_1" || result.Transaction.ID != "wtx_1" || result.TopUp.ID != "topup_1" || result.AuditEvent.ID != "aud_1" {
		t.Fatalf("unexpected replay result: %+v", result)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListManualTopUpsFiltersByAccountAndSourceEvent(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	topup := manualTopUpFixture("topup_1", "led_1", "wtx_1", "aud_1", createdAt)
	topup.SourceEventID = "console_manual_topup_1"
	topup.Reason = "initial launch credit"

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT payload FROM manual_topups WHERE account_id = $1 AND source_event_id = $2 ORDER BY created_at, id`)).
		WithArgs("acct_1", "console_manual_topup_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, topup)))

	topups, err := store.ListManualTopUps(context.Background(), ManualTopUpFilter{
		AccountID:     "acct_1",
		SourceEventID: "console_manual_topup_1",
	})
	if err != nil {
		t.Fatalf("list manual topups: %v", err)
	}
	if len(topups) != 1 {
		t.Fatalf("expected 1 topup, got %d", len(topups))
	}
	if topups[0].SourceEventID != "console_manual_topup_1" || topups[0].Reason != "initial launch credit" {
		t.Fatalf("unexpected topup: %+v", topups[0])
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreRecordRequestUsageWritesDedupAndDebitLoopInOneTransaction(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	quotaLimit := int64(2)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT usage_log_id, request_fingerprint\s+FROM request_usage_dedup`).
		WithArgs("ws_1", "gateway_req_1", "req_1").
		WillReturnRows(sqlmock.NewRows([]string{"usage_log_id", "request_fingerprint"}))
	mock.ExpectExec(`INSERT INTO request_usage_dedup`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "req_1", "gateway_req_1", "fp_1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(25000), int64(0), int64(25000), []byte(`{}`), createdAt, createdAt))
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "request_debit", "acct_1", "usr_1", "ws_1", "", "", "", "gateway_req_1", "fp_1", int64(-25), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "debit", int64(-25), "CNY", "gateway_req_1", sqlmock.AnyArg(), sqlmock.AnyArg(), "available_balance", int64(25000), int64(24975), int64(0), int64(0), int64(24975), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO request_usage_logs`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "req_1", "gateway_req_1", "fp_1", "openai", "gpt-5", int64(1000), int64(500), int64(25), int64(25), int64(0), "CNY", sqlmock.AnyArg(), int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE request_usage_dedup`).
		WithArgs("acct_1", "usr_1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_events`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "ws_1", "usr_1", "billing.request_usage_recorded", "request_usage", sqlmock.AnyArg(), "gateway_req_1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(24975), int64(0), int64(25000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.RecordRequestUsage(context.Background(), RequestUsageInput{
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		RequestID:          "req_1",
		Provider:           "openai",
		Model:              "gpt-5",
		InputTokens:        1000,
		OutputTokens:       500,
		AmountCents:        25,
		SourceEventID:      "gateway_req_1",
		RequestFingerprint: "fp_1",
		RequestQuota:       &usage.RequestQuota{Limit: &quotaLimit},
	})
	if err != nil {
		t.Fatalf("record request usage: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Wallet.BalanceCents != 24975 || result.Log.AmountCents != 25 || result.Transaction.UsageLogID != result.Log.ID || result.Entry.AmountCents != -25 {
		t.Fatalf("unexpected request usage result: %+v", result)
	}
	if result.Log.Quota == nil || result.Log.Quota.Used != 1 {
		t.Fatalf("quota result = %+v", result.Log.Quota)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreRecordRequestUsageQuotaExceededRollsBackBeforeMutation(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	limit := int64(0)

	mock.ExpectBegin()
	mock.ExpectRollback()

	_, err := store.RecordRequestUsage(context.Background(), RequestUsageInput{
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		RequestID:          "req_1",
		AmountCents:        25,
		SourceEventID:      "gateway_req_1",
		RequestFingerprint: "fp_1",
		RequestQuota:       &usage.RequestQuota{Limit: &limit},
	})
	if !errors.Is(err, usage.ErrRequestQuotaExceeded) {
		t.Fatalf("expected quota exceeded, got %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreCreateHoldWritesAccountingLoopInOneTransaction(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("compute_resource:compute_1:created", "").
		WillReturnRows(ledgerEntryRows())
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(1000), int64(0), int64(1000), []byte(`{}`), createdAt, createdAt))
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "compute_hold", "acct_1", "usr_1", "ws_1", "compute_1", "", "", "compute_resource:compute_1:created", "", int64(600), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "hold", int64(600), "CNY", "compute_resource:compute_1:created", sqlmock.AnyArg(), "", "", int64(1000), int64(1000), int64(0), int64(600), int64(400), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(1000), int64(600), int64(1000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.CreateHold(context.Background(), HoldInput{
		AccountID:     "acct_1",
		UserID:        "usr_1",
		WorkspaceID:   "ws_1",
		HoldType:      "compute",
		AmountCents:   600,
		SourceEventID: "compute_resource:compute_1:created",
		ResourceID:    "compute_1",
		PackageID:     "basic",
	})
	if err != nil {
		t.Fatalf("create hold: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Wallet.FrozenCents != 600 || result.Wallet.AvailableCents != 400 || result.Entry.EventType != "compute_hold" || result.Transaction.Type != wallet.TransactionHold {
		t.Fatalf("unexpected hold result: %+v", result)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreReleaseHoldsWritesAccountingLoopInOneTransaction(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`)).
		WithArgs("compute_resource:compute_1:stopped", "").
		WillReturnRows(ledgerEntryRows())
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(1000), int64(600), int64(1000), []byte(`{"compute":600}`), createdAt, createdAt))
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "compute_hold_released", "acct_1", "usr_1", "ws_1", "compute_1", "", "", "compute_resource:compute_1:stopped", "", int64(-600), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "hold_release", int64(-600), "CNY", "compute_resource:compute_1:stopped", sqlmock.AnyArg(), "", "", int64(1000), int64(1000), int64(600), int64(0), int64(1000), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(1000), int64(0), int64(1000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.ReleaseHolds(context.Background(), ReleaseHoldInput{
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		HoldTypes:     []string{"compute"},
		SourceEventID: "compute_resource:compute_1:stopped",
		ComputeID:     "compute_1",
		Reason:        "stop_compute",
	})
	if err != nil {
		t.Fatalf("release holds: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Wallet.FrozenCents != 0 || len(result.Entries) != 1 || result.Entries[0].AmountCents != -600 || len(result.Transactions) != 1 || result.Transactions[0].Type != wallet.TransactionHoldRelease {
		t.Fatalf("unexpected release result: %+v", result)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreSettleWorkspaceUsageWritesDebitsInOneTransaction(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at FROM ledger_entries WHERE source_event_id = \$1 OR source_event_id LIKE \$2 ORDER BY created_at, id`).
		WithArgs("billing_tick_1", "billing_tick_1:%").
		WillReturnRows(ledgerEntryRows())
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(1000), int64(700), int64(1000), []byte(`{"compute":700}`), createdAt, createdAt))
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "compute_debit", "acct_1", "usr_1", "ws_1", "compute_1", "", "", "billing_tick_1:compute:available_balance", "", int64(-300), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "debit", int64(-300), "CNY", "billing_tick_1:compute:available_balance", sqlmock.AnyArg(), "", "available_balance", int64(1000), int64(500), int64(700), int64(500), int64(0), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "compute_debit", "acct_1", "usr_1", "ws_1", "compute_1", "", "", "billing_tick_1:compute:compute_hold", "", int64(-200), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "debit", int64(-200), "CNY", "billing_tick_1:compute:compute_hold", sqlmock.AnyArg(), "", "compute_hold", int64(1000), int64(500), int64(700), int64(500), int64(0), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(500), int64(500), int64(1000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.SettleWorkspaceUsage(context.Background(), SettlementInput{
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		ComputeID:          "compute_1",
		SourceEventID:      "billing_tick_1",
		Hours:              1,
		ComputeActive:      true,
		ComputeHourlyCents: 500,
	})
	if err != nil {
		t.Fatalf("settle workspace usage: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Wallet.BalanceCents != 500 || result.Wallet.FrozenCents != 500 || len(result.Entries) != 2 || len(result.Transactions) != 2 {
		t.Fatalf("unexpected settlement result: %+v", result)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreRecordResourceUsageWritesPersistentLog(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)

	mock.ExpectQuery(`SELECT payload\s+FROM resource_usage_logs\s+WHERE source_event_id = \$1`).
		WithArgs("resource_usage:compute_1:billing_tick_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}))
	mock.ExpectExec(`INSERT INTO resource_usage_logs`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "compute_1", "", "", "compute", int64(1), "hour", int64(47), int64(47), int64(47), "CNY", "resource_usage:compute_1:billing_tick_1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	result, err := store.RecordResourceUsage(context.Background(), ResourceUsageInput{
		AccountID:      "acct_1",
		UserID:         "usr_1",
		WorkspaceID:    "ws_1",
		ComputeID:      "compute_1",
		ResourceKind:   usage.ResourceKindCompute,
		Quantity:       1,
		Unit:           "hour",
		UnitPriceCents: 47,
		AmountCents:    47,
		SourceEventID:  "resource_usage:compute_1:billing_tick_1",
	})
	if err != nil {
		t.Fatalf("record resource usage: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected created result")
	}
	if result.Log.ID == "" || result.Log.ComputeID != "compute_1" || result.Log.AmountCents != 47 {
		t.Fatalf("unexpected log: %+v", result.Log)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListWalletTransactionsFiltersByAccountAndType(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	transaction := wallet.Transaction{
		ID:                  "wtx_1",
		UserID:              "usr_1",
		AccountID:           "acct_1",
		WorkspaceID:         "ws_1",
		Type:                wallet.TransactionHold,
		AmountCents:         600,
		Currency:            "CNY",
		SourceEventID:       "compute_resource:compute_1:created",
		LedgerEntryID:       "led_1",
		BalanceBeforeCents:  1000,
		BalanceAfterCents:   1000,
		FrozenBeforeCents:   0,
		FrozenAfterCents:    600,
		AvailableAfterCents: 400,
		CreatedAt:           createdAt,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT payload FROM wallet_transactions WHERE account_id = $1 AND transaction_type = $2 ORDER BY created_at, id`)).
		WithArgs("acct_1", "hold").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, transaction)))

	transactions, err := store.ListWalletTransactions(context.Background(), WalletTransactionFilter{
		AccountID: "acct_1",
		Type:      wallet.TransactionHold,
	})
	if err != nil {
		t.Fatalf("list wallet transactions: %v", err)
	}
	if len(transactions) != 1 || transactions[0].ID != "wtx_1" || transactions[0].Type != wallet.TransactionHold {
		t.Fatalf("transactions = %+v", transactions)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListWalletsFiltersByAccountID(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds, created_at, updated_at FROM wallets WHERE account_id = $1 ORDER BY created_at, id`)).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(25000), int64(0), int64(25000), []byte(`{}`), createdAt, createdAt))

	wallets, err := store.ListWallets(context.Background(), WalletFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list wallets: %v", err)
	}
	if len(wallets) != 1 {
		t.Fatalf("expected 1 wallet, got %d", len(wallets))
	}
	if wallets[0].AccountID != "acct_1" || wallets[0].BalanceCents != 25000 || wallets[0].AvailableCents != 25000 || wallets[0].TotalRechargedCents != 25000 {
		t.Fatalf("unexpected wallet snapshot: %+v", wallets[0])
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListReconciliationReportsFiltersByProviderAndStatus(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	payload := map[string]any{"lines": []any{map[string]any{"workspaceId": "ws_1", "status": "fail"}}}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, provider, status, expected_amount_cents, actual_amount_cents, difference_cents, payload, created_at FROM billing_reconciliation_reports WHERE provider = $1 AND status = $2 ORDER BY created_at DESC, id DESC`)).
		WithArgs("tencent", "fail").
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"provider",
			"status",
			"expected_amount_cents",
			"actual_amount_cents",
			"difference_cents",
			"payload",
			"created_at",
		}).AddRow("rec_1", "tencent", "fail", int64(1200), int64(1100), int64(-100), mustJSON(t, payload), createdAt))

	reports, err := store.ListReconciliationReports(context.Background(), ReconciliationReportFilter{
		Provider: "tencent",
		Status:   "fail",
	})
	if err != nil {
		t.Fatalf("list reconciliation reports: %v", err)
	}
	if len(reports) != 1 || reports[0].ID != "rec_1" || reports[0].Status != "fail" || reports[0].DifferenceCents != -100 {
		t.Fatalf("reports = %+v", reports)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreUpsertRequestQuotaPersistsQuota(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	limit := int64(1)

	mock.ExpectExec(`INSERT INTO request_quotas`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	record, err := store.UpsertRequestQuota(context.Background(), RequestQuotaInput{
		AccountID:   "acct_1",
		UserID:      "usr_1",
		WorkspaceID: "ws_1",
		Quota:       usage.RequestQuota{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("upsert request quota: %v", err)
	}
	if record.ID == "" || record.Quota.Limit == nil || *record.Quota.Limit != 1 {
		t.Fatalf("record = %+v", record)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreRecordRequestUsageUsesPersistedQuotaInTransaction(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	quotaLimit := int64(1)
	storedQuota := usage.RequestQuota{Limit: &quotaLimit, Used: 0}

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT usage_log_id, request_fingerprint\s+FROM request_usage_dedup`).
		WithArgs("ws_1", "gateway_req_1", "req_1").
		WillReturnRows(sqlmock.NewRows([]string{"usage_log_id", "request_fingerprint"}))
	mock.ExpectQuery(`SELECT id, account_id, user_id, workspace_id, quota, created_at, updated_at\s+FROM request_quotas`).
		WithArgs("acct_1", "usr_1", "ws_1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "account_id", "user_id", "workspace_id", "quota", "created_at", "updated_at"}).
			AddRow("quota_1", "acct_1", "usr_1", "ws_1", mustJSON(t, storedQuota), createdAt, createdAt))
	mock.ExpectExec(`UPDATE request_quotas`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "quota_1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO request_usage_dedup`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "req_1", "gateway_req_1", "fp_1", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds`).
		WithArgs("acct_1").
		WillReturnRows(walletRows().AddRow("wal_1", "usr_1", "acct_1", int64(1000), int64(0), int64(1000), []byte(`{}`), createdAt, createdAt))
	mock.ExpectExec(`INSERT INTO ledger_entries`).
		WithArgs(sqlmock.AnyArg(), "request_debit", "acct_1", "usr_1", "ws_1", "", "", "", "gateway_req_1", "fp_1", int64(-25), "CNY", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallet_transactions`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "debit", int64(-25), "CNY", "gateway_req_1", sqlmock.AnyArg(), sqlmock.AnyArg(), "available_balance", int64(1000), int64(975), int64(0), int64(0), int64(975), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO request_usage_logs`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "usr_1", "ws_1", "req_1", "gateway_req_1", "fp_1", "", "", int64(0), int64(0), int64(25), int64(25), int64(0), "CNY", sqlmock.AnyArg(), int64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE request_usage_dedup`).
		WithArgs("acct_1", "usr_1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_events`).
		WithArgs(sqlmock.AnyArg(), "acct_1", "ws_1", "usr_1", "billing.request_usage_recorded", "request_usage", sqlmock.AnyArg(), "gateway_req_1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO wallets`).
		WithArgs(sqlmock.AnyArg(), "usr_1", "acct_1", int64(975), int64(0), int64(1000), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := store.RecordRequestUsage(context.Background(), RequestUsageInput{
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		RequestID:          "req_1",
		AmountCents:        25,
		SourceEventID:      "gateway_req_1",
		RequestFingerprint: "fp_1",
	})
	if err != nil {
		t.Fatalf("record request usage: %v", err)
	}
	if result.Log.Quota == nil || result.Log.Quota.Used != 1 {
		t.Fatalf("quota result = %+v", result.Log.Quota)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreAppendEvidenceRecordCreatesPersistentEvidenceRow(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)

	mock.ExpectExec(`INSERT INTO evidence_records`).
		WithArgs(sqlmock.AnyArg(), "workspace.created", "acct_1", "ws_1", "workspace_create_1", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	record, err := store.AppendEvidenceRecord(context.Background(), EvidenceRecordInput{
		Type:          "workspace.created",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		SourceEventID: "workspace_create_1",
		Plan:          map[string]any{"workspaceName": "Lab"},
		Approval:      map[string]any{"status": "implicit_console_policy"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
		ResourceRefs:  map[string]any{"serverId": "ins_1"},
	})
	if err != nil {
		t.Fatalf("append evidence record: %v", err)
	}
	if record.ID == "" || record.Type != "workspace.created" || record.SourceEventID != "workspace_create_1" {
		t.Fatalf("record = %+v", record)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListEvidenceRecordsFiltersByAccountWorkspaceTypeAndSourceEvent(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	record := EvidenceRecord{
		ID:            "evd_1",
		Type:          "workspace.created",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		SourceEventID: "workspace_create_1",
		Actor:         map[string]any{"type": "user", "id": "usr_1"},
		Plan:          map[string]any{"workspaceName": "Lab"},
		Approval:      map[string]any{"status": "implicit_console_policy"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
		CreatedAt:     createdAt,
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT payload FROM evidence_records WHERE account_id = $1 AND workspace_id = $2 AND evidence_type = $3 AND source_event_id = $4 ORDER BY created_at, id`)).
		WithArgs("acct_1", "ws_1", "workspace.created", "workspace_create_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, record)))

	records, err := store.ListEvidenceRecords(context.Background(), EvidenceRecordFilter{
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		Type:          "workspace.created",
		SourceEventID: "workspace_create_1",
	})
	if err != nil {
		t.Fatalf("list evidence records: %v", err)
	}
	if len(records) != 1 || records[0].ID != "evd_1" {
		t.Fatalf("records = %+v", records)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreAppendKubernetesEvidenceSnapshotStoresRedactedObject(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	collectedAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)

	mock.ExpectExec(`INSERT INTO kubernetes_evidence_snapshots`).
		WithArgs(sqlmock.AnyArg(), "cluster_1", "opl-cloud", "Deployment", "opl-ws-1", "ws_1", "42", int64(7), "ready", sqlmock.AnyArg(), collectedAt).
		WillReturnResult(sqlmock.NewResult(0, 1))

	snapshot, err := store.AppendKubernetesEvidenceSnapshot(context.Background(), KubernetesEvidenceSnapshot{
		ClusterID:          "cluster_1",
		Namespace:          "opl-cloud",
		ObjectKind:         "Deployment",
		ObjectName:         "opl-ws-1",
		WorkspaceID:        "ws_1",
		ResourceVersion:    "42",
		ObservedGeneration: 7,
		ReadinessStatus:    "ready",
		CollectedAt:        collectedAt,
		RedactedObject: map[string]any{
			"kind":          "Deployment",
			"name":          "opl-ws-1",
			"readyReplicas": int32(1),
		},
	})
	if err != nil {
		t.Fatalf("append kubernetes evidence snapshot: %v", err)
	}
	payload := string(mustJSON(t, snapshot.RedactedObject))
	if strings.Contains(payload, "secret-value") {
		t.Fatalf("redacted object leaked secret: %s", payload)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreListKubernetesEvidenceSnapshotsFiltersByWorkspaceAndKind(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	collectedAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	redactedObject := map[string]any{
		"kind":          "Deployment",
		"name":          "opl-ws-1",
		"readyReplicas": float64(1),
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT cluster_id, namespace, object_kind, object_name, workspace_id, resource_version, observed_generation, readiness_status, redacted_object, collected_at FROM kubernetes_evidence_snapshots WHERE object_kind = $1 AND workspace_id = $2 ORDER BY collected_at DESC, id DESC`)).
		WithArgs("Deployment", "ws_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"cluster_id",
			"namespace",
			"object_kind",
			"object_name",
			"workspace_id",
			"resource_version",
			"observed_generation",
			"readiness_status",
			"redacted_object",
			"collected_at",
		}).AddRow("cluster_1", "opl-cloud", "Deployment", "opl-ws-1", "ws_1", "42", int64(7), "ready", mustJSON(t, redactedObject), collectedAt))

	snapshots, err := store.ListKubernetesEvidenceSnapshots(context.Background(), KubernetesEvidenceSnapshotFilter{
		WorkspaceID: "ws_1",
		ObjectKind:  "Deployment",
	})
	if err != nil {
		t.Fatalf("list kubernetes evidence snapshots: %v", err)
	}
	if len(snapshots) != 1 || snapshots[0].ObjectName != "opl-ws-1" || snapshots[0].ReadinessStatus != "ready" {
		t.Fatalf("snapshots = %+v", snapshots)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreAppendTaskReceiptReplaysExistingSourceEvent(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	existing := TaskReceipt{
		ID:            "task_receipt_1",
		Type:          "task.evidence.v1",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		TaskID:        "task_1",
		SourceEventID: "task_source_1",
		Actor:         map[string]any{"type": "system", "id": "opl-ledger"},
		Plan:          map[string]any{"goal": "run analysis"},
		Approval:      map[string]any{"status": "approved"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
		InputRefs:     []map[string]any{},
		ExecutionRefs: []map[string]any{},
		OutputRefs:    []map[string]any{},
		ReviewResults: []map[string]any{},
		CreatedAt:     createdAt,
	}

	mock.ExpectQuery(`SELECT payload\s+FROM task_receipts`).
		WithArgs("acct_1", "ws_1", "task_1", "task_source_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, existing)))

	receipt, err := store.AppendTaskReceipt(context.Background(), TaskReceiptInput{
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		TaskID:        "task_1",
		SourceEventID: "task_source_1",
		Plan:          map[string]any{"goal": "run analysis"},
		Approval:      map[string]any{"status": "approved"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
	})
	if err != nil {
		t.Fatalf("append task receipt replay: %v", err)
	}
	if receipt.ID != "task_receipt_1" {
		t.Fatalf("receipt = %+v", receipt)
	}
	assertSQLExpectations(t, mock)
}

func TestPostgresStoreAppendTaskReceiptRejectsConflictingSourceEventReplay(t *testing.T) {
	db, mock := newMockDB(t)
	store := NewPostgresStore(db)
	createdAt := time.Date(2026, 7, 4, 10, 0, 0, 0, time.UTC)
	existing := TaskReceipt{
		ID:            "task_receipt_1",
		Type:          "task.evidence.v1",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		TaskID:        "task_1",
		SourceEventID: "task_source_1",
		Actor:         map[string]any{"type": "system", "id": "opl-ledger"},
		Plan:          map[string]any{"goal": "run analysis"},
		Approval:      map[string]any{"status": "approved"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
		InputRefs:     []map[string]any{},
		ExecutionRefs: []map[string]any{},
		OutputRefs:    []map[string]any{},
		ReviewResults: []map[string]any{},
		CreatedAt:     createdAt,
	}

	mock.ExpectQuery(`SELECT payload\s+FROM task_receipts`).
		WithArgs("acct_1", "ws_1", "task_1", "task_source_1").
		WillReturnRows(sqlmock.NewRows([]string{"payload"}).AddRow(mustJSON(t, existing)))

	_, err := store.AppendTaskReceipt(context.Background(), TaskReceiptInput{
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		TaskID:        "task_1",
		SourceEventID: "task_source_1",
		Plan:          map[string]any{"goal": "different analysis"},
		Approval:      map[string]any{"status": "approved"},
		Environment:   map[string]any{"runtimeProvider": "tencent-tke"},
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
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

func walletRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id",
		"user_id",
		"account_id",
		"balance_cents",
		"frozen_cents",
		"total_recharged_cents",
		"holds",
		"created_at",
		"updated_at",
	})
}

func walletTransactionFixture(t *testing.T, id string, ledgerEntryID string, sourceEventID string, createdAt time.Time) wallet.Transaction {
	t.Helper()
	return wallet.Transaction{
		ID:                  id,
		UserID:              "usr_1",
		AccountID:           "acct_1",
		WorkspaceID:         "account",
		Type:                wallet.TransactionCredit,
		AmountCents:         25000,
		Currency:            "CNY",
		SourceEventID:       sourceEventID,
		LedgerEntryID:       ledgerEntryID,
		BalanceBeforeCents:  0,
		BalanceAfterCents:   25000,
		FrozenBeforeCents:   0,
		FrozenAfterCents:    0,
		AvailableAfterCents: 25000,
		CreatedAt:           createdAt,
	}
}

func manualTopUpFixture(id string, ledgerEntryID string, transactionID string, auditID string, createdAt time.Time) ManualTopUp {
	return ManualTopUp{
		ID:                  id,
		OperatorUserID:      "usr_admin",
		OperatorAccountID:   "acct_admin",
		TargetUserID:        "usr_1",
		TargetAccountID:     "acct_1",
		AmountCents:         25000,
		Currency:            "CNY",
		Reason:              "owner_credit_1",
		Status:              "completed",
		BalanceBeforeCents:  0,
		BalanceAfterCents:   25000,
		LedgerEntryID:       ledgerEntryID,
		WalletTransactionID: transactionID,
		AuditEventID:        auditID,
		CreatedAt:           createdAt,
	}
}

func auditFixture(id string, sourceEventID string, createdAt time.Time) AuditEvent {
	return AuditEvent{
		ID:            id,
		AccountID:     "acct_1",
		ActorID:       "usr_admin",
		Action:        "account.credit_granted",
		TargetKind:    "manual_topup",
		TargetID:      sourceEventID,
		SourceEventID: sourceEventID,
		CreatedAt:     createdAt,
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return payload
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
