package migration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestDryRunPreviewReadsSingleOPLCloudStateFile(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "opl-cloud-state.json", map[string]any{
		"users": map[string]any{
			"usr_1": map[string]any{
				"id":             "usr_1",
				"accountId":      "acct_1",
				"balance":        200,
				"frozen":         0,
				"holds":          map[string]any{},
				"totalRecharged": 200,
			},
		},
		"billingLedger": []map[string]any{{
			"id":            "led_1",
			"type":          "credit",
			"accountId":     "acct_1",
			"userId":        "usr_1",
			"workspaceId":   "account",
			"sourceEventId": "console_manual_topup_1",
			"amount":        200,
			"currency":      "CNY",
		}},
		"walletTransactions": []map[string]any{{
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
		}},
		"manualTopups": []map[string]any{{
			"id":                  "topup_1",
			"operatorUserId":      "usr_admin",
			"operatorAccountId":   "acct_admin",
			"targetUserId":        "usr_1",
			"targetAccountId":     "acct_1",
			"sourceEventId":       "console_manual_topup_1",
			"reason":              "single state import",
			"amount":              200,
			"currency":            "CNY",
			"status":              "completed",
			"balanceBefore":       0,
			"balanceAfter":        200,
			"ledgerEntryId":       "led_1",
			"walletTransactionId": "wtx_1",
			"auditEventId":        "aud_1",
		}},
		"audit": []map[string]any{{
			"id":            "aud_1",
			"accountId":     "acct_1",
			"type":          "account.credit_granted",
			"targetKind":    "manual_topup",
			"targetId":      "topup_1",
			"sourceEventId": "topup_1",
		}},
	})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("report status = %q mismatches=%+v blocked=%+v", report.Status, report.Mismatches, report.BlockedReasons)
	}
	if report.RowCounts["wallets.preview.json"] != 1 || report.RowCounts["ledger_entries.preview.json"] != 1 || report.RowCounts["wallet_transactions.preview.json"] != 1 || report.RowCounts["manual_topups.preview.json"] != 1 || report.RowCounts["audit_events.preview.json"] != 1 {
		t.Fatalf("row counts = %+v", report.RowCounts)
	}
}

func TestDryRunPreviewMapsRequestUsageAndDedup(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{{
		"id":                 "led_req_1",
		"type":               "request_debit",
		"accountId":          "acct_1",
		"userId":             "usr_1",
		"workspaceId":        "ws_1",
		"sourceEventId":      "gateway_request:req_1",
		"requestFingerprint": "fp_1",
		"amountCents":        -25,
		"currency":           "CNY",
	}})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{{
		"id":            "wtx_req_1",
		"accountId":     "acct_1",
		"userId":        "usr_1",
		"workspaceId":   "ws_1",
		"type":          "debit",
		"amountCents":   -25,
		"currency":      "CNY",
		"sourceEventId": "gateway_request:req_1",
		"ledgerEntryId": "led_req_1",
		"usageLogId":    "usage_1",
	}})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{{
		"id":            "aud_req_1",
		"accountId":     "acct_1",
		"workspaceId":   "ws_1",
		"type":          "billing.request_usage_recorded",
		"targetKind":    "request_usage",
		"targetId":      "usage_1",
		"sourceEventId": "gateway_request:req_1",
	}})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{{
		"id":                   "usage_1",
		"accountId":            "acct_1",
		"userId":               "usr_1",
		"workspaceId":          "ws_1",
		"requestId":            "req_1",
		"sourceEventId":        "gateway_request:req_1",
		"requestFingerprint":   "fp_1",
		"provider":             "openai",
		"model":                "gpt-4.1-mini",
		"inputTokens":          100,
		"outputTokens":         20,
		"amount":               0.25,
		"requestedAmountCents": 25,
		"unpaidCents":          0,
		"currency":             "CNY",
		"ledgerEntryId":        "led_req_1",
	}})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", map[string]any{
		"ws_1:req_1": map[string]any{
			"id":                 "dedup_1",
			"accountId":          "acct_1",
			"userId":             "usr_1",
			"workspaceId":        "ws_1",
			"requestId":          "req_1",
			"sourceEventId":      "gateway_request:req_1",
			"requestFingerprint": "fp_1",
			"usageLogId":         "usage_1",
		},
	})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("report status = %q mismatches=%+v blocked=%+v", report.Status, report.Mismatches, report.BlockedReasons)
	}
	if report.RowCounts["request_usage_logs.preview.json"] != 1 || report.RowCounts["request_usage_dedup.preview.json"] != 1 {
		t.Fatalf("row counts = %+v", report.RowCounts)
	}
	logs := readJSONArray(t, outputDir, "request_usage_logs.preview.json")
	if logs[0]["amount_cents"] != float64(25) || logs[0]["request_fingerprint"] != "fp_1" || logs[0]["ledger_entry_id"] != "led_req_1" {
		t.Fatalf("request usage preview = %+v", logs[0])
	}
	dedup := readJSONArray(t, outputDir, "request_usage_dedup.preview.json")
	if dedup[0]["usage_log_id"] != "usage_1" || dedup[0]["workspace_id"] != "ws_1" {
		t.Fatalf("request usage dedup preview = %+v", dedup[0])
	}
}

