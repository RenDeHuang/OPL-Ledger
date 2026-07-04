package migration

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Report struct {
	GeneratedAt    time.Time      `json:"generatedAt"`
	Source         string         `json:"source"`
	Status         string         `json:"status"`
	RowCounts      map[string]int `json:"rowCounts"`
	Mismatches     []string       `json:"mismatches"`
	BlockedReasons []string       `json:"blockedReasons"`
}

type dryRun struct {
	inputDir  string
	outputDir string
	report    Report
}

func RunDryRun(inputDir string, outputDir string) (Report, error) {
	if inputDir == "" {
		return Report{}, errors.New("input directory is required")
	}
	if outputDir == "" {
		return Report{}, errors.New("output directory is required")
	}
	r := dryRun{
		inputDir:  inputDir,
		outputDir: outputDir,
		report: Report{
			GeneratedAt:    time.Now().UTC(),
			Source:         "local",
			Status:         "pass",
			RowCounts:      map[string]int{},
			Mismatches:     []string{},
			BlockedReasons: []string{},
		},
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return Report{}, err
	}

	users, err := readRecords(filepath.Join(inputDir, "users.json"))
	if err != nil {
		return Report{}, err
	}
	ledgerEntries, err := readRecords(filepath.Join(inputDir, "billingLedger.json"))
	if err != nil {
		return Report{}, err
	}
	walletTransactions, err := readRecords(filepath.Join(inputDir, "walletTransactions.json"))
	if err != nil {
		return Report{}, err
	}
	manualTopups, err := readRecords(filepath.Join(inputDir, "manualTopups.json"))
	if err != nil {
		return Report{}, err
	}
	auditEvents, err := readRecords(filepath.Join(inputDir, "audit.json"))
	if err != nil {
		return Report{}, err
	}

	walletPreview := r.mapWallets(users)
	ledgerPreview := r.mapLedgerEntries(ledgerEntries)
	transactionPreview := r.mapWalletTransactions(walletTransactions)
	topupPreview := r.mapManualTopups(manualTopups)
	auditPreview := r.mapAuditEvents(auditEvents)
	r.validateManualTopups(topupPreview, ledgerPreview, transactionPreview, auditPreview)

	if err := r.writePreview("wallets.preview.json", walletPreview); err != nil {
		return Report{}, err
	}
	if err := r.writePreview("ledger_entries.preview.json", ledgerPreview); err != nil {
		return Report{}, err
	}
	if err := r.writePreview("wallet_transactions.preview.json", transactionPreview); err != nil {
		return Report{}, err
	}
	if err := r.writePreview("manual_topups.preview.json", topupPreview); err != nil {
		return Report{}, err
	}
	if err := r.writePreview("audit_events.preview.json", auditPreview); err != nil {
		return Report{}, err
	}
	if len(r.report.Mismatches) > 0 || len(r.report.BlockedReasons) > 0 {
		r.report.Status = "fail"
	}
	if err := r.writeReport(); err != nil {
		return Report{}, err
	}
	return r.report, nil
}

func (r *dryRun) mapWallets(records []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		accountID := stringValue(record, "accountId", "account_id")
		userID := stringValue(record, "userId", "user_id", "id")
		balance := r.money(record, "balance_cents", "balanceCents", "balance")
		frozen := r.money(record, "frozen_cents", "frozenCents", "frozen")
		totalRecharged := r.money(record, "total_recharged_cents", "totalRechargedCents", "totalRecharged")
		holds := mapValue(record, "holds")
		out = append(out, map[string]any{
			"id":                    stableID("wallet", accountID),
			"user_id":               userID,
			"account_id":            accountID,
			"balance_cents":         balance,
			"frozen_cents":          frozen,
			"total_recharged_cents": totalRecharged,
			"holds":                 holds,
			"payload":               cloneMap(record),
		})
	}
	return out
}

func (r *dryRun) mapLedgerEntries(records []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"id":                  stringValue(record, "id"),
			"event_type":          stringValue(record, "eventType", "type"),
			"account_id":          stringValue(record, "accountId", "account_id"),
			"user_id":             stringValue(record, "userId", "user_id"),
			"workspace_id":        stringValue(record, "workspaceId", "workspace_id"),
			"compute_id":          stringValue(record, "computeId", "compute_id"),
			"storage_id":          stringValue(record, "storageId", "storage_id"),
			"attachment_id":       stringValue(record, "attachmentId", "attachment_id"),
			"source_event_id":     stringValue(record, "sourceEventId", "source_event_id"),
			"request_fingerprint": stringValue(record, "requestFingerprint", "request_fingerprint"),
			"amount_cents":        r.money(record, "amount_cents", "amountCents", "amount"),
			"currency":            defaultString(stringValue(record, "currency"), "CNY"),
			"payload":             cloneMap(record),
		})
	}
	return out
}

