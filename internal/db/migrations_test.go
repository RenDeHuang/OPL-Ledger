package db

import (
	"strings"
	"testing"
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