func TestDryRunPreviewFailsOnInconsistentRequestUsageDedup(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{{
		"id":                 "usage_1",
		"workspaceId":        "ws_1",
		"requestId":          "req_1",
		"sourceEventId":      "gateway_request:req_1",
		"requestFingerprint": "fp_1",
	}})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{{
		"id":                 "dedup_1",
		"workspaceId":        "ws_2",
		"requestId":          "req_2",
		"sourceEventId":      "gateway_request:req_2",
		"requestFingerprint": "fp_2",
		"usageLogId":         "usage_1",
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "request_usage_dedup_inconsistent")
}

func TestDryRunPreviewFailsOnInconsistentRequestUsageAccountingChain(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{})
	writeJSONFile(t, inputDir, "resourceUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{{
		"id":                 "led_req_1",
		"type":               "request_debit",
		"accountId":          "acct_1",
		"workspaceId":        "ws_1",
		"sourceEventId":      "wrong_source",
		"requestFingerprint": "wrong_fp",
		"amountCents":        -24,
	}})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{{
		"id":            "wtx_req_1",
		"accountId":     "acct_1",
		"workspaceId":   "ws_1",
		"type":          "debit",
		"amountCents":   -24,
		"sourceEventId": "wrong_source",
		"ledgerEntryId": "led_req_1",
		"usageLogId":    "usage_1",
	}})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{{
		"id":                 "usage_1",
		"accountId":          "acct_1",
		"workspaceId":        "ws_1",
		"requestId":          "req_1",
		"sourceEventId":      "gateway_request:req_1",
		"requestFingerprint": "fp_1",
		"amountCents":        25,
		"ledgerEntryId":      "led_req_1",
	}})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{{
		"id":            "aud_req_1",
		"accountId":     "acct_1",
		"workspaceId":   "ws_1",
		"type":          "billing.request_usage_recorded",
		"targetKind":    "request_usage",
		"targetId":      "wrong_usage",
		"sourceEventId": "wrong_source",
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "request_usage_chain_inconsistent")
}

func TestDryRunPreviewMapsResourceUsageLogs(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})
	writeJSONFile(t, inputDir, "resourceUsageLogs.json", []map[string]any{
		{
			"id":             "res_usage_1",
			"accountId":      "acct_1",
			"userId":         "usr_1",
			"workspaceId":    "ws_1",
			"computeId":      "compute_1",
			"resourceKind":   "compute",
			"quantity":       1,
			"unit":           "hour",
			"unitPriceCents": 47,
			"amountCents":    47,
			"requestedCents": 47,
			"sourceEventId":  "resource_usage:compute_1:tick_1",
		},
		{
			"id":             "res_usage_2",
			"accountId":      "acct_1",
			"userId":         "usr_1",
			"workspaceId":    "ws_1",
			"storageId":      "storage_1",
			"attachmentId":   "attach_1",
			"resourceType":   "storage",
			"quantity":       7,
			"unit":           "gb_hour",
			"unitPriceCents": 3,
			"amount":         0.21,
			"sourceEventId":  "resource_usage:storage_1:tick_1",
		},
	})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "pass" {
		t.Fatalf("report status = %q mismatches=%+v blocked=%+v", report.Status, report.Mismatches, report.BlockedReasons)
	}
	if report.RowCounts["resource_usage_logs.preview.json"] != 2 {
		t.Fatalf("row counts = %+v", report.RowCounts)
	}
	logs := readJSONArray(t, outputDir, "resource_usage_logs.preview.json")
	if logs[0]["resource_kind"] != "compute" || logs[0]["amount_cents"] != float64(47) || logs[0]["requested_cents"] != float64(47) {
		t.Fatalf("compute resource usage preview = %+v", logs[0])
	}
	if logs[1]["resource_kind"] != "storage" || logs[1]["attachment_id"] != "attach_1" || logs[1]["amount_cents"] != float64(21) || logs[1]["requested_cents"] != float64(21) {
		t.Fatalf("storage resource usage preview = %+v", logs[1])
	}
}