func (r *dryRun) mapWalletTransactions(records []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		balanceAfter := r.money(record, "balance_after_cents", "balanceAfterCents", "balanceAfter")
		frozenAfter := r.money(record, "frozen_after_cents", "frozenAfterCents", "frozenAfter")
		out = append(out, map[string]any{
			"id":                    stringValue(record, "id"),
			"account_id":            stringValue(record, "accountId", "account_id"),
			"user_id":               stringValue(record, "userId", "user_id"),
			"workspace_id":          defaultString(stringValue(record, "workspaceId", "workspace_id"), "account"),
			"transaction_type":      stringValue(record, "type", "transactionType", "transaction_type"),
			"amount_cents":          r.money(record, "amount_cents", "amountCents", "amount"),
			"currency":              defaultString(stringValue(record, "currency"), "CNY"),
			"source_event_id":       stringValue(record, "sourceEventId", "source_event_id"),
			"ledger_entry_id":       stringValue(record, "ledgerEntryId", "ledger_entry_id"),
			"usage_log_id":          stringValue(record, "usageLogId", "usage_log_id"),
			"funding_source":        stringValue(record, "fundingSource", "funding_source"),
			"balance_before_cents":  r.money(record, "balance_before_cents", "balanceBeforeCents", "balanceBefore"),
			"balance_after_cents":   balanceAfter,
			"frozen_before_cents":   r.money(record, "frozen_before_cents", "frozenBeforeCents", "frozenBefore"),
			"frozen_after_cents":    frozenAfter,
			"available_after_cents": balanceAfter - frozenAfter,
			"payload":               cloneMap(record),
		})
	}
	return out
}

func (r *dryRun) mapManualTopups(records []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		sourceEventID := stringValue(record, "sourceEventId", "source_event_id")
		reason := stringValue(record, "reason")
		if sourceEventID == "" {
			sourceEventID = reason
		}
		payload := cloneMap(record)
		payload["reason"] = reason
		out = append(out, map[string]any{
			"id":                    stringValue(record, "id"),
			"account_id":            stringValue(record, "targetAccountId", "accountId", "account_id"),
			"user_id":               stringValue(record, "targetUserId", "userId", "user_id"),
			"operator_id":           stringValue(record, "operatorUserId", "operator_id"),
			"operator_account_id":   stringValue(record, "operatorAccountId", "operator_account_id"),
			"target_user_id":        stringValue(record, "targetUserId", "userId", "user_id"),
			"target_account_id":     stringValue(record, "targetAccountId", "accountId", "account_id"),
			"source_event_id":       sourceEventID,
			"amount_cents":          r.money(record, "amount_cents", "amountCents", "amount"),
			"currency":              defaultString(stringValue(record, "currency"), "CNY"),
			"status":                defaultString(stringValue(record, "status"), "completed"),
			"balance_before_cents":  r.money(record, "balance_before_cents", "balanceBeforeCents", "balanceBefore"),
			"balance_after_cents":   r.money(record, "balance_after_cents", "balanceAfterCents", "balanceAfter"),
			"ledger_entry_id":       stringValue(record, "ledgerEntryId", "ledger_entry_id"),
			"wallet_transaction_id": stringValue(record, "walletTransactionId", "wallet_transaction_id"),
			"audit_event_id":        stringValue(record, "auditEventId", "audit_event_id"),
			"payload":               payload,
		})
	}
	return out
}

func (r *dryRun) mapAuditEvents(records []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		out = append(out, map[string]any{
			"id":              stringValue(record, "id"),
			"account_id":      stringValue(record, "accountId", "account_id"),
			"workspace_id":    stringValue(record, "workspaceId", "workspace_id"),
			"actor_id":        stringValue(record, "actorId", "actor_id"),
			"action":          stringValue(record, "action", "type"),
			"target_kind":     stringValue(record, "targetKind", "target_kind"),
			"target_id":       stringValue(record, "targetId", "target_id"),
			"source_event_id": stringValue(record, "sourceEventId", "source_event_id"),
			"payload":         cloneMap(record),
		})
	}
	return out
}

