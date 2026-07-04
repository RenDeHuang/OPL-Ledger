package reconciliation

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type LedgerRow struct {
	WorkspaceID  string
	ResourceType string
	AmountCents  int64
	Currency     string
}

type TencentRow struct {
	WorkspaceID      string
	ResourceType     string
	AmountCents      int64
	Currency         string
	SourceResourceID string
}

type Input struct {
	MarkupRate  float64
	LedgerRows  []LedgerRow
	TencentRows []TencentRow
}

type Report struct {
	Status              string       `json:"status"`
	FailureReason       string       `json:"failureReason,omitempty"`
	Currency            string       `json:"currency,omitempty"`
	LedgerAmountCents   int64        `json:"ledgerAmountCents"`
	ExpectedAmountCents int64        `json:"expectedAmountCents"`
	DifferenceCents     int64        `json:"differenceCents"`
	Lines               []LineReport `json:"lines"`
}

type RawTencentBillRow map[string]any

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
	currency, err := singleCurrency(input.LedgerRows, input.TencentRows)
	if err != nil {
		return Report{Status: "fail", FailureReason: err.Error()}
	}
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

	report := Report{Status: "pass", Currency: currency}
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

func NormalizeTencentBillRows(rows []RawTencentBillRow) ([]TencentRow, error) {
	normalized := make([]TencentRow, 0, len(rows))
	for _, row := range rows {
		resourceType := tencentResourceType(row)
		if resourceType == "" {
			continue
		}
		workspaceID := workspaceIDFrom(row)
		sourceResourceID := stringValue(firstPresent(row, "sourceResourceId", "ResourceId", "resourceId", "InstanceId", "DiskId"))
		if workspaceID == "" {
			if sourceResourceID == "" {
				sourceResourceID = "unknown_resource"
			}
			return nil, fmt.Errorf("tencent_bill_workspace_id_missing:%s", sourceResourceID)
		}
		normalized = append(normalized, TencentRow{
			WorkspaceID:      workspaceID,
			ResourceType:     resourceType,
			AmountCents:      amountCentsFrom(row),
			Currency:         currencyOrDefault(stringValue(firstPresent(row, "currency", "Currency"))),
			SourceResourceID: sourceResourceID,
		})
	}
	return normalized, nil
}

func singleCurrency(ledgerRows []LedgerRow, tencentRows []TencentRow) (string, error) {
	currencies := map[string]struct{}{}
	for _, row := range ledgerRows {
		currencies[currencyOrDefault(row.Currency)] = struct{}{}
	}
	for _, row := range tencentRows {
		currencies[currencyOrDefault(row.Currency)] = struct{}{}
	}
	if len(currencies) > 1 {
		return "", errors.New("mixed_currency_not_supported")
	}
	for currency := range currencies {
		return currency, nil
	}
	return "CNY", nil
}

func currencyOrDefault(currency string) string {
	if currency == "" {
		return "CNY"
	}
	return currency
}

func workspaceIDFrom(row RawTencentBillRow) string {
	if direct := stringValue(firstPresent(row, "workspaceId", "WorkspaceId", "workspace_id", "WorkspaceID")); direct != "" {
		return direct
	}
	tags := tagsFrom(row)
	for _, key := range []string{"workspace_id", "workspaceId", "WorkspaceId", "WorkspaceID"} {
		if value := tags[key]; value != "" {
			return value
		}
	}
	return ""
}

func tagsFrom(row RawTencentBillRow) map[string]string {
	raw := firstPresent(row, "Tag", "Tags", "tag", "tags")
	switch tags := raw.(type) {
	case nil:
		return map[string]string{}
	case map[string]string:
		return tags
	case map[string]any:
		out := make(map[string]string, len(tags))
		for key, value := range tags {
			out[key] = stringValue(value)
		}
		return out
	default:
		return parseTagString(stringValue(raw))
	}
}

func parseTagString(tags string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(tags, func(r rune) bool { return r == ';' || r == ',' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		separator := "="
		if strings.Contains(part, ":") {
			separator = ":"
		}
		key, value, ok := strings.Cut(part, separator)
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}

func tencentResourceType(row RawTencentBillRow) string {
	product := strings.ToLower(stringValue(firstPresent(row, "resourceType", "ProductName", "productName", "InstanceType", "BusinessCode", "businessCode")))
	if product == "server" || strings.Contains(product, "compute") || strings.Contains(product, "kubernetes") || strings.Contains(product, "container") || strings.Contains(product, "tke") {
		return "compute"
	}
	if product == "storage" || strings.Contains(product, "block storage") || strings.Contains(product, "cbs") || strings.Contains(product, "disk") {
		return "storage"
	}
	return ""
}

func amountCentsFrom(row RawTencentBillRow) int64 {
	value := floatValue(firstPresent(row, "amount", "Amount", "RealTotalCost", "realTotalCost", "Cost", "cost", "CashPayAmount", "cashPayAmount"))
	return int64(math.Round(value * 100))
}

func firstPresent(row RawTencentBillRow, keys ...string) any {
	for _, key := range keys {
		value, ok := row[key]
		if !ok || value == nil {
			continue
		}
		if text, ok := value.(string); ok && text == "" {
			continue
		}
		return value
	}
	return nil
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case json.Number:
		result, _ := typed.Float64()
		return result
	case string:
		result, _ := strconv.ParseFloat(typed, 64)
		return result
	default:
		return 0
	}
}