func TestDryRunPreviewFailsOnInvalidResourceUsageLogs(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})
	writeJSONFile(t, inputDir, "resourceUsageLogs.json", []map[string]any{{
		"id":            "res_usage_1",
		"workspaceId":   "ws_1",
		"resourceKind":  "compute",
		"quantity":      1,
		"unit":          "hour",
		"amountCents":   47,
		"sourceEventId": "resource_usage:missing_compute:tick_1",
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "resource_usage_invalid")
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

func TestDryRunPreviewFailsOnInconsistentTopUpAccountingChain(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{{
		"id":            "led_1",
		"type":          "credit",
		"accountId":     "acct_1",
		"sourceEventId": "wrong_source",
		"amount":        199,
	}})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{{
		"id":            "wtx_1",
		"accountId":     "acct_1",
		"type":          "credit",
		"amount":        198,
		"sourceEventId": "wrong_source",
		"ledgerEntryId": "wrong_ledger",
	}})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{{
		"id":            "aud_1",
		"accountId":     "acct_1",
		"type":          "account.credit_granted",
		"targetKind":    "manual_topup",
		"targetId":      "wrong_topup",
		"sourceEventId": "wrong_topup",
	}})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{{
		"id":                  "topup_1",
		"targetAccountId":     "acct_1",
		"sourceEventId":       "console_manual_topup_1",
		"amount":              200,
		"ledgerEntryId":       "led_1",
		"walletTransactionId": "wtx_1",
		"auditEventId":        "aud_1",
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "manual_topup_chain_inconsistent")
}

func TestDryRunPreviewFailsOnInconsistentWalletSnapshot(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{{
		"id":             "usr_1",
		"accountId":      "acct_1",
		"balance":        200,
		"frozen":         50,
		"available":      160,
		"holds":          map[string]any{"compute": 30},
		"totalRecharged": 200,
	}})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "wallet_snapshot_inconsistent")
}

func TestDryRunPreviewReportsRecordIdentityForNonIntegerMoney(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{{
		"id":             "usr_fractional",
		"accountId":      "acct_fractional",
		"balance":        10.1234,
		"frozen":         0,
		"holds":          map[string]any{},
		"totalRecharged": 20,
	}})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{})
	writeJSONFile(t, inputDir, "resourceUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "non_integer_money_values")
	assertMismatchContains(t, report.Mismatches, "record=usr_fractional")
}

func TestDryRunPreviewFailsOnInconsistentWalletTransactionLedgerLink(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{})
	writeJSONFile(t, inputDir, "resourceUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{{
		"id":            "led_hold_1",
		"type":          "compute_hold",
		"accountId":     "acct_1",
		"workspaceId":   "ws_1",
		"sourceEventId": "wrong_source",
		"amountCents":   500,
	}})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{{
		"id":            "wtx_hold_1",
		"accountId":     "acct_1",
		"workspaceId":   "ws_1",
		"type":          "hold",
		"amountCents":   600,
		"sourceEventId": "compute_resource:compute_1:created",
		"ledgerEntryId": "led_hold_1",
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "wallet_transaction_ledger_inconsistent")
}

func TestDryRunPreviewFailsWhenWalletMovingLedgerHasNoWalletTransaction(t *testing.T) {
	inputDir := t.TempDir()
	outputDir := t.TempDir()
	writeJSONFile(t, inputDir, "users.json", []map[string]any{})
	writeJSONFile(t, inputDir, "manualTopups.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "requestUsageDedup.json", []map[string]any{})
	writeJSONFile(t, inputDir, "resourceUsageLogs.json", []map[string]any{})
	writeJSONFile(t, inputDir, "audit.json", []map[string]any{})
	writeJSONFile(t, inputDir, "walletTransactions.json", []map[string]any{})
	writeJSONFile(t, inputDir, "billingLedger.json", []map[string]any{{
		"id":            "led_debit_1",
		"type":          "compute_debit",
		"accountId":     "acct_1",
		"workspaceId":   "ws_1",
		"sourceEventId": "billing_tick_1:compute:available_balance",
		"amountCents":   -500,
	}})

	report, err := RunDryRun(inputDir, outputDir)
	if err != nil {
		t.Fatalf("run dry run: %v", err)
	}
	if report.Status != "fail" {
		t.Fatalf("report status = %q", report.Status)
	}
	assertContains(t, report.BlockedReasons, "wallet_moving_ledger_missing_transaction")
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

func assertContains(t *testing.T, values []string, expected string) {
	t.Helper()
	for _, value := range values {
		if value == expected {
			return
		}
	}
	t.Fatalf("expected %q in %+v", expected, values)
}

func assertMismatchContains(t *testing.T, values []string, expected string) {
	t.Helper()
	for _, value := range values {
		if strings.Contains(value, expected) {
			return
		}
	}
	t.Fatalf("expected mismatch containing %q in %+v", expected, values)
}
