package reconciliation

import "testing"

func TestTencentReconciliationPassesWhenLedgerCoversCostPlusMarkup(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0.20,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: -1200}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "pass" {
		t.Fatalf("expected pass, got %s diff=%d", report.Status, report.DifferenceCents)
	}
	if report.LedgerAmountCents != 1200 {
		t.Fatalf("expected normalized ledger amount 1200, got %d", report.LedgerAmountCents)
	}
	if report.DifferenceCents != 0 {
		t.Fatalf("expected 0 difference, got %d", report.DifferenceCents)
	}
}

func TestTencentReconciliationFailsWhenPositiveLedgerRowMatchesCostPlusMarkup(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0.20,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1200}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s diff=%d", report.Status, report.DifferenceCents)
	}
	if report.LedgerAmountCents != 0 {
		t.Fatalf("expected positive ledger amount to be excluded, got %d", report.LedgerAmountCents)
	}
	if len(report.Lines) != 1 {
		t.Fatalf("expected 1 line report, got %d", len(report.Lines))
	}
	line := report.Lines[0]
	if line.Status != "fail" {
		t.Fatalf("expected line fail, got %s", line.Status)
	}
	if line.InvalidLedgerRows != 1 {
		t.Fatalf("expected 1 invalid ledger row, got %d", line.InvalidLedgerRows)
	}
}

func TestTencentReconciliationFailsWhenLedgerUndercharges(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0.20,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: -1000}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s", report.Status)
	}
	if report.DifferenceCents != -200 {
		t.Fatalf("expected -200 difference, got %d", report.DifferenceCents)
	}
}

func TestTencentReconciliationFailsWhenGlobalOverchargeHidesPerResourceUndercharge(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate: 0.20,
		LedgerRows: []LedgerRow{
			{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: -2400},
			{WorkspaceID: "ws_2", ResourceType: "storage", AmountCents: -1000},
		},
		TencentRows: []TencentRow{
			{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000},
			{WorkspaceID: "ws_2", ResourceType: "storage", AmountCents: 1000},
		},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s diff=%d", report.Status, report.DifferenceCents)
	}
	if report.DifferenceCents != 1000 {
		t.Fatalf("expected aggregate overcharge difference 1000, got %d", report.DifferenceCents)
	}
	if len(report.Lines) != 2 {
		t.Fatalf("expected 2 line reports, got %d", len(report.Lines))
	}

	var undercharged LineReport
	for _, line := range report.Lines {
		if line.WorkspaceID == "ws_2" && line.ResourceType == "storage" {
			undercharged = line
		}
	}
	if undercharged.WorkspaceID == "" {
		t.Fatal("expected ws_2/storage line report")
	}
	if undercharged.Status != "fail" || undercharged.DifferenceCents != -200 {
		t.Fatalf("expected ws_2/storage to fail with -200 difference, got status=%s diff=%d", undercharged.Status, undercharged.DifferenceCents)
	}
}

func TestTencentReconciliationPassesOneCentUnderchargeTolerance(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: -999}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "pass" {
		t.Fatalf("expected pass, got %s diff=%d", report.Status, report.DifferenceCents)
	}
	if report.DifferenceCents != -1 {
		t.Fatalf("expected -1 difference, got %d", report.DifferenceCents)
	}
}

func TestTencentReconciliationFailsTwoCentUndercharge(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: -998}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s diff=%d", report.Status, report.DifferenceCents)
	}
	if report.DifferenceCents != -2 {
		t.Fatalf("expected -2 difference, got %d", report.DifferenceCents)
	}
}

func TestNormalizeTencentBillRowsReadsWorkspaceIDFromTags(t *testing.T) {
	rows, err := NormalizeTencentBillRows([]RawTencentBillRow{
		{
			"ProductName":      "Tencent Kubernetes Engine",
			"RealTotalCost":    12.34,
			"Currency":         "CNY",
			"ResourceId":       "ins_1",
			"Tags":             "workspace_id=ws_1;owner=opl",
			"BillingMonth":     "2026-07",
			"AvailabilityZone": "ap-shanghai-1",
		},
	})
	if err != nil {
		t.Fatalf("normalize rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 normalized row, got %d", len(rows))
	}
	row := rows[0]
	if row.WorkspaceID != "ws_1" || row.ResourceType != "compute" || row.AmountCents != 1234 || row.Currency != "CNY" || row.SourceResourceID != "ins_1" {
		t.Fatalf("row = %+v", row)
	}
}

func TestNormalizeTencentBillRowsFailsClosedWhenWorkspaceIDMissing(t *testing.T) {
	_, err := NormalizeTencentBillRows([]RawTencentBillRow{
		{
			"ProductName":   "Cloud Block Storage",
			"RealTotalCost": 8.00,
			"Currency":      "CNY",
			"ResourceId":    "disk_1",
			"Tags":          "owner=opl",
		},
	})
	if err == nil || err.Error() != "tencent_bill_workspace_id_missing:disk_1" {
		t.Fatalf("error = %v", err)
	}
}

func TestTencentReconciliationFailsClosedForMixedCurrency(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0.20,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: -1200, Currency: "CNY"}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000, Currency: "USD"}},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s", report.Status)
	}
	if report.FailureReason != "mixed_currency_not_supported" {
		t.Fatalf("failure reason = %q", report.FailureReason)
	}
}
