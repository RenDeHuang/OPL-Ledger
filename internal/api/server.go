package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/auth"
	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	"github.com/RenDeHuang/OPL-Ledger/internal/ownership"
	"github.com/RenDeHuang/OPL-Ledger/internal/reconciliation"
	"github.com/RenDeHuang/OPL-Ledger/internal/version"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

type Server struct {
	store              ledger.Store
	workspaceOwnership ownership.WorkspaceResolver
	auth               auth.Config
	mux                *http.ServeMux
}

type Options struct {
	WorkspaceOwnership ownership.WorkspaceResolver
	Auth               auth.Config
}

func NewServer(store ledger.Store) http.Handler {
	return NewServerWithOptions(store, Options{})
}

func NewServerWithOwnership(store ledger.Store, workspaceOwnership ownership.WorkspaceResolver) http.Handler {
	return NewServerWithOptions(store, Options{WorkspaceOwnership: workspaceOwnership})
}

func NewServerWithOptions(store ledger.Store, options Options) http.Handler {
	s := &Server{
		store:              store,
		workspaceOwnership: options.WorkspaceOwnership,
		auth:               options.Auth,
		mux:                http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.healthz)
	s.mux.HandleFunc("POST /api/v1/ledger/entries", s.appendEntry)
	s.mux.HandleFunc("GET /api/v1/ledger/entries", s.listEntries)
	s.mux.HandleFunc("GET /api/v1/ledger/summary", s.summary)
	s.mux.HandleFunc("GET /api/v1/billing/wallet-transactions", s.listWalletTransactions)
	s.mux.HandleFunc("POST /api/v1/billing/topups", s.manualTopUp)
	s.mux.HandleFunc("POST /api/v1/billing/holds", s.createHold)
	s.mux.HandleFunc("POST /api/v1/billing/holds/release", s.releaseHolds)
	s.mux.HandleFunc("POST /api/v1/billing/settlements", s.settleWorkspaceUsage)
	s.mux.HandleFunc("POST /api/v1/billing/resource-usage", s.recordResourceUsage)
	s.mux.HandleFunc("POST /api/v1/billing/request-usage", s.recordRequestUsage)
	s.mux.HandleFunc("PUT /api/v1/billing/request-quotas", s.upsertRequestQuota)
	s.mux.HandleFunc("GET /api/v1/billing/request-quotas", s.listRequestQuotas)
	s.mux.HandleFunc("POST /api/v1/billing/reconciliation", s.recordReconciliation)
	s.mux.HandleFunc("GET /api/v1/billing/reconciliation", s.listReconciliation)
	s.mux.HandleFunc("GET /api/v1/billing/reconciliation/latest", s.latestReconciliation)
	s.mux.HandleFunc("GET /api/v1/billing/reconciliation/guard", s.reconciliationGuard)
	s.mux.HandleFunc("POST /api/v1/audit/events", s.recordAuditEvent)
	s.mux.HandleFunc("GET /api/v1/audit/events", s.listAuditEvents)
	s.mux.HandleFunc("POST /api/v1/ledger/evidence-records", s.recordEvidenceRecord)
	s.mux.HandleFunc("GET /api/v1/ledger/evidence-records", s.listEvidenceRecords)
	s.mux.HandleFunc("POST /api/v1/ledger/kubernetes-evidence-snapshots", s.recordKubernetesEvidenceSnapshot)
	s.mux.HandleFunc("GET /api/v1/ledger/kubernetes-evidence-snapshots", s.listKubernetesEvidenceSnapshots)
	s.mux.HandleFunc("POST /api/v1/ledger/task-receipts", s.recordTaskReceipt)
	s.mux.HandleFunc("GET /api/v1/ledger/task-receipts", s.listTaskReceipts)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": version.ServiceName, "apiVersion": version.APIVersion})
}