func (r *dryRun) validateManualTopups(topups []map[string]any, ledgerEntries []map[string]any, walletTransactions []map[string]any, auditEvents []map[string]any) {
	ledgerIDs := idSet(ledgerEntries)
	transactionIDs := idSet(walletTransactions)
	auditIDs := idSet(auditEvents)
	sourceSeen := map[string]bool{}
	for _, topup := range topups {
		source := fmt.Sprint(topup["source_event_id"])
		if source == "" {
			r.block("manual_topup_source_event_missing")
		}
		if sourceSeen[source] {
			r.mismatch("duplicate manual topup source_event_id: " + source)
			r.block("manual_topup_source_event_duplicate")
		}
		sourceSeen[source] = true
		if id := fmt.Sprint(topup["ledger_entry_id"]); id != "" && !ledgerIDs[id] {
			r.mismatch("manual topup references missing ledger entry: " + id)
			r.block("manual_topup_missing_ledger_entry")
		}
		if id := fmt.Sprint(topup["wallet_transaction_id"]); id != "" && !transactionIDs[id] {
			r.mismatch("manual topup references missing wallet transaction: " + id)
			r.block("manual_topup_missing_wallet_transaction")
		}
		if id := fmt.Sprint(topup["audit_event_id"]); id != "" && !auditIDs[id] {
			r.mismatch("manual topup references missing audit event: " + id)
			r.block("manual_topup_missing_audit_event")
		}
	}
}

func (r *dryRun) money(record map[string]any, keys ...string) int64 {
	key, value, ok := lookup(record, keys...)
	if !ok || value == nil {
		return 0
	}
	centsKey := strings.Contains(strings.ToLower(key), "cent")
	cents, err := moneyToCents(value, centsKey)
	if err != nil {
		r.mismatch(fmt.Sprintf("non-integer money value for %s: %v", key, value))
		r.block("non_integer_money_values")
		return 0
	}
	return cents
}

func moneyToCents(value any, alreadyCents bool) (int64, error) {
	number, err := numberValue(value)
	if err != nil {
		return 0, err
	}
	if alreadyCents {
		if math.Trunc(number) != number {
			return 0, errors.New("cents value must be integer")
		}
		return int64(number), nil
	}
	cents := number * 100
	if math.Abs(cents-math.Round(cents)) > 0.000001 {
		return 0, errors.New("money value must resolve to integer cents")
	}
	return int64(math.Round(cents)), nil
}

func numberValue(value any) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		return v.Float64()
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("unsupported money type %T", value)
	}
}

func readRecords(path string) ([]map[string]any, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []map[string]any{}, nil
		}
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	return normalizeRecords(raw), nil
}

func normalizeRecords(raw any) []map[string]any {
	switch v := raw.(type) {
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if record, ok := item.(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	case map[string]any:
		if looksLikeRecord(v) {
			return []map[string]any{v}
		}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]map[string]any, 0, len(v))
		for _, key := range keys {
			if record, ok := v[key].(map[string]any); ok {
				out = append(out, record)
			}
		}
		return out
	default:
		return []map[string]any{}
	}
}

func looksLikeRecord(value map[string]any) bool {
	for _, key := range []string{"id", "accountId", "sourceEventId", "amount", "amountCents"} {
		if _, ok := value[key]; ok {
			return true
		}
	}
	return false
}

func (r *dryRun) writePreview(name string, value []map[string]any) error {
	r.report.RowCounts[name] = len(value)
	return writeJSON(filepath.Join(r.outputDir, name), value)
}

func (r *dryRun) writeReport() error {
	return writeJSON(filepath.Join(r.outputDir, "migration-report.json"), r.report)
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(path, payload, 0o600)
}

func idSet(records []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, record := range records {
		id := stringValue(record, "id")
		if id != "" {
			out[id] = true
		}
	}
	return out
}

func stringValue(record map[string]any, keys ...string) string {
	_, value, ok := lookup(record, keys...)
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func lookup(record map[string]any, keys ...string) (string, any, bool) {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return key, value, true
		}
	}
	return "", nil, false
}

func mapValue(record map[string]any, key string) map[string]any {
	if value, ok := record[key].(map[string]any); ok {
		return cloneMap(value)
	}
	return map[string]any{}
}

func cloneMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func stableID(prefix string, value string) string {
	if value == "" {
		return prefix + "_missing"
	}
	return prefix + "_" + value
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (r *dryRun) mismatch(message string) {
	r.report.Mismatches = append(r.report.Mismatches, message)
}

func (r *dryRun) block(reason string) {
	for _, existing := range r.report.BlockedReasons {
		if existing == reason {
			return
		}
	}
	r.report.BlockedReasons = append(r.report.BlockedReasons, reason)
}
