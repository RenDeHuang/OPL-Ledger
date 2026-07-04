package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestInitialMigrationDefinesLedgerTables(t *testing.T) {
	sqlBytes, err := Migrations.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(sqlBytes)
	required := []string{
		"CREATE TABLE IF NOT EXISTS ledger_entries",
		"CREATE TABLE IF NOT EXISTS audit_events",
		"CREATE TABLE IF NOT EXISTS evidence_records",
		"CREATE TABLE IF NOT EXISTS task_receipts",
		"CREATE TABLE IF NOT EXISTS wallets",
		"CREATE TABLE IF NOT EXISTS request_usage_logs",
		"CREATE TABLE IF NOT EXISTS resource_usage_logs",
		"CREATE TABLE IF NOT EXISTS wallet_transactions",
		"CREATE TABLE IF NOT EXISTS manual_topups",
		"CREATE TABLE IF NOT EXISTS billing_reconciliation_reports",
		"CREATE TABLE IF NOT EXISTS idempotency_keys",
		"CREATE TABLE IF NOT EXISTS kubernetes_evidence_snapshots",
	}
	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}
}

func TestInitialMigrationDefinesWalletSnapshotTable(t *testing.T) {
	sqlBytes, err := Migrations.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(sqlBytes)
	required := []string{
		"CREATE TABLE IF NOT EXISTS wallets",
		"user_id TEXT NOT NULL",
		"account_id TEXT NOT NULL",
		"balance_cents BIGINT NOT NULL DEFAULT 0",
		"frozen_cents BIGINT NOT NULL DEFAULT 0",
		"total_recharged_cents BIGINT NOT NULL DEFAULT 0",
		"holds JSONB NOT NULL DEFAULT '{}'::jsonb",
		"CREATE UNIQUE INDEX IF NOT EXISTS wallets_account_id_idx",
		"CREATE INDEX IF NOT EXISTS wallets_user_id_idx",
	}
	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}
}

func TestInitialMigrationDefinesWalletTransactionAndTopUpAuditFields(t *testing.T) {
	sqlBytes, err := Migrations.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(sqlBytes)
	walletTransactions := tableBlock(t, sql, "wallet_transactions")
	requiredWalletFields := []string{
		"account_id TEXT",
		"workspace_id TEXT",
		"source_event_id TEXT",
		"ledger_entry_id TEXT NOT NULL REFERENCES ledger_entries(id)",
		"usage_log_id TEXT",
		"funding_source TEXT",
		"balance_before_cents BIGINT NOT NULL DEFAULT 0",
		"balance_after_cents BIGINT NOT NULL DEFAULT 0",
		"frozen_before_cents BIGINT NOT NULL DEFAULT 0",
		"frozen_after_cents BIGINT NOT NULL DEFAULT 0",
		"available_after_cents BIGINT NOT NULL DEFAULT 0",
	}
	for _, needle := range requiredWalletFields {
		if !strings.Contains(walletTransactions, needle) {
			t.Fatalf("wallet_transactions migration missing %q", needle)
		}
	}
	manualTopups := tableBlock(t, sql, "manual_topups")
	requiredTopupFields := []string{
		"operator_account_id TEXT",
		"target_user_id TEXT",
		"target_account_id TEXT NOT NULL",
		"ledger_entry_id TEXT NOT NULL REFERENCES ledger_entries(id)",
		"wallet_transaction_id TEXT NOT NULL REFERENCES wallet_transactions(id)",
		"audit_event_id TEXT NOT NULL REFERENCES audit_events(id)",
		"status TEXT NOT NULL",
	}
	for _, needle := range requiredTopupFields {
		if !strings.Contains(manualTopups, needle) {
			t.Fatalf("manual_topups migration missing %q", needle)
		}
	}
	requiredIndexes := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS manual_topups_source_event_idx",
		"CREATE INDEX IF NOT EXISTS wallet_transactions_account_id_idx",
	}
	for _, needle := range requiredIndexes {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}
}

func TestInitialMigrationDefinesRequestUsageDedupAndBillingFields(t *testing.T) {
	sqlBytes, err := Migrations.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(sqlBytes)
	required := []string{
		"request_id TEXT",
		"source_event_id TEXT",
		"provider TEXT",
		"model TEXT",
		"input_tokens BIGINT NOT NULL DEFAULT 0",
		"output_tokens BIGINT NOT NULL DEFAULT 0",
		"amount_cents BIGINT NOT NULL DEFAULT 0",
		"requested_amount_cents BIGINT NOT NULL DEFAULT 0",
		"unpaid_cents BIGINT NOT NULL DEFAULT 0",
		"ledger_entry_id TEXT REFERENCES ledger_entries(id)",
		"CREATE TABLE IF NOT EXISTS request_usage_dedup",
		"CREATE UNIQUE INDEX IF NOT EXISTS request_usage_dedup_source_idx",
		"CREATE UNIQUE INDEX IF NOT EXISTS request_usage_dedup_request_idx",
	}
	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}
}

func TestInitialMigrationUsesTextIDsForGeneratedLedgerIDs(t *testing.T) {
	sqlBytes, err := Migrations.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(sqlBytes)
	required := []string{
		"id TEXT PRIMARY KEY",
		"ledger_entry_id TEXT REFERENCES ledger_entries(id)",
	}
	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}
}

func TestRunMigrationsExecutesEmbeddedSQL(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS ledger_entries").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	if err := RunMigrations(context.Background(), db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
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

func tableBlock(t *testing.T, sql string, table string) string {
	t.Helper()
	startNeedle := "CREATE TABLE IF NOT EXISTS " + table + " ("
	start := strings.Index(sql, startNeedle)
	if start == -1 {
		t.Fatalf("migration missing table %q", table)
	}
	remaining := sql[start+len(startNeedle):]
	end := strings.Index(remaining, "\n);")
	if end == -1 {
		t.Fatalf("migration table %q missing closing delimiter", table)
	}
	return remaining[:end]
}
