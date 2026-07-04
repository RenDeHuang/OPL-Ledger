package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"
	"time"

	auditlog "github.com/RenDeHuang/OPL-Ledger/internal/audit"
	evidencelog "github.com/RenDeHuang/OPL-Ledger/internal/evidence"
	k8sevidence "github.com/RenDeHuang/OPL-Ledger/internal/k8s"
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
	evidenceRecords       []EvidenceRecord
	kubernetesSnapshots   []KubernetesEvidenceSnapshot
	taskReceipts          []TaskReceipt
	reconciliationReports []ReconciliationReport
	bySourceEvent         map[string]Entry
	byRequestFingerprint  map[string]Entry
	topUpsBySourceEvent   map[string]ManualTopUp
	transactionsBySource  map[string]wallet.Transaction
	releaseHoldsBySource  map[string]ReleaseHoldResult
	settlementsBySource   map[string]SettlementResult
	resourceUsageBySource map[string]ResourceUsageResult
	requestUsageBySource  map[string]RequestUsageResult
	requestUsageByRequest map[string]RequestUsageResult
	requestQuotas         map[string]RequestQuotaRecord
	auditBySourceEvent    map[string]AuditEvent
	taskReceiptBySource   map[string]TaskReceipt
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bySourceEvent:         map[string]Entry{},
		byRequestFingerprint:  map[string]Entry{},
		wallets:               map[string]wallet.Wallet{},
		topUpsBySourceEvent:   map[string]ManualTopUp{},
		transactionsBySource:  map[string]wallet.Transaction{},
		releaseHoldsBySource:  map[string]ReleaseHoldResult{},
		settlementsBySource:   map[string]SettlementResult{},
		resourceUsageBySource: map[string]ResourceUsageResult{},
		requestUsageBySource:  map[string]RequestUsageResult{},
		requestUsageByRequest: map[string]RequestUsageResult{},
		requestQuotas:         map[string]RequestQuotaRecord{},
		auditBySourceEvent:    map[string]AuditEvent{},
		taskReceiptBySource:   map[string]TaskReceipt{},
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
	sourceEventID := manualTopUpSourceEventID(input)
	reason := manualTopUpReason(input, sourceEventID)
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.bySourceEvent[sourceEventID]; ok {
		replayUserID := input.UserID
		if replayUserID == "" {
			replayUserID = entry.UserID
		}
		replay := AppendEntryInput{
			EventType:     "credit",
			AccountID:     input.AccountID,
			UserID:        replayUserID,
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
			"reason":            reason,
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
			"reason":        reason,
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
		SourceEventID:       sourceEventID,
		Reason:              reason,
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

func (s *MemoryStore) CreateHold(_ context.Context, input HoldInput) (HoldResult, error) {
	if err := validateHoldInput(input); err != nil {
		return HoldResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, ok := s.bySourceEvent[input.SourceEventID]; ok {
		replay := holdAppendInput(input)
		if input.UserID == "" {
			replay.UserID = entry.UserID
		}
		if !sameReplayPayload(entry, replay) {
			return HoldResult{}, ErrIdempotencyConflict
		}
		transaction, ok := s.transactionsBySource[input.SourceEventID]
		if !ok {
			return HoldResult{}, errors.New("wallet_transaction_record_missing")
		}
		w := s.wallets[input.AccountID]
		return HoldResult{
			Wallet:      w.Snapshot(),
			Entry:       entry,
			Transaction: transaction,
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
	if err := w.AddHold(input.HoldType, input.AmountCents); err != nil {
		return HoldResult{}, err
	}
	after := w.Snapshot()
	createdAt := time.Now().UTC()
	entry := Entry{
		ID:            randomID(),
		EventType:     input.HoldType + "_hold",
		AccountID:     input.AccountID,
		UserID:        w.UserID,
		WorkspaceID:   workspaceOrResource(input.WorkspaceID),
		SourceEventID: input.SourceEventID,
		AmountCents:   input.AmountCents,
		Currency:      "CNY",
		CreatedAt:     createdAt,
	}
	if input.HoldType == "compute" {
		entry.ComputeID = input.ResourceID
	}
	if input.HoldType == "storage" {
		entry.StorageID = input.ResourceID
	}
	metadata := cloneMetadataMap(input.Metadata)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["holdType"] = input.HoldType
	metadata["resourceId"] = input.ResourceID
	metadata["packageId"] = input.PackageID
	transaction := wallet.NewTransaction(wallet.TransactionInput{
		UserID:              w.UserID,
		AccountID:           input.AccountID,
		WorkspaceID:         entry.WorkspaceID,
		Type:                wallet.TransactionHold,
		AmountCents:         input.AmountCents,
		Currency:            "CNY",
		SourceEventID:       input.SourceEventID,
		LedgerEntryID:       entry.ID,
		BalanceBeforeCents:  before.BalanceCents,
		BalanceAfterCents:   after.BalanceCents,
		FrozenBeforeCents:   before.FrozenCents,
		FrozenAfterCents:    after.FrozenCents,
		AvailableAfterCents: after.AvailableCents,
		Metadata:            metadata,
		CreatedAt:           createdAt,
	})
	s.wallets[input.AccountID] = w
	s.entries = append(s.entries, entry)
	s.bySourceEvent[input.SourceEventID] = entry
	s.walletTransactions = append(s.walletTransactions, transaction)
	s.transactionsBySource[input.SourceEventID] = transaction
	return HoldResult{
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
		Created:     true,
	}, nil
}

func (s *MemoryStore) ReleaseHolds(_ context.Context, input ReleaseHoldInput) (ReleaseHoldResult, error) {
	if err := validateReleaseHoldInput(input); err != nil {
		return ReleaseHoldResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.releaseHoldsBySource[input.SourceEventID]; ok {
		w := s.wallets[input.AccountID]
		existing.Wallet = w.Snapshot()
		existing.Created = false
		return existing, nil
	}

	w := s.wallets[input.AccountID]
	if w.AccountID == "" {
		w.AccountID = input.AccountID
		w.UserID = "usr-" + input.AccountID
	}
	var entries []Entry
	var transactions []wallet.Transaction
	multi := len(input.HoldTypes) > 1
	for _, holdType := range input.HoldTypes {
		before := w.Snapshot()
		released := w.ReleaseHold(holdType, before.Holds[holdType])
		if released <= 0 {
			continue
		}
		after := w.Snapshot()
		createdAt := time.Now().UTC()
		sourceEventID := holdReleaseSourceEventID(input.SourceEventID, holdType, multi)
		entry := Entry{
			ID:            randomID(),
			EventType:     holdType + "_hold_released",
			AccountID:     input.AccountID,
			UserID:        w.UserID,
			WorkspaceID:   workspaceOrResource(input.WorkspaceID),
			SourceEventID: sourceEventID,
			AmountCents:   -released,
			Currency:      "CNY",
			CreatedAt:     createdAt,
		}
		if holdType == "compute" {
			entry.ComputeID = input.ComputeID
		}
		if holdType == "storage" {
			entry.StorageID = input.StorageID
		}
		transaction := wallet.NewTransaction(wallet.TransactionInput{
			UserID:              w.UserID,
			AccountID:           input.AccountID,
			WorkspaceID:         entry.WorkspaceID,
			Type:                wallet.TransactionHoldRelease,
			AmountCents:         -released,
			Currency:            "CNY",
			SourceEventID:       sourceEventID,
			LedgerEntryID:       entry.ID,
			BalanceBeforeCents:  before.BalanceCents,
			BalanceAfterCents:   after.BalanceCents,
			FrozenBeforeCents:   before.FrozenCents,
			FrozenAfterCents:    after.FrozenCents,
			AvailableAfterCents: after.AvailableCents,
			Metadata: map[string]any{
				"reason":        input.Reason,
				"holdType":      holdType,
				"sourceEventId": input.SourceEventID,
			},
			CreatedAt: createdAt,
		})
		entries = append(entries, entry)
		transactions = append(transactions, transaction)
		s.entries = append(s.entries, entry)
		s.bySourceEvent[sourceEventID] = entry
		s.walletTransactions = append(s.walletTransactions, transaction)
		s.transactionsBySource[sourceEventID] = transaction
	}
	s.wallets[input.AccountID] = w
	result := ReleaseHoldResult{
		Wallet:       w.Snapshot(),
		Entries:      entries,
		Transactions: transactions,
		Created:      true,
	}
	s.releaseHoldsBySource[input.SourceEventID] = result
	return result, nil
}

func (s *MemoryStore) SettleWorkspaceUsage(_ context.Context, input SettlementInput) (SettlementResult, error) {
	if err := validateSettlementInput(input); err != nil {
		return SettlementResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.settlementsBySource[input.SourceEventID]; ok {
		w := s.wallets[input.AccountID]
		existing.Wallet = w.Snapshot()
		existing.Created = false
		return existing, nil
	}

	w := s.wallets[input.AccountID]
	if w.AccountID == "" {
		w.AccountID = input.AccountID
		w.UserID = input.UserID
		if w.UserID == "" {
			w.UserID = "usr-" + input.AccountID
		}
	}
	createdAt := time.Now().UTC()
	var entries []Entry
	var transactions []wallet.Transaction
	var intents []SettlementIntent
	var unpaid int64
	if input.ComputeActive {
		result := settleCharge(&w, settlementChargeInput{
			holdType:       "compute",
			resourceKind:   "compute",
			eventType:      "compute_debit",
			accountID:      input.AccountID,
			userID:         w.UserID,
			workspaceID:    input.WorkspaceID,
			resourceID:     input.ComputeID,
			sourceEventID:  input.SourceEventID,
			requestedCents: input.ComputeHourlyCents * input.Hours,
			billableHours:  input.Hours,
		}, createdAt)
		entries = append(entries, result.entries...)
		transactions = append(transactions, result.transactions...)
		unpaid += result.unpaidCents
		if result.exhaustedHold {
			intents = append(intents, SettlementIntent{
				Type:          IntentComputeAutoStopped,
				AccountID:     input.AccountID,
				WorkspaceID:   input.WorkspaceID,
				ComputeID:     input.ComputeID,
				SourceEventID: input.SourceEventID,
				Reason:        "compute_hold_exhausted",
			})
		}
	}
	if input.StorageActive {
		result := settleCharge(&w, settlementChargeInput{
			holdType:       "storage",
			resourceKind:   "storage",
			eventType:      "storage_debit",
			accountID:      input.AccountID,
			userID:         w.UserID,
			workspaceID:    input.WorkspaceID,
			resourceID:     input.StorageID,
			sourceEventID:  input.SourceEventID,
			requestedCents: input.StorageHourlyCents * input.Hours,
			billableHours:  input.Hours,
		}, createdAt)
		entries = append(entries, result.entries...)
		transactions = append(transactions, result.transactions...)
		unpaid += result.unpaidCents
		if result.unpaidCents > 0 || result.exhaustedHold {
			intents = append(intents, SettlementIntent{
				Type:          IntentStorageHoldExhausted,
				AccountID:     input.AccountID,
				WorkspaceID:   input.WorkspaceID,
				StorageID:     input.StorageID,
				SourceEventID: input.SourceEventID,
				Reason:        "storage_hold_exhausted",
			})
		}
	}
	for _, entry := range entries {
		s.entries = append(s.entries, entry)
		s.bySourceEvent[entry.SourceEventID] = entry
	}
	for _, transaction := range transactions {
		s.walletTransactions = append(s.walletTransactions, transaction)
		s.transactionsBySource[transaction.SourceEventID] = transaction
	}
	s.wallets[input.AccountID] = w
	result := SettlementResult{
		Wallet:       w.Snapshot(),
		Entries:      entries,
		Transactions: transactions,
		Intents:      intents,
		UnpaidCents:  unpaid,
		Created:      true,
	}
	s.settlementsBySource[input.SourceEventID] = result
	return result, nil
}

func (s *MemoryStore) RecordResourceUsage(_ context.Context, input ResourceUsageInput) (ResourceUsageResult, error) {
	if err := validateResourceUsageInput(input); err != nil {
		return ResourceUsageResult{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.resourceUsageBySource[input.SourceEventID]; ok {
		if !sameResourceUsageReplay(existing.Log, input) {
			return ResourceUsageResult{}, ErrIdempotencyConflict
		}
		existing.Created = false
		return existing, nil
	}

	log := usage.NewResourceUsageLog(toUsageResourceInput(input))
	result := ResourceUsageResult{Log: log, Created: true}
	s.resourceUsageBySource[input.SourceEventID] = result
	return result, nil
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
	now := time.Now().UTC()
	nextQuota, err := s.incrementRequestQuotaLocked(accountID, input.UserID, input.WorkspaceID, input.RequestQuota, now)
	if err != nil {
		return RequestUsageResult{}, err
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
	createdAt := now

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

func (s *MemoryStore) incrementRequestQuotaLocked(accountID string, userID string, workspaceID string, requestQuota *usage.RequestQuota, now time.Time) (*usage.RequestQuota, error) {
	if requestQuota != nil {
		return usage.IncrementOptionalRequestQuota(requestQuota, 1, now)
	}
	if userID == "" {
		return nil, nil
	}
	key := requestQuotaKey(accountID, userID, workspaceID)
	record, ok := s.requestQuotas[key]
	if !ok {
		return nil, nil
	}
	next, err := usage.IncrementRequestQuota(record.Quota, 1, now)
	if err != nil {
		return nil, err
	}
	record.Quota = next
	record.UpdatedAt = now
	s.requestQuotas[key] = record
	return &next, nil
}

func (s *MemoryStore) UpsertRequestQuota(_ context.Context, input RequestQuotaInput) (RequestQuotaRecord, error) {
	if err := validateRequestQuotaInput(input); err != nil {
		return RequestQuotaRecord{}, err
	}
	now := time.Now().UTC()
	key := requestQuotaKey(input.AccountID, input.UserID, input.WorkspaceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.requestQuotas[key]
	if !ok {
		record = RequestQuotaRecord{
			ID:          randomScopedID("quota"),
			AccountID:   input.AccountID,
			UserID:      input.UserID,
			WorkspaceID: input.WorkspaceID,
			CreatedAt:   now,
		}
	}
	record.Quota = input.Quota
	record.UpdatedAt = now
	s.requestQuotas[key] = record
	return record, nil
}

func (s *MemoryStore) ListRequestQuotas(_ context.Context, filter RequestQuotaFilter) ([]RequestQuotaRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []RequestQuotaRecord
	for _, record := range s.requestQuotas {
		if !matchesRequestQuota(record, filter) {
			continue
		}
		out = append(out, record)
	}
	return out, nil
}

func (s *MemoryStore) ListWalletTransactions(_ context.Context, filter WalletTransactionFilter) ([]wallet.Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []wallet.Transaction
	for _, transaction := range s.walletTransactions {
		if !matchesWalletTransaction(transaction, filter) {
			continue
		}
		out = append(out, cloneWalletTransaction(transaction))
	}
	return out, nil
}

func (s *MemoryStore) ListManualTopUps(_ context.Context, filter ManualTopUpFilter) ([]ManualTopUp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ManualTopUp
	for _, topup := range s.manualTopUps {
		if !matchesManualTopUp(topup, filter) {
			continue
		}
		out = append(out, topup)
	}
	return out, nil
}

func (s *MemoryStore) ListWallets(_ context.Context, filter WalletFilter) ([]wallet.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []wallet.Snapshot
	for _, w := range s.wallets {
		if !matchesWallet(w, filter) {
			continue
		}
		out = append(out, w.Snapshot())
	}
	return out, nil
}

func (s *MemoryStore) AppendAuditEvent(_ context.Context, input AuditEventInput) (AuditEvent, error) {
	event, err := auditlog.NewEvent(input)
	if err != nil {
		return AuditEvent{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditEvents = append(s.auditEvents, event)
	if event.SourceEventID != "" {
		s.auditBySourceEvent[event.SourceEventID] = event
	}
	return event, nil
}

func (s *MemoryStore) ListAuditEvents(_ context.Context, filter AuditEventFilter) ([]AuditEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []AuditEvent
	for _, event := range s.auditEvents {
		if auditlog.Matches(event, filter) {
			out = append(out, cloneAuditEvent(event))
		}
	}
	return out, nil
}

func (s *MemoryStore) AppendEvidenceRecord(_ context.Context, input EvidenceRecordInput) (EvidenceRecord, error) {
	record, err := evidencelog.NewRecord(input)
	if err != nil {
		return EvidenceRecord{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidenceRecords = append(s.evidenceRecords, record)
	return record, nil
}

func (s *MemoryStore) ListEvidenceRecords(_ context.Context, filter EvidenceRecordFilter) ([]EvidenceRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []EvidenceRecord
	for _, record := range s.evidenceRecords {
		if evidencelog.Matches(record, filter) {
			out = append(out, evidencelog.CloneRecord(record))
		}
	}
	return out, nil
}

func (s *MemoryStore) AppendKubernetesEvidenceSnapshot(_ context.Context, snapshot KubernetesEvidenceSnapshot) (KubernetesEvidenceSnapshot, error) {
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.kubernetesSnapshots = append(s.kubernetesSnapshots, cloneKubernetesEvidenceSnapshot(snapshot))
	return snapshot, nil
}

func (s *MemoryStore) ListKubernetesEvidenceSnapshots(_ context.Context, filter KubernetesEvidenceSnapshotFilter) ([]KubernetesEvidenceSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []KubernetesEvidenceSnapshot
	for _, snapshot := range s.kubernetesSnapshots {
		if !matchesKubernetesEvidenceSnapshot(snapshot, filter) {
			continue
		}
		out = append(out, cloneKubernetesEvidenceSnapshot(snapshot))
	}
	return out, nil
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
	receipt, err := newTaskReceipt(input, time.Now().UTC())
	if err != nil {
		return TaskReceipt{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if receipt.SourceEventID != "" {
		key := taskReceiptSourceKey(receipt.AccountID, receipt.WorkspaceID, receipt.TaskID, receipt.SourceEventID)
		if existing, ok := s.taskReceiptBySource[key]; ok {
			if !sameTaskReceiptReplay(existing, receipt) {
				return TaskReceipt{}, ErrIdempotencyConflict
			}
			return existing, nil
		}
		s.taskReceiptBySource[key] = receipt
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

func (s *MemoryStore) ListReconciliationReports(_ context.Context, filter ReconciliationReportFilter) ([]ReconciliationReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []ReconciliationReport
	for _, report := range s.reconciliationReports {
		if !matchesReconciliationReport(report, filter) {
			continue
		}
		out = append(out, cloneReconciliationReport(report))
	}
	return out, nil
}

func (s *MemoryStore) LatestReconciliationReport(_ context.Context) (ReconciliationReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reconciliationReports) == 0 {
		return ReconciliationReport{}, errors.New("billing_reconciliation_report_missing")
	}
	return s.reconciliationReports[len(s.reconciliationReports)-1], nil
}

func matchesReconciliationReport(report ReconciliationReport, filter ReconciliationReportFilter) bool {
	if filter.Provider != "" && report.Provider != filter.Provider {
		return false
	}
	if filter.Status != "" && report.Status != filter.Status {
		return false
	}
	return true
}

func cloneReconciliationReport(report ReconciliationReport) ReconciliationReport {
	if report.Payload == nil {
		return report
	}
	report.Payload = cloneMap(report.Payload)
	return report
}

func matchesKubernetesEvidenceSnapshot(snapshot KubernetesEvidenceSnapshot, filter KubernetesEvidenceSnapshotFilter) bool {
	if filter.ClusterID != "" && snapshot.ClusterID != filter.ClusterID {
		return false
	}
	if filter.Namespace != "" && snapshot.Namespace != filter.Namespace {
		return false
	}
	if filter.ObjectKind != "" && snapshot.ObjectKind != filter.ObjectKind {
		return false
	}
	if filter.ObjectName != "" && snapshot.ObjectName != filter.ObjectName {
		return false
	}
	if filter.WorkspaceID != "" && snapshot.WorkspaceID != filter.WorkspaceID {
		return false
	}
	return true
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

func cloneAuditEvent(event AuditEvent) AuditEvent {
	event.Payload = cloneMap(event.Payload)
	return event
}

func cloneKubernetesEvidenceSnapshot(snapshot KubernetesEvidenceSnapshot) KubernetesEvidenceSnapshot {
	snapshot.RedactedObject = cloneMap(snapshot.RedactedObject)
	return snapshot
}

var _ k8sevidence.SnapshotStore = (*MemoryStore)(nil)

func newTaskReceipt(input TaskReceiptInput, createdAt time.Time) (TaskReceipt, error) {
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
	return TaskReceipt{
		ID:            randomID(),
		Type:          "task.evidence.v1",
		AccountID:     input.AccountID,
		WorkspaceID:   input.WorkspaceID,
		TaskID:        input.TaskID,
		SourceEventID: input.SourceEventID,
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
		CreatedAt:     createdAt,
	}, nil
}

func taskReceiptSourceKey(accountID string, workspaceID string, taskID string, sourceEventID string) string {
	return accountID + "\x00" + workspaceID + "\x00" + taskID + "\x00" + sourceEventID
}

func sameTaskReceiptReplay(existing TaskReceipt, replay TaskReceipt) bool {
	return existing.Type == replay.Type &&
		existing.AccountID == replay.AccountID &&
		existing.WorkspaceID == replay.WorkspaceID &&
		existing.TaskID == replay.TaskID &&
		existing.SourceEventID == replay.SourceEventID &&
		reflect.DeepEqual(existing.Actor, replay.Actor) &&
		reflect.DeepEqual(existing.Plan, replay.Plan) &&
		reflect.DeepEqual(existing.Approval, replay.Approval) &&
		reflect.DeepEqual(existing.Environment, replay.Environment) &&
		reflect.DeepEqual(existing.InputRefs, replay.InputRefs) &&
		reflect.DeepEqual(existing.ExecutionRefs, replay.ExecutionRefs) &&
		reflect.DeepEqual(existing.OutputRefs, replay.OutputRefs) &&
		reflect.DeepEqual(existing.ReviewResults, replay.ReviewResults) &&
		reflect.DeepEqual(existing.Continuation, replay.Continuation) &&
		reflect.DeepEqual(existing.Metadata, replay.Metadata)
}

func randomID() string {
	return randomScopedID("led")
}

func randomScopedID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
