package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
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
