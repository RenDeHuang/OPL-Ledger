package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/usage"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

type MemoryStore struct {
	mu                    sync.Mutex
	entries               []Entry
	wallets               map[string]wallet.Wallet
	walletTransactions    []wallet.Transaction
	manualTopUps          []ManualTopUp
	requestUsageLogs      []RequestUsageLog
	auditEvents           []AuditEvent
	taskReceipts          []TaskReceipt
	reconciliationReports []ReconciliationReport
	bySourceEvent         map[string]Entry
	byRequestFingerprint  map[string]Entry
	topUpsBySourceEvent   map[string]ManualTopUp
	transactionsBySource  map[string]wallet.Transaction
	requestUsageBySource  map[string]RequestUsageResult
	requestUsageByRequest map[string]RequestUsageResult
	auditBySourceEvent    map[string]AuditEvent
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bySourceEvent:         map[string]Entry{},
		byRequestFingerprint:  map[string]Entry{},
		wallets:               map[string]wallet.Wallet{},
		topUpsBySourceEvent:   map[string]ManualTopUp{},
		transactionsBySource:  map[string]wallet.Transaction{},
		requestUsageBySource:  map[string]RequestUsageResult{},
		requestUsageByRequest: map[string]RequestUsageResult{},
		auditBySourceEvent:    map[string]AuditEvent{},
	}
}

