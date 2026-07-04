package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunPreviewMapsWalletTopUpAccounting(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", map[string]any{
		"usr_1": map[string]any{
			"id":             "usr_1",
			"accountId":      "acct_1",
			"balance":        200,
			"frozen":         0,
			"holds":          map[string]any{},
			"totalRecharged": 200,
		},
	})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{{
		"id":            "led_1",
		"type":          "credit",
		"accountId":     "acct_1",
		"userId":        "usr_1",
		"workspaceId":   "account",
		"sourceEventId": "console_manual_topup_1",
		"amount":        200,
		"currency":      "CNY",
	}})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{{
		"id":            "wtx_1",
		"accountId":     "acct_1",
		"userId":        "usr_1",
		"workspaceId":   "account",
		"type":          "credit",
		"amount":        200,
		"currency":      "CNY",
		"sourceEventId": "console_manual_topup_1",
		"ledgerEntryId": "led_1",
		"balanceBefore": 0,
		"balanceAfter":  200,
		"frozenBefore":  0,
		"frozenAfter":   0,
	}})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{{
		"id":                  "topup_1",
		"operatorUserId":      "usr_admin",
		"operatorAccountId":   "acct_admin",
		"targetUserId":        "usr_1",
		"targetAccountId":     "acct_1",
		"sourceEventId":       "console_manual_topup_1",
		"reason":              "initial launch credit",
		"amount":              200,
		"currency":            "CNY",
		"status":              "completed",
		"balanceBefore":       0,
		"balanceAfter":        200,
		"ledgerEntryId":       "led_1",
		"walletTransactionId": "wtx_1",
		"auditEventId":        "aud_1",
	}})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{{
		"id":            "aud_1",
		"accountId":     "acct_1",
		"type":          "account.credit_granted",
		"targetKind":    "manual_topup",
		"targetId":      "topup_1",
		"sourceEventId": "topup_1",
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("report status = %q mismatches=%+v blocked=%+v", report.Status, report.Mismatches, report.BlockedReasons)
	}
	if report.RowCounts["wallets.preview.json"] != 1 || report.RowCounts["manual_topups.preview.json"] != 1 || report.RowCounts["audit_events.preview.json"] != 1 {
		t.Fatalf("row counts = %+v", report.RowCounts)
	}

	wallets := readJSONArray(t, outputDir, "wallets.preview.json")
	if wallets[0]["balance_cents"] != float64(20000) || wallets[0]["total_recharged_cents"] != float64(20000) {
		t.Fatalf("wallet preview = %+v", wallets[0])
	}
	topups := readJSONArray(t, outputDir, "manual_topups.preview.json")
	if topups[0]["source_event_id"] != "console_manual_topup_1" || topups[0]["amount_cents"] != float64(20000) {
		t.Fatalf("topup preview = %+v", topups[0])
	}
	payload := topups[0]["payload"].(map[string]any)
	if payload["reason"] != "initial launch credit" {
		t.Fatalf("topup payload = %+v", payload)
	}
	ledgerEntries := readJSONArray(t, outputDir, "ledger_entries.preview.json")
	if ledgerEntries[0]["amount_cents"] != float64(20000) {
		t.Fatalf("ledger entry preview = %+v", ledgerEntries[0])
	}
	transactions := readJSONArray(t, outputDir, "wallet_transactions.preview.json")
	if transactions[0]["amount_cents"] != float64(20000) || transactions[0]["available_after_cents"] != float64(20000) {
		t.Fatalf("wallet transaction preview = %+v", transactions[0])
	}
	auditEvents := readJSONArray(t, outputDir, "audit_events.preview.json")
	if auditEvents[0]["action"] != "account.credit_granted" || auditEvents[0]["source_event_id"] != "topup_1" {
		t.Fatalf("audit event preview = %+v", auditEvents[0])
	}
}

func TestDryRunPreviewFailsOnDuplicateTopUpSourceAndMissingReferences(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{
		{
			"id":                  "topup_1",
			"targetAccountId":     "acct_1",
			"sourceEventId":       "dup_source",
			"amount":              200,
			"ledgerEntryId":       "missing_ledger",
			"walletTransactionId": "missing_wtx",
			"auditEventId":        "missing_audit",
		},
		{
			"id":              "topup_2",
			"targetAccountId": "acct_1",
			"sourceEventId":   "dup_source",
			"amount":          100,
		},
	})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	if len(report.Mismatches) == 0 || len(report.BlockedReasons) == 0 {
		t.Fatalf("expected mismatches and blocked reasons, got %+v", report)
	}
}

func writeJSONFile(t *testing.T, dir string, name string, value any) {
	t.Helper()
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), payload, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readJSONArray(t *testing.T, dir string, name string) []map[string]any {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var out []map[string]any
	if err := json.Unmarshal(payload, &out); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return out
}
