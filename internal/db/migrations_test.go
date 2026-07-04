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
