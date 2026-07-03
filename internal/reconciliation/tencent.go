package reconciliation

import "math"

type LedgerRow struct {
	WorkspaceID  string
	ResourceType string
	AmountCents  int64
}

type TencentRow struct {
	WorkspaceID  string
	ResourceType string
	AmountCents  int64
}

type Input struct {
	MarkupRate  float64
	LedgerRows  []LedgerRow
	TencentRows []TencentRow
}

type Report struct {
	Status              string `json:"status"`
	LedgerAmountCents   int64  `json:"ledgerAmountCents"`
	ExpectedAmountCents int64  `json:"expectedAmountCents"`
	DifferenceCents     int64  `json:"differenceCents"`
}

func ReconcileTencentBills(input Input) Report {
	var ledgerTotal int64
	for _, row := range input.LedgerRows {
		ledgerTotal += row.AmountCents
	}
	var tencentTotal int64
	for _, row := range input.TencentRows {
		tencentTotal += row.AmountCents
	}
	expected := int64(math.Round(float64(tencentTotal) * (1 + input.MarkupRate)))
	diff := ledgerTotal - expected
	status := "pass"
	if diff < 0 {
		status = "fail"
	}
	return Report{
		Status:              status,
		LedgerAmountCents:   ledgerTotal,
		ExpectedAmountCents: expected,
		DifferenceCents:     diff,
	}
}