func (s *Server) appendEntry(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.AppendEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.store.AppendEntry(r.Context(), input)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ledger.ErrIdempotencyConflict) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListEntries(r.Context(), filterFromQuery(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Summary(r.Context(), filterFromQuery(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) listWalletTransactions(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	transactions, err := s.store.ListWalletTransactions(r.Context(), walletTransactionFilterFromQuery(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, transactions)
}

func (s *Server) manualTopUp(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.ManualTopUpInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.store.ManualTopUp(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) createHold(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.HoldInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.store.CreateHold(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) releaseHolds(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.ReleaseHoldInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.store.ReleaseHolds(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) settleWorkspaceUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.SettlementInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.store.SettleWorkspaceUsage(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) recordResourceUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.ResourceUsageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.store.RecordResourceUsage(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) recordRequestUsage(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.RequestUsageInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if input.WorkspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "workspace_required"})
		return
	}
	if input.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request_required"})
		return
	}
	if input.AmountCents < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "non_negative_amount_required"})
		return
	}
	sourceEventID := input.SourceEventID
	if sourceEventID == "" {
		sourceEventID = "gateway_request:" + input.RequestID
	}
	fingerprint := input.RequestFingerprint
	if fingerprint == "" {
		fingerprint = requestUsageFingerprint(input.Provider, input.Model, input.InputTokens, input.OutputTokens, input.AmountCents, sourceEventID)
	}
	input.SourceEventID = sourceEventID
	input.RequestFingerprint = fingerprint
	result, err := s.store.RecordRequestUsage(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (s *Server) upsertRequestQuota(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.RequestQuotaInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	record, err := s.store.UpsertRequestQuota(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) listRequestQuotas(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	q := r.URL.Query()
	records, err := s.store.ListRequestQuotas(r.Context(), ledger.RequestQuotaFilter{
		AccountID:   q.Get("accountId"),
		UserID:      q.Get("userId"),
		WorkspaceID: q.Get("workspaceId"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) recordTaskReceipt(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.TaskReceiptInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if err := ownership.ValidateWorkspaceAccount(r.Context(), s.workspaceOwnership, input.AccountID, input.WorkspaceID); err != nil {
		writeOwnershipError(w, err)
		return
	}
	receipt, err := s.store.AppendTaskReceipt(r.Context(), input)
	if err != nil {
		writeAppendError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}

func (s *Server) recordAuditEvent(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.AuditEventInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	event, err := s.store.AppendAuditEvent(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	q := r.URL.Query()
	events, err := s.store.ListAuditEvents(r.Context(), ledger.AuditEventFilter{
		AccountID:     q.Get("accountId"),
		WorkspaceID:   q.Get("workspaceId"),
		Action:        q.Get("action"),
		SourceEventID: q.Get("sourceEventId"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, events)
}

func (s *Server) recordEvidenceRecord(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input ledger.EvidenceRecordInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	record, err := s.store.AppendEvidenceRecord(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) listEvidenceRecords(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	q := r.URL.Query()
	records, err := s.store.ListEvidenceRecords(r.Context(), ledger.EvidenceRecordFilter{
		AccountID:     q.Get("accountId"),
		WorkspaceID:   q.Get("workspaceId"),
		Type:          q.Get("type"),
		SourceEventID: q.Get("sourceEventId"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, records)
}

func (s *Server) recordKubernetesEvidenceSnapshot(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var snapshot ledger.KubernetesEvidenceSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	recorded, err := s.store.AppendKubernetesEvidenceSnapshot(r.Context(), snapshot)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, recorded)
}

func (s *Server) listKubernetesEvidenceSnapshots(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	q := r.URL.Query()
	snapshots, err := s.store.ListKubernetesEvidenceSnapshots(r.Context(), ledger.KubernetesEvidenceSnapshotFilter{
		ClusterID:   q.Get("clusterId"),
		Namespace:   q.Get("namespace"),
		ObjectKind:  q.Get("objectKind"),
		ObjectName:  q.Get("objectName"),
		WorkspaceID: q.Get("workspaceId"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) listTaskReceipts(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	q := r.URL.Query()
	receipts, err := s.store.ListTaskReceipts(r.Context(), ledger.TaskReceiptFilter{
		AccountID:   q.Get("accountId"),
		WorkspaceID: q.Get("workspaceId"),
		TaskID:      q.Get("taskId"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, receipts)
}

func (s *Server) recordReconciliation(w http.ResponseWriter, r *http.Request) {
	if !s.requireService(w, r) {
		return
	}
	var input struct {
		Provider    string                      `json:"provider"`
		MarkupRate  float64                     `json:"markupRate"`
		LedgerRows  []reconciliation.LedgerRow  `json:"ledgerRows"`
		TencentRows []reconciliation.TencentRow `json:"tencentRows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	if input.Provider == "" {
		input.Provider = "manual"
	}
	report := reconciliation.ReconcileTencentBills(reconciliation.Input{
		MarkupRate:  input.MarkupRate,
		LedgerRows:  input.LedgerRows,
		TencentRows: input.TencentRows,
	})
	payload := map[string]any{}
	payloadBytes, _ := json.Marshal(report)
	_ = json.Unmarshal(payloadBytes, &payload)
	stored, err := s.store.AppendReconciliationReport(r.Context(), ledger.ReconciliationReport{
		Provider:            input.Provider,
		Status:              report.Status,
		LedgerAmountCents:   report.LedgerAmountCents,
		ExpectedAmountCents: report.ExpectedAmountCents,
		DifferenceCents:     report.DifferenceCents,
		Payload:             payload,
		CreatedAt:           time.Now().UTC(),
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

func (s *Server) listReconciliation(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	q := r.URL.Query()
	reports, err := s.store.ListReconciliationReports(r.Context(), ledger.ReconciliationReportFilter{
		Provider: q.Get("provider"),
		Status:   q.Get("status"),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, reports)
}

func (s *Server) latestReconciliation(w http.ResponseWriter, r *http.Request) {
	report, err := s.store.LatestReconciliationReport(r.Context())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) reconciliationGuard(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	maxAgeHours := 30.0
	if raw := r.URL.Query().Get("maxAgeHours"); raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil || parsed <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_max_age_hours"})
			return
		}
		maxAgeHours = parsed
	}
	now := time.Now().UTC()
	report, err := s.store.LatestReconciliationReport(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, ledger.ReconciliationGuard{
			Status:             "blocked",
			BlockNewWorkspaces: true,
			Reason:             "billing_reconciliation_report_missing",
			CheckedAt:          now,
		})
		return
	}
	guard := ledger.ReconciliationGuard{
		Status:      "ok",
		Reason:      "billing_reconciliation_ok",
		CheckedAt:   now,
		GeneratedAt: report.CreatedAt,
		AgeHours:    now.Sub(report.CreatedAt).Hours(),
	}
	if guard.AgeHours > maxAgeHours {
		guard.Status = "blocked"
		guard.BlockNewWorkspaces = true
		guard.Reason = "billing_reconciliation_report_stale"
		writeJSON(w, http.StatusOK, guard)
		return
	}
	if report.Status != "pass" {
		guard.Status = "blocked"
		guard.BlockNewWorkspaces = true
		guard.Reason = "tencent_bill_reconciliation_failed"
	}
	writeJSON(w, http.StatusOK, guard)
}

func filterFromQuery(r *http.Request) ledger.EntryFilter {
	q := r.URL.Query()
	return ledger.EntryFilter{
		AccountID:     q.Get("accountId"),
		UserID:        q.Get("userId"),
		WorkspaceID:   q.Get("workspaceId"),
		ComputeID:     q.Get("computeId"),
		StorageID:     q.Get("storageId"),
		AttachmentID:  q.Get("attachmentId"),
		SourceEventID: q.Get("sourceEventId"),
	}
}

func walletTransactionFilterFromQuery(r *http.Request) ledger.WalletTransactionFilter {
	q := r.URL.Query()
	return ledger.WalletTransactionFilter{
		AccountID:     q.Get("accountId"),
		UserID:        q.Get("userId"),
		WorkspaceID:   q.Get("workspaceId"),
		Type:          walletTransactionType(q.Get("type")),
		SourceEventID: q.Get("sourceEventId"),
		LedgerEntryID: q.Get("ledgerEntryId"),
		UsageLogID:    q.Get("usageLogId"),
		FundingSource: q.Get("fundingSource"),
	}
}

func walletTransactionType(value string) wallet.TransactionType {
	return wallet.TransactionType(value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) requireService(w http.ResponseWriter, r *http.Request) bool {
	return s.requireRole(w, r, auth.RoleService)
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	return s.requireRole(w, r, auth.RoleAdmin)
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, role auth.Role) bool {
	err := auth.Authorize(r, s.auth, role)
	if err == nil {
		return true
	}
	writeAuthError(w, err)
	return false
}

func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusForbidden
	if errors.Is(err, auth.ErrMissingToken) {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeAppendError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, ledger.ErrIdempotencyConflict) {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeOwnershipError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, ownership.ErrWorkspaceNotFound) {
		status = http.StatusNotFound
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func requestUsageFingerprint(provider string, model string, inputTokens int64, outputTokens int64, amountCents int64, sourceEventID string) string {
	raw := fmt.Sprintf("%s:%s:%d:%d:%d:%s", provider, model, inputTokens, outputTokens, amountCents, sourceEventID)
	sum := sha256.Sum256([]byte(raw))
	return "fp-" + hex.EncodeToString(sum[:8])
}
