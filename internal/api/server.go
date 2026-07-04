package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	"github.com/RenDeHuang/OPL-Ledger/internal/reconciliation"
	"github.com/RenDeHuang/OPL-Ledger/internal/version"
)

type Server struct {
	store ledger.Store
	mux   *http.ServeMux
}

func NewServer(store ledger.Store) http.Handler {
	s := &Server{store: store, mux: http.NewServeMux()}
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
	s.mux.HandleFunc("POST /api/v1/billing/topups", s.manualTopUp)
	s.mux.HandleFunc("POST /api/v1/billing/request-usage", s.recordRequestUsage)
	s.mux.HandleFunc("POST /api/v1/billing/reconciliation", s.recordReconciliation)
	s.mux.HandleFunc("GET /api/v1/billing/reconciliation/latest", s.latestReconciliation)
	s.mux.HandleFunc("POST /api/v1/ledger/task-receipts", s.recordTaskReceipt)
	s.mux.HandleFunc("GET /api/v1/ledger/task-receipts", s.listTaskReceipts)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": version.ServiceName, "apiVersion": version.APIVersion})
}

func (s *Server) appendEntry(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, status, result.Entry)
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

func (s *Server) manualTopUp(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) recordRequestUsage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountID          string `json:"accountId"`
		UserID             string `json:"userId"`
		WorkspaceID        string `json:"workspaceId"`
		RequestID          string `json:"requestId"`
		Provider           string `json:"provider"`
		Model              string `json:"model"`
		InputTokens        int64  `json:"inputTokens"`
		OutputTokens       int64  `json:"outputTokens"`
		AmountCents        int64  `json:"amountCents"`
		SourceEventID      string `json:"sourceEventId"`
		RequestFingerprint string `json:"requestFingerprint"`
	}
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
	result, err := s.store.AppendEntry(r.Context(), ledger.AppendEntryInput{
		EventType:          "request_debit",
		AccountID:          input.AccountID,
		UserID:             input.UserID,
		WorkspaceID:        input.WorkspaceID,
		SourceEventID:      sourceEventID,
		RequestFingerprint: fingerprint,
		AmountCents:        -input.AmountCents,
		Currency:           "CNY",
	})
	if err != nil {
		writeAppendError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result.Entry)
}

func (s *Server) recordTaskReceipt(w http.ResponseWriter, r *http.Request) {
	var input ledger.TaskReceiptInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	receipt, err := s.store.AppendTaskReceipt(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}

func (s *Server) listTaskReceipts(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) latestReconciliation(w http.ResponseWriter, r *http.Request) {
	report, err := s.store.LatestReconciliationReport(r.Context())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAppendError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, ledger.ErrIdempotencyConflict) {
		status = http.StatusConflict
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func requestUsageFingerprint(provider string, model string, inputTokens int64, outputTokens int64, amountCents int64, sourceEventID string) string {
	raw := fmt.Sprintf("%s:%s:%d:%d:%d:%s", provider, model, inputTokens, outputTokens, amountCents, sourceEventID)
	sum := sha256.Sum256([]byte(raw))
	return "fp-" + hex.EncodeToString(sum[:8])
}
