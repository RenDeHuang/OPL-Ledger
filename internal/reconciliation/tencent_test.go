package reconciliation

import "testing"

func TestTencentReconciliationPassesWhenLedgerCoversCostPlusMarkup(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0.20,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1200}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "pass" {
		t.Fatalf("expected pass, got %s diff=%d", report.Status, report.DifferenceCents)
	}
}

func TestTencentReconciliationFailsWhenLedgerUndercharges(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate:  0.20,
		LedgerRows:  []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s", report.Status)
	}
	if report.DifferenceCents != -200 {
		t.Fatalf("expected -200 difference, got %d", report.DifferenceCents)
	}
}