func (s *MemoryStore) AppendEntry(_ context.Context, input AppendEntryInput) (AppendEntryResult, error) {
	if input.EventType == "" {
		return AppendEntryResult{}, errors.New("eventType is required")
	}
	if input.SourceEventID == "" && input.RequestFingerprint == "" {
		return AppendEntryResult{}, errors.New("sourceEventId or requestFingerprint is required")
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sourceEntry Entry
	var sourceFound bool
	if input.SourceEventID != "" {
		sourceEntry, sourceFound = s.bySourceEvent[input.SourceEventID]
	}
	var fingerprintEntry Entry
	var fingerprintFound bool
	if input.RequestFingerprint != "" {
		fingerprintEntry, fingerprintFound = s.byRequestFingerprint[input.RequestFingerprint]
	}
	if sourceFound && fingerprintFound && sourceEntry.ID != fingerprintEntry.ID {
		return AppendEntryResult{}, ErrIdempotencyConflict
	}
	if sourceFound {
		entry := sourceEntry
		if !sameReplayPayload(entry, input) {
			return AppendEntryResult{}, ErrIdempotencyConflict
		}
		if input.RequestFingerprint != "" {
			if entry.RequestFingerprint != "" && entry.RequestFingerprint != input.RequestFingerprint {
				return AppendEntryResult{}, ErrIdempotencyConflict
			}
			if !fingerprintFound {
				entry = s.bindRequestFingerprint(entry, input.RequestFingerprint)
			}
		}
		return AppendEntryResult{Entry: entry, Created: false}, nil
	}
	if fingerprintFound {
		entry := fingerprintEntry
		if !sameReplayPayload(entry, input) {
			return AppendEntryResult{}, ErrIdempotencyConflict
		}
		if input.SourceEventID != "" {
			if entry.SourceEventID != "" && entry.SourceEventID != input.SourceEventID {
				return AppendEntryResult{}, ErrIdempotencyConflict
			}
			if !sourceFound {
				entry = s.bindSourceEvent(entry, input.SourceEventID)
			}
		}
		return AppendEntryResult{Entry: entry, Created: false}, nil
	}
	entry := Entry{
		ID:                 randomID(),
		EventType:          input.EventType,
		AccountID:          input.AccountID,
		UserID:             input.UserID,
		WorkspaceID:        input.WorkspaceID,
		ComputeID:          input.ComputeID,
		StorageID:          input.StorageID,
		AttachmentID:       input.AttachmentID,
		SourceEventID:      input.SourceEventID,
		RequestFingerprint: input.RequestFingerprint,
		AmountCents:        input.AmountCents,
		Currency:           input.Currency,
		CreatedAt:          time.Now().UTC(),
	}
	s.entries = append(s.entries, entry)
	if entry.SourceEventID != "" {
		s.bySourceEvent[entry.SourceEventID] = entry
	}
	if entry.RequestFingerprint != "" {
		s.byRequestFingerprint[entry.RequestFingerprint] = entry
	}
	return AppendEntryResult{Entry: entry, Created: true}, nil
}

func (s *MemoryStore) ManualTopUp(_ context.Context, input ManualTopUpInput) (ManualTopUpResult, error) {
	if input.AccountID == "" {
		return ManualTopUpResult{}, errors.New("account_required")
	}
	if input.AmountCents <= 0 {
		return ManualTopUpResult{}, errors.New("positive_credit_required")
	}
	sourceEventID := input.Reason
	if sourceEventID == "" {
		sourceEventID = "owner_credit"
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.bySourceEvent[sourceEventID]; ok {
		replay := AppendEntryInput{
			EventType:     "credit",
			AccountID:     input.AccountID,
			UserID:        input.UserID,
			WorkspaceID:   "account",
			SourceEventID: sourceEventID,
			AmountCents:   input.AmountCents,
			Currency:      "CNY",
		}
		if !sameReplayPayload(entry, replay) {
			return ManualTopUpResult{}, ErrIdempotencyConflict
		}
		topup, ok := s.topUpsBySourceEvent[sourceEventID]
		if !ok {
			return ManualTopUpResult{}, errors.New("manual_topup_record_missing")
		}
		transaction, ok := s.transactionsBySource[sourceEventID]
		if !ok {
			return ManualTopUpResult{}, errors.New("wallet_transaction_record_missing")
		}
		audit, ok := s.auditBySourceEvent[topup.ID]
		if !ok {
			return ManualTopUpResult{}, errors.New("audit_event_record_missing")
		}
		w := s.wallets[input.AccountID]
		return ManualTopUpResult{
			Wallet:      w.Snapshot(),
			Entry:       entry,
			Transaction: transaction,
			TopUp:       topup,
			AuditEvent:  audit,
			Created:     false,
		}, nil
	}

	w := s.wallets[input.AccountID]
	if w.AccountID == "" {
		w.AccountID = input.AccountID
		w.UserID = input.UserID
		if w.UserID == "" {
			w.UserID = "usr-" + input.AccountID
		}
	}
	before := w.Snapshot()
	w.Credit(input.AmountCents)
	after := w.Snapshot()

	entry := Entry{
		ID:            randomID(),
		EventType:     "credit",
		AccountID:     input.AccountID,
		UserID:        w.UserID,
		WorkspaceID:   "account",
		SourceEventID: sourceEventID,
		AmountCents:   input.AmountCents,
		Currency:      "CNY",
		CreatedAt:     time.Now().UTC(),
	}
	transaction := wallet.NewTransaction(wallet.TransactionInput{
		UserID:              w.UserID,
		AccountID:           input.AccountID,
		WorkspaceID:         "account",
		Type:                wallet.TransactionCredit,
		AmountCents:         input.AmountCents,
		Currency:            "CNY",
		SourceEventID:       sourceEventID,
		LedgerEntryID:       entry.ID,
		BalanceBeforeCents:  before.BalanceCents,
		BalanceAfterCents:   after.BalanceCents,
		FrozenBeforeCents:   before.FrozenCents,
		FrozenAfterCents:    after.FrozenCents,
		AvailableAfterCents: after.AvailableCents,
		Metadata: map[string]any{
			"operatorUserId":    input.OperatorUserID,
			"operatorAccountId": input.OperatorAccountID,
		},
	})
	audit := AuditEvent{
		ID:            randomScopedID("aud"),
		AccountID:     input.AccountID,
		ActorID:       input.OperatorUserID,
		Action:        "account.credit_granted",
		TargetKind:    "manual_topup",
		SourceEventID: "",
		Payload: map[string]any{
			"sourceEventId": sourceEventID,
			"amountCents":   input.AmountCents,
		},
		CreatedAt: time.Now().UTC(),
	}
	topup := ManualTopUp{
		ID:                  randomScopedID("topup"),
		OperatorUserID:      input.OperatorUserID,
		OperatorAccountID:   input.OperatorAccountID,
		TargetUserID:        w.UserID,
		TargetAccountID:     input.AccountID,
		AmountCents:         input.AmountCents,
		Currency:            "CNY",
		Reason:              sourceEventID,
		Status:              "completed",
		BalanceBeforeCents:  before.BalanceCents,
		BalanceAfterCents:   after.BalanceCents,
		LedgerEntryID:       entry.ID,
		WalletTransactionID: transaction.ID,
		AuditEventID:        audit.ID,
		CreatedAt:           time.Now().UTC(),
	}
	audit.TargetID = topup.ID
	audit.SourceEventID = topup.ID

	s.wallets[input.AccountID] = w
	s.entries = append(s.entries, entry)
	s.bySourceEvent[sourceEventID] = entry
	s.walletTransactions = append(s.walletTransactions, transaction)
	s.transactionsBySource[sourceEventID] = transaction
	s.manualTopUps = append(s.manualTopUps, topup)
	s.topUpsBySourceEvent[sourceEventID] = topup
	s.auditEvents = append(s.auditEvents, audit)
	s.auditBySourceEvent[audit.SourceEventID] = audit

	return ManualTopUpResult{
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
		TopUp:       topup,
		AuditEvent:  audit,
		Created:     true,
	}, nil
}

func (s *MemoryStore) RecordRequestUsage(_ context.Context, input RequestUsageInput) (RequestUsageResult, error) {
	if input.WorkspaceID == "" {
		return RequestUsageResult{}, errors.New("workspace_required")
	}
	if input.RequestID == "" {
		return RequestUsageResult{}, errors.New("request_required")
	}
	if input.AmountCents < 0 {
		return RequestUsageResult{}, errors.New("non_negative_amount_required")
	}
	nextQuota, err := usage.IncrementOptionalRequestQuota(input.RequestQuota, 1, time.Now().UTC())
	if err != nil {
		return RequestUsageResult{}, err
	}
	sourceEventID := input.SourceEventID
	if sourceEventID == "" {
		sourceEventID = "gateway_request:" + input.RequestID
	}
	requestKey := input.WorkspaceID + "\x00" + input.RequestID
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.requestUsageBySource[sourceEventID]; ok {
		if existing.Log.RequestFingerprint != input.RequestFingerprint {
			return RequestUsageResult{}, ErrIdempotencyConflict
		}
		existing.Created = false
		return existing, nil
	}
	if existing, ok := s.requestUsageByRequest[requestKey]; ok {
		if existing.Log.RequestFingerprint != input.RequestFingerprint {
			return RequestUsageResult{}, ErrIdempotencyConflict
		}
		existing.Created = false
		return existing, nil
	}

	accountID := input.AccountID
	if accountID == "" {
		accountID = "acct-" + input.WorkspaceID
	}
	w := s.wallets[accountID]
	if w.AccountID == "" {
		w.AccountID = accountID
		w.UserID = input.UserID
		if w.UserID == "" {
			w.UserID = "usr-" + accountID
		}
	}
	before := w.Snapshot()
	charge := w.Charge("", input.AmountCents)
	after := w.Snapshot()
	createdAt := time.Now().UTC()

	var entry Entry
	var transaction wallet.Transaction
	if charge.ChargedCents > 0 {
		entry = Entry{
			ID:                 randomID(),
			EventType:          "request_debit",
			AccountID:          accountID,
			UserID:             w.UserID,
			WorkspaceID:        input.WorkspaceID,
			SourceEventID:      sourceEventID,
			RequestFingerprint: input.RequestFingerprint,
			AmountCents:        -charge.ChargedCents,
			Currency:           "CNY",
			CreatedAt:          createdAt,
		}
		transaction = wallet.NewTransaction(wallet.TransactionInput{
			UserID:              w.UserID,
			AccountID:           accountID,
			WorkspaceID:         input.WorkspaceID,
			Type:                wallet.TransactionDebit,
			AmountCents:         -charge.ChargedCents,
			Currency:            "CNY",
			SourceEventID:       sourceEventID,
			LedgerEntryID:       entry.ID,
			FundingSource:       "available_balance",
			BalanceBeforeCents:  before.BalanceCents,
			BalanceAfterCents:   after.BalanceCents,
			FrozenBeforeCents:   before.FrozenCents,
			FrozenAfterCents:    after.FrozenCents,
			AvailableAfterCents: after.AvailableCents,
			Metadata: map[string]any{
				"requestId":          input.RequestID,
				"provider":           input.Provider,
				"model":              input.Model,
				"requestFingerprint": input.RequestFingerprint,
			},
			CreatedAt: createdAt,
		})
	}
	log := RequestUsageLog{
		ID:                   randomScopedID("usage"),
		UserID:               w.UserID,
		AccountID:            accountID,
		WorkspaceID:          input.WorkspaceID,
		RequestID:            input.RequestID,
		Provider:             input.Provider,
		Model:                input.Model,
		InputTokens:          input.InputTokens,
		OutputTokens:         input.OutputTokens,
		AmountCents:          charge.ChargedCents,
		RequestedAmountCents: input.AmountCents,
		UnpaidCents:          input.AmountCents - charge.ChargedCents,
		Currency:             "CNY",
		SourceEventID:        sourceEventID,
		RequestFingerprint:   input.RequestFingerprint,
		Quota:                nextQuota,
		CreatedAt:            createdAt,
	}
	if entry.ID != "" {
		log.LedgerEntryID = entry.ID
		transaction.UsageLogID = log.ID
	}
	audit := AuditEvent{
		ID:            randomScopedID("aud"),
		AccountID:     accountID,
		WorkspaceID:   input.WorkspaceID,
		ActorID:       w.UserID,
		Action:        "billing.request_usage_recorded",
		TargetKind:    "request_usage",
		TargetID:      log.ID,
		SourceEventID: sourceEventID,
		Payload: map[string]any{
			"requestId":          input.RequestID,
			"requestFingerprint": input.RequestFingerprint,
			"requestedAmount":    input.AmountCents,
			"chargedAmount":      charge.ChargedCents,
		},
		CreatedAt: createdAt,
	}

	s.wallets[accountID] = w
	if entry.ID != "" {
		s.entries = append(s.entries, entry)
		s.bySourceEvent[sourceEventID] = entry
		s.byRequestFingerprint[input.RequestFingerprint] = entry
		s.walletTransactions = append(s.walletTransactions, transaction)
		s.transactionsBySource[sourceEventID] = transaction
	}
	s.requestUsageLogs = append(s.requestUsageLogs, log)
	s.auditEvents = append(s.auditEvents, audit)
	s.auditBySourceEvent[audit.SourceEventID] = audit
	result := RequestUsageResult{
		Log:         log,
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
		AuditEvent:  audit,
		Created:     true,
	}
	s.requestUsageBySource[sourceEventID] = result
	s.requestUsageByRequest[requestKey] = result
	return result, nil
}

func sameReplayPayload(entry Entry, input AppendEntryInput) bool {
	return entry.EventType == input.EventType &&
		entry.AccountID == input.AccountID &&
		entry.UserID == input.UserID &&
		entry.WorkspaceID == input.WorkspaceID &&
		entry.ComputeID == input.ComputeID &&
		entry.StorageID == input.StorageID &&
		entry.AttachmentID == input.AttachmentID &&
		entry.AmountCents == input.AmountCents &&
		entry.Currency == input.Currency
}

func (s *MemoryStore) bindRequestFingerprint(entry Entry, requestFingerprint string) Entry {
	entry.RequestFingerprint = requestFingerprint
	for i := range s.entries {
		if s.entries[i].ID == entry.ID {
			s.entries[i].RequestFingerprint = requestFingerprint
			entry = s.entries[i]
			break
		}
	}
	s.byRequestFingerprint[requestFingerprint] = entry
	if entry.SourceEventID != "" {
		s.bySourceEvent[entry.SourceEventID] = entry
	}
	return entry
}

func (s *MemoryStore) bindSourceEvent(entry Entry, sourceEventID string) Entry {
	entry.SourceEventID = sourceEventID
	for i := range s.entries {
		if s.entries[i].ID == entry.ID {
			s.entries[i].SourceEventID = sourceEventID
			entry = s.entries[i]
			break
		}
	}
	s.bySourceEvent[sourceEventID] = entry
	if entry.RequestFingerprint != "" {
		s.byRequestFingerprint[entry.RequestFingerprint] = entry
	}
	return entry
}

func (s *MemoryStore) ListEntries(_ context.Context, filter EntryFilter) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, entry := range s.entries {
		if matches(entry, filter) {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *MemoryStore) Summary(ctx context.Context, filter EntryFilter) (Summary, error) {
	entries, err := s.ListEntries(ctx, filter)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{AccountID: filter.AccountID, Currency: "CNY", EntryCount: len(entries)}
	for _, entry := range entries {
		summary.BalanceCents += entry.AmountCents
		if entry.Currency != "" {
			summary.Currency = entry.Currency
		}
	}
	return summary, nil
}

func (s *MemoryStore) AppendTaskReceipt(_ context.Context, input TaskReceiptInput) (TaskReceipt, error) {
	if input.AccountID == "" {
		return TaskReceipt{}, errors.New("task_evidence_account_required")
	}
	if input.TaskID == "" {
		return TaskReceipt{}, errors.New("task_evidence_task_required")
	}
	if len(input.Plan) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_plan_required")
	}
	if len(input.Approval) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_approval_required")
	}
	if len(input.Environment) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_environment_required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	receipt := TaskReceipt{
		ID:            randomID(),
		Type:          "task.evidence.v1",
		AccountID:     input.AccountID,
		WorkspaceID:   input.WorkspaceID,
		TaskID:        input.TaskID,
		Actor:         mapOrDefault(input.Actor, map[string]any{"type": "system", "id": "opl-ledger"}),
		Plan:          cloneMap(input.Plan),
		Approval:      cloneMap(input.Approval),
		Environment:   cloneMap(input.Environment),
		InputRefs:     cloneMapSlice(input.InputRefs),
		ExecutionRefs: cloneMapSlice(input.ExecutionRefs),
		OutputRefs:    cloneMapSlice(input.OutputRefs),
		ReviewResults: cloneMapSlice(input.ReviewResults),
		Continuation:  cloneMap(input.Continuation),
		Metadata:      cloneMap(input.Metadata),
		CreatedAt:     time.Now().UTC(),
	}
	s.taskReceipts = append(s.taskReceipts, receipt)
	return receipt, nil
}

func (s *MemoryStore) ListTaskReceipts(_ context.Context, filter TaskReceiptFilter) ([]TaskReceipt, error) {
	if filter.AccountID == "" {
		return nil, errors.New("task_evidence_account_required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []TaskReceipt
	for _, receipt := range s.taskReceipts {
		if receipt.AccountID != filter.AccountID {
			continue
		}
		if filter.WorkspaceID != "" && receipt.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.TaskID != "" && receipt.TaskID != filter.TaskID {
			continue
		}
		out = append(out, receipt)
	}
	return out, nil
}

func (s *MemoryStore) AppendReconciliationReport(_ context.Context, report ReconciliationReport) (ReconciliationReport, error) {
	if report.ID == "" {
		report.ID = randomID()
	}
	if report.Provider == "" {
		report.Provider = "manual"
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconciliationReports = append(s.reconciliationReports, report)
	return report, nil
}

func (s *MemoryStore) LatestReconciliationReport(_ context.Context) (ReconciliationReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reconciliationReports) == 0 {
		return ReconciliationReport{}, errors.New("billing_reconciliation_report_missing")
	}
	return s.reconciliationReports[len(s.reconciliationReports)-1], nil
}

func matches(entry Entry, filter EntryFilter) bool {
	if filter.AccountID != "" && entry.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && entry.UserID != filter.UserID {
		return false
	}
	if filter.WorkspaceID != "" && entry.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.ComputeID != "" && entry.ComputeID != filter.ComputeID {
		return false
	}
	if filter.StorageID != "" && entry.StorageID != filter.StorageID {
		return false
	}
	if filter.AttachmentID != "" && entry.AttachmentID != filter.AttachmentID {
		return false
	}
	if filter.SourceEventID != "" && entry.SourceEventID != filter.SourceEventID {
		return false
	}
	return true
}

func mapOrDefault(value map[string]any, fallback map[string]any) map[string]any {
	if len(value) == 0 {
		return cloneMap(fallback)
	}
	return cloneMap(value)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func cloneMapSlice(value []map[string]any) []map[string]any {
	if value == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(value))
	for _, item := range value {
		out = append(out, cloneMap(item))
	}
	return out
}

func randomID() string {
	return randomScopedID("led")
}

func randomScopedID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
