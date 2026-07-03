package reconciliation

import (
	"math"
	"sort"
)

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
	Status              string       `json:"status"`
	LedgerAmountCents   int64        `json:"ledgerAmountCents"`
	ExpectedAmountCents int64        `json:"expectedAmountCents"`
	DifferenceCents     int64        `json:"differenceCents"`
	Lines               []LineReport `json:"lines"`
}

type LineReport struct {
	WorkspaceID         string `json:"workspaceId"`
	ResourceType        string `json:"resourceType"`
	Status              string `json:"status"`
	LedgerAmountCents   int64  `json:"ledgerAmountCents"`
	ExpectedAmountCents int64  `json:"expectedAmountCents"`
	DifferenceCents     int64  `json:"differenceCents"`
	InvalidLedgerRows   int    `json:"invalidLedgerRows"`
}

type reconcileKey struct {
	workspaceID  string
	resourceType string
}

func ReconcileTencentBills(input Input) Report {
	ledgerByKey := make(map[reconcileKey]int64)
	invalidLedgerRowsByKey := make(map[reconcileKey]int)
	tencentByKey := make(map[reconcileKey]int64)
	keys := make(map[reconcileKey]struct{})

	for _, row := range input.LedgerRows {
		key := reconcileKey{workspaceID: row.WorkspaceID, resourceType: row.ResourceType}
		if row.AmountCents < 0 {
			ledgerByKey[key] += -row.AmountCents
		} else {
			invalidLedgerRowsByKey[key]++
		}
		keys[key] = struct{}{}
	}
	for _, row := range input.TencentRows {
		key := reconcileKey{workspaceID: row.WorkspaceID, resourceType: row.ResourceType}
		tencentByKey[key] += row.AmountCents
		keys[key] = struct{}{}
	}

	orderedKeys := make([]reconcileKey, 0, len(keys))
	for key := range keys {
		orderedKeys = append(orderedKeys, key)
	}
	sort.Slice(orderedKeys, func(i, j int) bool {
		if orderedKeys[i].workspaceID == orderedKeys[j].workspaceID {
			return orderedKeys[i].resourceType < orderedKeys[j].resourceType
		}
		return orderedKeys[i].workspaceID < orderedKeys[j].workspaceID
	})

	report := Report{Status: "pass"}
	for _, key := range orderedKeys {
		ledger := ledgerByKey[key]
		invalidLedgerRows := invalidLedgerRowsByKey[key]
		expected := int64(math.Round(float64(tencentByKey[key]) * (1 + input.MarkupRate)))
		diff := ledger - expected
		status := "pass"
		if diff < -1 || invalidLedgerRows > 0 {
			status = "fail"
			report.Status = "fail"
		}
		report.LedgerAmountCents += ledger
		report.ExpectedAmountCents += expected
		report.DifferenceCents += diff
		report.Lines = append(report.Lines, LineReport{
			WorkspaceID:         key.workspaceID,
			ResourceType:        key.resourceType,
			Status:              status,
			LedgerAmountCents:   ledger,
			ExpectedAmountCents: expected,
			DifferenceCents:     diff,
			InvalidLedgerRows:   invalidLedgerRows,
		})
	}
	return report
}
