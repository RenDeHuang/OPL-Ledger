package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	auditlog "github.com/RenDeHuang/OPL-Ledger/internal/audit"
	evidencelog "github.com/RenDeHuang/OPL-Ledger/internal/evidence"
	"github.com/RenDeHuang/OPL-Ledger/internal/usage"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

type Store interface {
	AppendEntry(context.Context, AppendEntryInput) (AppendEntryResult, error)
	ManualTopUp(context.Context, ManualTopUpInput) (ManualTopUpResult, error)
	CreateHold(context.Context, HoldInput) (HoldResult, error)
	ReleaseHolds(context.Context, ReleaseHoldInput) (ReleaseHoldResult, error)
	SettleWorkspaceUsage(context.Context, SettlementInput) (SettlementResult, error)
	RecordResourceUsage(context.Context, ResourceUsageInput) (ResourceUsageResult, error)
	RecordRequestUsage(context.Context, RequestUsageInput) (RequestUsageResult, error)
	ListWalletTransactions(context.Context, WalletTransactionFilter) ([]wallet.Transaction, error)
	AppendAuditEvent(context.Context, AuditEventInput) (AuditEvent, error)
	ListAuditEvents(context.Context, AuditEventFilter) ([]AuditEvent, error)
	AppendEvidenceRecord(context.Context, EvidenceRecordInput) (EvidenceRecord, error)
	ListEvidenceRecords(context.Context, EvidenceRecordFilter) ([]EvidenceRecord, error)
	AppendKubernetesEvidenceSnapshot(context.Context, KubernetesEvidenceSnapshot) (KubernetesEvidenceSnapshot, error)
	ListEntries(context.Context, EntryFilter) ([]Entry, error)
	Summary(context.Context, EntryFilter) (Summary, error)
	AppendTaskReceipt(context.Context, TaskReceiptInput) (TaskReceipt, error)
	ListTaskReceipts(context.Context, TaskReceiptFilter) ([]TaskReceipt, error)
	AppendReconciliationReport(context.Context, ReconciliationReport) (ReconciliationReport, error)
	LatestReconciliationReport(context.Context) (ReconciliationReport, error)
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) AppendEntry(ctx context.Context, input AppendEntryInput) (AppendEntryResult, error) {
	if input.EventType == "" {
		return AppendEntryResult{}, errors.New("eventType is required")
	}
	if input.SourceEventID == "" && input.RequestFingerprint == "" {
		return AppendEntryResult{}, errors.New("sourceEventId or requestFingerprint is required")
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendEntryResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	existing, err := s.entriesForIdempotencyKeys(ctx, tx, input)
	if err != nil {
		return AppendEntryResult{}, err
	}
	if len(existing) > 1 {
		return AppendEntryResult{}, ErrIdempotencyConflict
	}
	if len(existing) == 1 {
		entry := existing[0]
		if !sameReplayPayload(entry, input) {
			return AppendEntryResult{}, ErrIdempotencyConflict
		}
		if input.SourceEventID != "" && entry.SourceEventID == "" {
			entry.SourceEventID = input.SourceEventID
			if err := s.bindSourceEventPostgres(ctx, tx, entry.ID, input.SourceEventID); err != nil {
				return AppendEntryResult{}, err
			}
		}
		if input.RequestFingerprint != "" && entry.RequestFingerprint == "" {
			entry.RequestFingerprint = input.RequestFingerprint
			if err := s.bindRequestFingerprintPostgres(ctx, tx, entry.ID, input.RequestFingerprint); err != nil {
				return AppendEntryResult{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return AppendEntryResult{}, err
		}
		committed = true
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
		CreatedAt:          nowUTC(),
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id,
			source_event_id, request_fingerprint, amount_cents, currency, payload, created_at
		) VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), $11, $12, '{}'::jsonb, $13
		)`,
		entry.ID,
		entry.EventType,
		entry.AccountID,
		entry.UserID,
		entry.WorkspaceID,
		entry.ComputeID,
		entry.StorageID,
		entry.AttachmentID,
		entry.SourceEventID,
		entry.RequestFingerprint,
		entry.AmountCents,
		entry.Currency,
		entry.CreatedAt,
	)
	if err != nil {
		return AppendEntryResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AppendEntryResult{}, err
	}
	committed = true
	return AppendEntryResult{Entry: entry, Created: true}, nil
}

func (s *PostgresStore) ManualTopUp(ctx context.Context, input ManualTopUpInput) (ManualTopUpResult, error) {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	existing, err := s.entriesForIdempotencyKeys(ctx, tx, AppendEntryInput{SourceEventID: sourceEventID})
	if err != nil {
		return ManualTopUpResult{}, err
	}
	if len(existing) > 1 {
		return ManualTopUpResult{}, ErrIdempotencyConflict
	}
	if len(existing) == 1 {
		entry := existing[0]
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
		result, err := s.loadManualTopUpResult(ctx, tx, input.AccountID, sourceEventID, entry)
		if err != nil {
			return ManualTopUpResult{}, err
		}
		result.Created = false
		if err := tx.Commit(); err != nil {
			return ManualTopUpResult{}, err
		}
		committed = true
		return result, nil
	}

	w, found, err := s.walletForUpdate(ctx, tx, input.AccountID)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	if !found {
		w = wallet.Wallet{
			UserID:    input.UserID,
			AccountID: input.AccountID,
			Holds:     map[string]int64{},
		}
		if w.UserID == "" {
			w.UserID = "usr-" + input.AccountID
		}
	}
	before := w.Snapshot()
	w.Credit(input.AmountCents)
	after := w.Snapshot()
	createdAt := nowUTC()

	entry := Entry{
		ID:            randomID(),
		EventType:     "credit",
		AccountID:     input.AccountID,
		UserID:        w.UserID,
		WorkspaceID:   "account",
		SourceEventID: sourceEventID,
		AmountCents:   input.AmountCents,
		Currency:      "CNY",
		CreatedAt:     createdAt,
	}
	entryPayload, err := json.Marshal(map[string]any{
		"operatorUserId":    input.OperatorUserID,
		"operatorAccountId": input.OperatorAccountID,
	})
	if err != nil {
		return ManualTopUpResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id,
			source_event_id, request_fingerprint, amount_cents, currency, payload, created_at
		) VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13, $14
		)`,
		entry.ID,
		entry.EventType,
		entry.AccountID,
		entry.UserID,
		entry.WorkspaceID,
		entry.ComputeID,
		entry.StorageID,
		entry.AttachmentID,
		entry.SourceEventID,
		entry.RequestFingerprint,
		entry.AmountCents,
		entry.Currency,
		entryPayload,
		entry.CreatedAt,
	)
	if err != nil {
		return ManualTopUpResult{}, err
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
		CreatedAt: createdAt,
	})
	transactionPayload, err := json.Marshal(transaction)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO wallet_transactions (
			id, account_id, user_id, workspace_id, transaction_type, amount_cents, currency, source_event_id,
			ledger_entry_id, usage_log_id, funding_source, balance_before_cents, balance_after_cents,
			frozen_before_cents, frozen_after_cents, available_after_cents, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13, $14, $15, $16, $17, $18
		)`,
		transaction.ID,
		transaction.AccountID,
		transaction.UserID,
		transaction.WorkspaceID,
		string(transaction.Type),
		transaction.AmountCents,
		transaction.Currency,
		transaction.SourceEventID,
		transaction.LedgerEntryID,
		transaction.UsageLogID,
		transaction.FundingSource,
		transaction.BalanceBeforeCents,
		transaction.BalanceAfterCents,
		transaction.FrozenBeforeCents,
		transaction.FrozenAfterCents,
		transaction.AvailableAfterCents,
		transactionPayload,
		transaction.CreatedAt,
	)
	if err != nil {
		return ManualTopUpResult{}, err
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
		CreatedAt:           createdAt,
	}
	audit := AuditEvent{
		ID:            randomScopedID("aud"),
		AccountID:     input.AccountID,
		ActorID:       input.OperatorUserID,
		Action:        "account.credit_granted",
		TargetKind:    "manual_topup",
		TargetID:      topup.ID,
		SourceEventID: topup.ID,
		Payload: map[string]any{
			"sourceEventId": sourceEventID,
			"amountCents":   input.AmountCents,
		},
		CreatedAt: createdAt,
	}
	topup.AuditEventID = audit.ID
	auditPayload, err := json.Marshal(audit)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, account_id, workspace_id, actor_id, action, target_kind, target_id, source_event_id, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10
		)`,
		audit.ID,
		audit.AccountID,
		audit.WorkspaceID,
		audit.ActorID,
		audit.Action,
		audit.TargetKind,
		audit.TargetID,
		audit.SourceEventID,
		auditPayload,
		audit.CreatedAt,
	)
	if err != nil {
		return ManualTopUpResult{}, err
	}

	topupPayload, err := json.Marshal(topup)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO manual_topups (
			id, account_id, user_id, operator_id, operator_account_id, target_user_id, target_account_id, source_event_id,
			amount_cents, currency, status, balance_before_cents, balance_after_cents, ledger_entry_id,
			wallet_transaction_id, audit_event_id, payload, created_at
		) VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), $7, NULLIF($8, ''),
			$9, $10, $11, $12, $13, NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''), $17, $18
		)`,
		topup.ID,
		topup.TargetAccountID,
		topup.TargetUserID,
		topup.OperatorUserID,
		topup.OperatorAccountID,
		topup.TargetUserID,
		topup.TargetAccountID,
		topup.Reason,
		topup.AmountCents,
		topup.Currency,
		topup.Status,
		topup.BalanceBeforeCents,
		topup.BalanceAfterCents,
		topup.LedgerEntryID,
		topup.WalletTransactionID,
		topup.AuditEventID,
		topupPayload,
		topup.CreatedAt,
	)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	if err := s.upsertWallet(ctx, tx, w, after, createdAt); err != nil {
		return ManualTopUpResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManualTopUpResult{}, err
	}
	committed = true
	return ManualTopUpResult{
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
		TopUp:       topup,
		AuditEvent:  audit,
		Created:     true,
	}, nil
}

func (s *PostgresStore) CreateHold(ctx context.Context, input HoldInput) (HoldResult, error) {
	if err := validateHoldInput(input); err != nil {
		return HoldResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HoldResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	existing, err := s.entriesForIdempotencyKeys(ctx, tx, AppendEntryInput{SourceEventID: input.SourceEventID})
	if err != nil {
		return HoldResult{}, err
	}
	if len(existing) > 1 {
		return HoldResult{}, ErrIdempotencyConflict
	}
	if len(existing) == 1 {
		entry := existing[0]
		replay := holdAppendInput(input)
		if replay.UserID == "" {
			replay.UserID = entry.UserID
		}
		if !sameReplayPayload(entry, replay) {
			return HoldResult{}, ErrIdempotencyConflict
		}
		w, _, err := s.walletForUpdate(ctx, tx, input.AccountID)
		if err != nil {
			return HoldResult{}, err
		}
		transaction, err := loadWalletTransactionBySource(ctx, tx, input.SourceEventID)
		if err != nil {
			return HoldResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return HoldResult{}, err
		}
		committed = true
		return HoldResult{
			Wallet:      w.Snapshot(),
			Entry:       entry,
			Transaction: transaction,
			Created:     false,
		}, nil
	}

	w, found, err := s.walletForUpdate(ctx, tx, input.AccountID)
	if err != nil {
		return HoldResult{}, err
	}
	if !found {
		w = wallet.Wallet{
			UserID:    input.UserID,
			AccountID: input.AccountID,
			Holds:     map[string]int64{},
		}
		if w.UserID == "" {
			w.UserID = "usr-" + input.AccountID
		}
	}
	before := w.Snapshot()
	if err := w.AddHold(input.HoldType, input.AmountCents); err != nil {
		return HoldResult{}, err
	}
	after := w.Snapshot()
	createdAt := nowUTC()
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
	entryPayload, err := json.Marshal(metadata)
	if err != nil {
		return HoldResult{}, err
	}
	if err := insertLedgerEntry(ctx, tx, entry, entryPayload); err != nil {
		return HoldResult{}, err
	}
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
	if err := insertWalletTransaction(ctx, tx, transaction); err != nil {
		return HoldResult{}, err
	}
	if err := s.upsertWallet(ctx, tx, w, after, createdAt); err != nil {
		return HoldResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return HoldResult{}, err
	}
	committed = true
	return HoldResult{
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
		Created:     true,
	}, nil
}

func (s *PostgresStore) ReleaseHolds(ctx context.Context, input ReleaseHoldInput) (ReleaseHoldResult, error) {
	if err := validateReleaseHoldInput(input); err != nil {
		return ReleaseHoldResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReleaseHoldResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	multi := len(input.HoldTypes) > 1
	existingEntries, existingTransactions, err := loadExistingHoldRelease(ctx, tx, input, multi)
	if err != nil {
		return ReleaseHoldResult{}, err
	}
	if len(existingEntries) > 0 {
		w, _, err := s.walletForUpdate(ctx, tx, input.AccountID)
		if err != nil {
			return ReleaseHoldResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ReleaseHoldResult{}, err
		}
		committed = true
		return ReleaseHoldResult{
			Wallet:       w.Snapshot(),
			Entries:      existingEntries,
			Transactions: existingTransactions,
			Created:      false,
		}, nil
	}

	w, found, err := s.walletForUpdate(ctx, tx, input.AccountID)
	if err != nil {
		return ReleaseHoldResult{}, err
	}
	if !found {
		w = wallet.Wallet{
			UserID:    "usr-" + input.AccountID,
			AccountID: input.AccountID,
			Holds:     map[string]int64{},
		}
	}
	var entries []Entry
	var transactions []wallet.Transaction
	for _, holdType := range input.HoldTypes {
		before := w.Snapshot()
		released := w.ReleaseHold(holdType, before.Holds[holdType])
		if released <= 0 {
			continue
		}
		after := w.Snapshot()
		createdAt := nowUTC()
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
		entryPayload, err := json.Marshal(map[string]any{
			"reason":        input.Reason,
			"holdType":      holdType,
			"sourceEventId": input.SourceEventID,
		})
		if err != nil {
			return ReleaseHoldResult{}, err
		}
		if err := insertLedgerEntry(ctx, tx, entry, entryPayload); err != nil {
			return ReleaseHoldResult{}, err
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
		if err := insertWalletTransaction(ctx, tx, transaction); err != nil {
			return ReleaseHoldResult{}, err
		}
		entries = append(entries, entry)
		transactions = append(transactions, transaction)
	}
	after := w.Snapshot()
	if len(entries) > 0 {
		if err := s.upsertWallet(ctx, tx, w, after, nowUTC()); err != nil {
			return ReleaseHoldResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ReleaseHoldResult{}, err
	}
	committed = true
	return ReleaseHoldResult{
		Wallet:       after,
		Entries:      entries,
		Transactions: transactions,
		Created:      true,
	}, nil
}

func (s *PostgresStore) SettleWorkspaceUsage(ctx context.Context, input SettlementInput) (SettlementResult, error) {
	if err := validateSettlementInput(input); err != nil {
		return SettlementResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SettlementResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	existing, err := loadSettlementEntriesBySource(ctx, tx, input.SourceEventID)
	if err != nil {
		return SettlementResult{}, err
	}
	if len(existing) > 0 {
		w, _, err := s.walletForUpdate(ctx, tx, input.AccountID)
		if err != nil {
			return SettlementResult{}, err
		}
		transactions := make([]wallet.Transaction, 0, len(existing))
		for _, entry := range existing {
			transaction, err := loadWalletTransactionBySource(ctx, tx, entry.SourceEventID)
			if err != nil {
				return SettlementResult{}, err
			}
			transactions = append(transactions, transaction)
		}
		if err := tx.Commit(); err != nil {
			return SettlementResult{}, err
		}
		committed = true
		return SettlementResult{
			Wallet:       w.Snapshot(),
			Entries:      existing,
			Transactions: transactions,
			Created:      false,
		}, nil
	}

	w, found, err := s.walletForUpdate(ctx, tx, input.AccountID)
	if err != nil {
		return SettlementResult{}, err
	}
	if !found {
		w = wallet.Wallet{
			UserID:    input.UserID,
			AccountID: input.AccountID,
			Holds:     map[string]int64{},
		}
		if w.UserID == "" {
			w.UserID = "usr-" + input.AccountID
		}
	}
	createdAt := nowUTC()
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
	for i, entry := range entries {
		payload, err := json.Marshal(map[string]any{
			"sourceEventId":  input.SourceEventID,
			"fundingSource":  transactions[i].FundingSource,
			"billableHours":  input.Hours,
			"requestedCents": settlementRequestedCents(input, entry),
		})
		if err != nil {
			return SettlementResult{}, err
		}
		if err := insertLedgerEntry(ctx, tx, entry, payload); err != nil {
			return SettlementResult{}, err
		}
		if err := insertWalletTransaction(ctx, tx, transactions[i]); err != nil {
			return SettlementResult{}, err
		}
	}
	after := w.Snapshot()
	if err := s.upsertWallet(ctx, tx, w, after, createdAt); err != nil {
		return SettlementResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return SettlementResult{}, err
	}
	committed = true
	return SettlementResult{
		Wallet:       after,
		Entries:      entries,
		Transactions: transactions,
		Intents:      intents,
		UnpaidCents:  unpaid,
		Created:      true,
	}, nil
}

func (s *PostgresStore) RecordResourceUsage(ctx context.Context, input ResourceUsageInput) (ResourceUsageResult, error) {
	if err := validateResourceUsageInput(input); err != nil {
		return ResourceUsageResult{}, err
	}
	existing, found, err := s.loadResourceUsageBySource(ctx, input.SourceEventID)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	if found {
		if !sameResourceUsageReplay(existing, input) {
			return ResourceUsageResult{}, ErrIdempotencyConflict
		}
		return ResourceUsageResult{Log: existing, Created: false}, nil
	}
	log := usage.NewResourceUsageLog(toUsageResourceInput(input))
	payload, err := json.Marshal(log)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO resource_usage_logs (
			id, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, resource_kind,
			quantity, unit, unit_price_cents, amount_cents, requested_cents, currency, source_event_id, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8,
			$9, $10, $11, $12, $13, $14, $15, $16, $17
		)`,
		log.ID,
		log.AccountID,
		log.UserID,
		log.WorkspaceID,
		log.ComputeID,
		log.StorageID,
		log.AttachmentID,
		string(log.ResourceKind),
		log.Quantity,
		log.Unit,
		log.UnitPriceCents,
		log.AmountCents,
		log.RequestedCents,
		log.Currency,
		log.SourceEventID,
		payload,
		log.CreatedAt,
	)
	if err != nil {
		return ResourceUsageResult{}, err
	}
	return ResourceUsageResult{Log: log, Created: true}, nil
}

func (s *PostgresStore) RecordRequestUsage(ctx context.Context, input RequestUsageInput) (RequestUsageResult, error) {
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
	if input.RequestFingerprint == "" {
		input.RequestFingerprint = sourceEventID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RequestUsageResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	nextQuota, err := usage.IncrementOptionalRequestQuota(input.RequestQuota, 1, nowUTC())
	if err != nil {
		return RequestUsageResult{}, err
	}

	usageLogID, fingerprint, found, err := s.requestUsageDedup(ctx, tx, input.WorkspaceID, sourceEventID, input.RequestID)
	if err != nil {
		return RequestUsageResult{}, err
	}
	if found {
		if fingerprint != input.RequestFingerprint {
			return RequestUsageResult{}, ErrIdempotencyConflict
		}
		result, err := s.loadRequestUsageResult(ctx, tx, input.AccountID, sourceEventID, usageLogID)
		if err != nil {
			return RequestUsageResult{}, err
		}
		result.Created = false
		if err := tx.Commit(); err != nil {
			return RequestUsageResult{}, err
		}
		committed = true
		return result, nil
	}

	accountID := input.AccountID
	if accountID == "" {
		accountID = "acct-" + input.WorkspaceID
	}
	createdAt := nowUTC()
	logID := randomScopedID("usage")
	dedupID := randomScopedID("dedup")
	_, err = tx.ExecContext(ctx, `
		INSERT INTO request_usage_dedup (
			id, account_id, user_id, workspace_id, request_id, source_event_id, request_fingerprint, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7, $8
		)`,
		dedupID,
		accountID,
		input.UserID,
		input.WorkspaceID,
		input.RequestID,
		sourceEventID,
		input.RequestFingerprint,
		createdAt,
	)
	if err != nil {
		return RequestUsageResult{}, err
	}

	w, walletFound, err := s.walletForUpdate(ctx, tx, accountID)
	if err != nil {
		return RequestUsageResult{}, err
	}
	if !walletFound {
		w = wallet.Wallet{
			UserID:    input.UserID,
			AccountID: accountID,
			Holds:     map[string]int64{},
		}
		if w.UserID == "" {
			w.UserID = "usr-" + accountID
		}
	}
	before := w.Snapshot()
	charge := w.Charge("", input.AmountCents)
	after := w.Snapshot()

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
		entryPayload, err := json.Marshal(map[string]any{
			"requestId":          input.RequestID,
			"provider":           input.Provider,
			"model":              input.Model,
			"inputTokens":        input.InputTokens,
			"outputTokens":       input.OutputTokens,
			"requestedAmount":    input.AmountCents,
			"fundingSource":      "available_balance",
			"requestFingerprint": input.RequestFingerprint,
			"usageLogId":         logID,
		})
		if err != nil {
			return RequestUsageResult{}, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO ledger_entries (
				id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id,
				source_event_id, request_fingerprint, amount_cents, currency, payload, created_at
			) VALUES (
				$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
				NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13, $14
			)`,
			entry.ID,
			entry.EventType,
			entry.AccountID,
			entry.UserID,
			entry.WorkspaceID,
			entry.ComputeID,
			entry.StorageID,
			entry.AttachmentID,
			entry.SourceEventID,
			entry.RequestFingerprint,
			entry.AmountCents,
			entry.Currency,
			entryPayload,
			entry.CreatedAt,
		)
		if err != nil {
			return RequestUsageResult{}, err
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
			UsageLogID:          logID,
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
		transactionPayload, err := json.Marshal(transaction)
		if err != nil {
			return RequestUsageResult{}, err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO wallet_transactions (
				id, account_id, user_id, workspace_id, transaction_type, amount_cents, currency, source_event_id,
				ledger_entry_id, usage_log_id, funding_source, balance_before_cents, balance_after_cents,
				frozen_before_cents, frozen_after_cents, available_after_cents, payload, created_at
			) VALUES (
				$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, NULLIF($8, ''),
				NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13, $14, $15, $16, $17, $18
			)`,
			transaction.ID,
			transaction.AccountID,
			transaction.UserID,
			transaction.WorkspaceID,
			string(transaction.Type),
			transaction.AmountCents,
			transaction.Currency,
			transaction.SourceEventID,
			transaction.LedgerEntryID,
			transaction.UsageLogID,
			transaction.FundingSource,
			transaction.BalanceBeforeCents,
			transaction.BalanceAfterCents,
			transaction.FrozenBeforeCents,
			transaction.FrozenAfterCents,
			transaction.AvailableAfterCents,
			transactionPayload,
			transaction.CreatedAt,
		)
		if err != nil {
			return RequestUsageResult{}, err
		}
	}

	log := RequestUsageLog{
		ID:                   logID,
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
		LedgerEntryID:        entry.ID,
		Quota:                nextQuota,
		CreatedAt:            createdAt,
	}
	logPayload, err := json.Marshal(log)
	if err != nil {
		return RequestUsageResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO request_usage_logs (
			id, account_id, user_id, workspace_id, request_id, source_event_id, request_fingerprint,
			provider, model, input_tokens, output_tokens, amount_cents, requested_amount_cents, unpaid_cents,
			currency, ledger_entry_id, units, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7,
			NULLIF($8, ''), NULLIF($9, ''), $10, $11, $12, $13, $14,
			$15, NULLIF($16, ''), $17, $18, $19
		)`,
		log.ID,
		log.AccountID,
		log.UserID,
		log.WorkspaceID,
		log.RequestID,
		log.SourceEventID,
		log.RequestFingerprint,
		log.Provider,
		log.Model,
		log.InputTokens,
		log.OutputTokens,
		log.AmountCents,
		log.RequestedAmountCents,
		log.UnpaidCents,
		log.Currency,
		log.LedgerEntryID,
		int64(1),
		logPayload,
		log.CreatedAt,
	)
	if err != nil {
		return RequestUsageResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE request_usage_dedup
		SET account_id = NULLIF($1, ''), user_id = NULLIF($2, ''), usage_log_id = $3
		WHERE id = $4`,
		accountID,
		w.UserID,
		log.ID,
		dedupID,
	)
	if err != nil {
		return RequestUsageResult{}, err
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
	auditPayload, err := json.Marshal(audit)
	if err != nil {
		return RequestUsageResult{}, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, account_id, workspace_id, actor_id, action, target_kind, target_id, source_event_id, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10
		)`,
		audit.ID,
		audit.AccountID,
		audit.WorkspaceID,
		audit.ActorID,
		audit.Action,
		audit.TargetKind,
		audit.TargetID,
		audit.SourceEventID,
		auditPayload,
		audit.CreatedAt,
	)
	if err != nil {
		return RequestUsageResult{}, err
	}
	if err := s.upsertWallet(ctx, tx, w, after, createdAt); err != nil {
		return RequestUsageResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RequestUsageResult{}, err
	}
	committed = true
	return RequestUsageResult{
		Log:         log,
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
		AuditEvent:  audit,
		Created:     true,
	}, nil
}

func (s *PostgresStore) ListWalletTransactions(ctx context.Context, filter WalletTransactionFilter) ([]wallet.Transaction, error) {
	where, args := walletTransactionWhere(filter)
	query := `SELECT payload FROM wallet_transactions`
	if where != "" {
		query += ` WHERE ` + where
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanWalletTransactions(rows)
}

func (s *PostgresStore) AppendAuditEvent(ctx context.Context, input AuditEventInput) (AuditEvent, error) {
	event, err := auditlog.NewEvent(input)
	if err != nil {
		return AuditEvent{}, err
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return AuditEvent{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO audit_events (
			id, account_id, workspace_id, actor_id, action, target_kind, target_id, source_event_id, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, NULLIF($7, ''), NULLIF($8, ''), $9, $10
		)`,
		event.ID,
		event.AccountID,
		event.WorkspaceID,
		event.ActorID,
		event.Action,
		event.TargetKind,
		event.TargetID,
		event.SourceEventID,
		payload,
		event.CreatedAt,
	)
	if err != nil {
		return AuditEvent{}, err
	}
	return event, nil
}

func (s *PostgresStore) ListAuditEvents(ctx context.Context, filter AuditEventFilter) ([]AuditEvent, error) {
	where, args := auditEventWhere(filter)
	query := selectAuditEventColumns + ` FROM audit_events`
	if where != "" {
		query += ` WHERE ` + where
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAuditEvents(rows)
}

func (s *PostgresStore) AppendEvidenceRecord(ctx context.Context, input EvidenceRecordInput) (EvidenceRecord, error) {
	record, err := evidencelog.NewRecord(input)
	if err != nil {
		return EvidenceRecord{}, err
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return EvidenceRecord{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO evidence_records (
			id, evidence_type, account_id, workspace_id, source_event_id, payload, created_at
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), $6, $7
		)`,
		record.ID,
		record.Type,
		record.AccountID,
		record.WorkspaceID,
		record.SourceEventID,
		payload,
		record.CreatedAt,
	)
	if err != nil {
		return EvidenceRecord{}, err
	}
	return record, nil
}

func (s *PostgresStore) ListEvidenceRecords(ctx context.Context, filter EvidenceRecordFilter) ([]EvidenceRecord, error) {
	where, args := evidenceRecordWhere(filter)
	query := `SELECT payload FROM evidence_records`
	if where != "" {
		query += ` WHERE ` + where
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvidenceRecords(rows)
}

func (s *PostgresStore) AppendKubernetesEvidenceSnapshot(ctx context.Context, snapshot KubernetesEvidenceSnapshot) (KubernetesEvidenceSnapshot, error) {
	if snapshot.CollectedAt.IsZero() {
		snapshot.CollectedAt = nowUTC()
	}
	redactedObject, err := json.Marshal(snapshot.RedactedObject)
	if err != nil {
		return KubernetesEvidenceSnapshot{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO kubernetes_evidence_snapshots (
			id, cluster_id, namespace, object_kind, object_name, workspace_id, resource_version,
			observed_generation, readiness_status, redacted_object, collected_at
		) VALUES (
			$1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9, $10, $11
		)`,
		randomScopedID("kes"),
		snapshot.ClusterID,
		snapshot.Namespace,
		snapshot.ObjectKind,
		snapshot.ObjectName,
		snapshot.WorkspaceID,
		snapshot.ResourceVersion,
		snapshot.ObservedGeneration,
		snapshot.ReadinessStatus,
		redactedObject,
		snapshot.CollectedAt,
	)
	if err != nil {
		return KubernetesEvidenceSnapshot{}, err
	}
	return snapshot, nil
}

func (s *PostgresStore) entriesForIdempotencyKeys(ctx context.Context, tx *sql.Tx, input AppendEntryInput) ([]Entry, error) {
	rows, err := tx.QueryContext(ctx, selectLedgerEntryColumns+` FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`, input.SourceEventID, input.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (s *PostgresStore) walletForUpdate(ctx context.Context, tx *sql.Tx, accountID string) (wallet.Wallet, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds, created_at, updated_at
		FROM wallets
		WHERE account_id = $1
		FOR UPDATE`,
		accountID,
	)
	w, err := scanWallet(row)
	if errors.Is(err, sql.ErrNoRows) {
		return wallet.Wallet{}, false, nil
	}
	if err != nil {
		return wallet.Wallet{}, false, err
	}
	return w, true, nil
}

func (s *PostgresStore) upsertWallet(ctx context.Context, tx *sql.Tx, w wallet.Wallet, snapshot wallet.Snapshot, timestamp time.Time) error {
	holds, err := json.Marshal(snapshot.Holds)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO wallets (
			id, user_id, account_id, balance_cents, frozen_cents, total_recharged_cents, holds, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
		ON CONFLICT (account_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			balance_cents = EXCLUDED.balance_cents,
			frozen_cents = EXCLUDED.frozen_cents,
			total_recharged_cents = EXCLUDED.total_recharged_cents,
			holds = EXCLUDED.holds,
			updated_at = EXCLUDED.updated_at`,
		randomScopedID("wal"),
		w.UserID,
		w.AccountID,
		snapshot.BalanceCents,
		snapshot.FrozenCents,
		snapshot.TotalRechargedCents,
		holds,
		timestamp,
		timestamp,
	)
	return err
}

func (s *PostgresStore) loadManualTopUpResult(ctx context.Context, tx *sql.Tx, accountID string, sourceEventID string, entry Entry) (ManualTopUpResult, error) {
	w, _, err := s.walletForUpdate(ctx, tx, accountID)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	transaction, err := loadWalletTransactionBySource(ctx, tx, sourceEventID)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	topup, err := loadManualTopUpBySource(ctx, tx, sourceEventID)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	audit, err := loadAuditEventBySource(ctx, tx, topup.ID)
	if err != nil {
		return ManualTopUpResult{}, err
	}
	return ManualTopUpResult{
		Wallet:      w.Snapshot(),
		Entry:       entry,
		Transaction: transaction,
		TopUp:       topup,
		AuditEvent:  audit,
	}, nil
}

func (s *PostgresStore) requestUsageDedup(ctx context.Context, tx *sql.Tx, workspaceID string, sourceEventID string, requestID string) (string, string, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT usage_log_id, request_fingerprint
		FROM request_usage_dedup
		WHERE workspace_id = $1 AND (source_event_id = $2 OR request_id = $3)
		ORDER BY created_at, id
		LIMIT 1`,
		workspaceID,
		sourceEventID,
		requestID,
	)
	var usageLogID string
	var fingerprint string
	err := row.Scan(&usageLogID, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return usageLogID, fingerprint, true, nil
}

func (s *PostgresStore) loadRequestUsageResult(ctx context.Context, tx *sql.Tx, accountID string, sourceEventID string, usageLogID string) (RequestUsageResult, error) {
	log, err := loadRequestUsageLogByID(ctx, tx, usageLogID)
	if err != nil {
		return RequestUsageResult{}, err
	}
	if accountID == "" {
		accountID = log.AccountID
	}
	w, _, err := s.walletForUpdate(ctx, tx, accountID)
	if err != nil {
		return RequestUsageResult{}, err
	}
	var entry Entry
	if log.LedgerEntryID != "" {
		entry, err = loadLedgerEntryByID(ctx, tx, log.LedgerEntryID)
		if err != nil {
			return RequestUsageResult{}, err
		}
	}
	var transaction wallet.Transaction
	if log.LedgerEntryID != "" {
		transaction, err = loadWalletTransactionBySource(ctx, tx, sourceEventID)
		if err != nil {
			return RequestUsageResult{}, err
		}
	}
	audit, err := loadAuditEventBySource(ctx, tx, sourceEventID)
	if err != nil {
		return RequestUsageResult{}, err
	}
	return RequestUsageResult{
		Log:         log,
		Wallet:      w.Snapshot(),
		Entry:       entry,
		Transaction: transaction,
		AuditEvent:  audit,
	}, nil
}

func loadRequestUsageLogByID(ctx context.Context, tx *sql.Tx, id string) (RequestUsageLog, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT payload
		FROM request_usage_logs
		WHERE id = $1`,
		id,
	)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return RequestUsageLog{}, err
	}
	var log RequestUsageLog
	if err := json.Unmarshal(payload, &log); err != nil {
		return RequestUsageLog{}, err
	}
	return log, nil
}

func (s *PostgresStore) loadResourceUsageBySource(ctx context.Context, sourceEventID string) (usage.ResourceUsageLog, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT payload
		FROM resource_usage_logs
		WHERE source_event_id = $1
		ORDER BY created_at, id
		LIMIT 1`,
		sourceEventID,
	)
	var payload []byte
	err := row.Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return usage.ResourceUsageLog{}, false, nil
	}
	if err != nil {
		return usage.ResourceUsageLog{}, false, err
	}
	var log usage.ResourceUsageLog
	if err := json.Unmarshal(payload, &log); err != nil {
		return usage.ResourceUsageLog{}, false, err
	}
	return log, true, nil
}

func loadLedgerEntryByID(ctx context.Context, tx *sql.Tx, id string) (Entry, error) {
	row := tx.QueryRowContext(ctx, selectLedgerEntryColumns+` FROM ledger_entries WHERE id = $1`, id)
	return scanEntry(row)
}

func insertLedgerEntry(ctx context.Context, tx *sql.Tx, entry Entry, payload []byte) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id,
			source_event_id, request_fingerprint, amount_cents, currency, payload, created_at
		) VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), $11, $12, $13, $14
		)`,
		entry.ID,
		entry.EventType,
		entry.AccountID,
		entry.UserID,
		entry.WorkspaceID,
		entry.ComputeID,
		entry.StorageID,
		entry.AttachmentID,
		entry.SourceEventID,
		entry.RequestFingerprint,
		entry.AmountCents,
		entry.Currency,
		payload,
		entry.CreatedAt,
	)
	return err
}

func insertWalletTransaction(ctx context.Context, tx *sql.Tx, transaction wallet.Transaction) error {
	payload, err := json.Marshal(transaction)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO wallet_transactions (
			id, account_id, user_id, workspace_id, transaction_type, amount_cents, currency, source_event_id,
			ledger_entry_id, usage_log_id, funding_source, balance_before_cents, balance_after_cents,
			frozen_before_cents, frozen_after_cents, available_after_cents, payload, created_at
		) VALUES (
			$1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, $6, $7, NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13, $14, $15, $16, $17, $18
		)`,
		transaction.ID,
		transaction.AccountID,
		transaction.UserID,
		transaction.WorkspaceID,
		string(transaction.Type),
		transaction.AmountCents,
		transaction.Currency,
		transaction.SourceEventID,
		transaction.LedgerEntryID,
		transaction.UsageLogID,
		transaction.FundingSource,
		transaction.BalanceBeforeCents,
		transaction.BalanceAfterCents,
		transaction.FrozenBeforeCents,
		transaction.FrozenAfterCents,
		transaction.AvailableAfterCents,
		payload,
		transaction.CreatedAt,
	)
	return err
}

func loadExistingHoldRelease(ctx context.Context, tx *sql.Tx, input ReleaseHoldInput, multi bool) ([]Entry, []wallet.Transaction, error) {
	var entries []Entry
	var transactions []wallet.Transaction
	for _, holdType := range input.HoldTypes {
		sourceEventID := holdReleaseSourceEventID(input.SourceEventID, holdType, multi)
		existing, err := entriesForSourceEvent(ctx, tx, sourceEventID)
		if err != nil {
			return nil, nil, err
		}
		if len(existing) == 0 {
			continue
		}
		if len(existing) > 1 {
			return nil, nil, ErrIdempotencyConflict
		}
		entry := existing[0]
		if !sameHoldReleaseReplay(entry, input, holdType, sourceEventID) {
			return nil, nil, ErrIdempotencyConflict
		}
		transaction, err := loadWalletTransactionBySource(ctx, tx, sourceEventID)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, entry)
		transactions = append(transactions, transaction)
	}
	return entries, transactions, nil
}

func entriesForSourceEvent(ctx context.Context, tx *sql.Tx, sourceEventID string) ([]Entry, error) {
	rows, err := tx.QueryContext(ctx, selectLedgerEntryColumns+` FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`, sourceEventID, "")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func loadSettlementEntriesBySource(ctx context.Context, tx *sql.Tx, sourceEventID string) ([]Entry, error) {
	rows, err := tx.QueryContext(ctx, selectLedgerEntryColumns+` FROM ledger_entries WHERE source_event_id = $1 OR source_event_id LIKE $2 ORDER BY created_at, id`, sourceEventID, sourceEventID+":%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func settlementRequestedCents(input SettlementInput, entry Entry) int64 {
	if entry.ComputeID != "" {
		return input.ComputeHourlyCents * input.Hours
	}
	if entry.StorageID != "" {
		return input.StorageHourlyCents * input.Hours
	}
	return 0
}

func sameHoldReleaseReplay(entry Entry, input ReleaseHoldInput, holdType string, sourceEventID string) bool {
	expected := AppendEntryInput{
		EventType:     holdType + "_hold_released",
		AccountID:     input.AccountID,
		UserID:        entry.UserID,
		WorkspaceID:   workspaceOrResource(input.WorkspaceID),
		SourceEventID: sourceEventID,
		AmountCents:   entry.AmountCents,
		Currency:      "CNY",
	}
	if holdType == "compute" {
		expected.ComputeID = input.ComputeID
	}
	if holdType == "storage" {
		expected.StorageID = input.StorageID
	}
	return sameReplayPayload(entry, expected)
}

type walletScanner interface {
	Scan(dest ...any) error
}

func scanWallet(scanner walletScanner) (wallet.Wallet, error) {
	var id string
	var w wallet.Wallet
	var frozen int64
	var holdsBytes []byte
	var createdAt time.Time
	var updatedAt time.Time
	err := scanner.Scan(
		&id,
		&w.UserID,
		&w.AccountID,
		&w.BalanceCents,
		&frozen,
		&w.TotalRechargedCents,
		&holdsBytes,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return wallet.Wallet{}, err
	}
	if len(holdsBytes) > 0 {
		if err := json.Unmarshal(holdsBytes, &w.Holds); err != nil {
			return wallet.Wallet{}, err
		}
	}
	if w.Holds == nil {
		w.Holds = map[string]int64{}
	}
	return w, nil
}

func loadWalletTransactionBySource(ctx context.Context, tx *sql.Tx, sourceEventID string) (wallet.Transaction, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT payload
		FROM wallet_transactions
		WHERE source_event_id = $1
		ORDER BY created_at, id
		LIMIT 1`,
		sourceEventID,
	)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return wallet.Transaction{}, err
	}
	var transaction wallet.Transaction
	if err := json.Unmarshal(payload, &transaction); err != nil {
		return wallet.Transaction{}, err
	}
	return transaction, nil
}

func scanWalletTransactions(rows *sql.Rows) ([]wallet.Transaction, error) {
	var transactions []wallet.Transaction
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var transaction wallet.Transaction
		if err := json.Unmarshal(payload, &transaction); err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return transactions, nil
}

func loadManualTopUpBySource(ctx context.Context, tx *sql.Tx, sourceEventID string) (ManualTopUp, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT payload
		FROM manual_topups
		WHERE source_event_id = $1
		ORDER BY created_at, id
		LIMIT 1`,
		sourceEventID,
	)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return ManualTopUp{}, err
	}
	var topup ManualTopUp
	if err := json.Unmarshal(payload, &topup); err != nil {
		return ManualTopUp{}, err
	}
	return topup, nil
}

func loadAuditEventBySource(ctx context.Context, tx *sql.Tx, sourceEventID string) (AuditEvent, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT payload
		FROM audit_events
		WHERE source_event_id = $1
		ORDER BY created_at, id
		LIMIT 1`,
		sourceEventID,
	)
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		return AuditEvent{}, err
	}
	var audit AuditEvent
	if err := json.Unmarshal(payload, &audit); err != nil {
		return AuditEvent{}, err
	}
	return audit, nil
}

func (s *PostgresStore) bindSourceEventPostgres(ctx context.Context, tx *sql.Tx, id string, sourceEventID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE ledger_entries SET source_event_id = NULLIF($1, '') WHERE id = $2`, sourceEventID, id)
	return err
}

func (s *PostgresStore) bindRequestFingerprintPostgres(ctx context.Context, tx *sql.Tx, id string, requestFingerprint string) error {
	_, err := tx.ExecContext(ctx, `UPDATE ledger_entries SET request_fingerprint = NULLIF($1, '') WHERE id = $2`, requestFingerprint, id)
	return err
}

func (s *PostgresStore) ListEntries(ctx context.Context, filter EntryFilter) ([]Entry, error) {
	where, args := ledgerEntryWhere(filter)
	query := selectLedgerEntryColumns + ` FROM ledger_entries`
	if where != "" {
		query += ` WHERE ` + where
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (s *PostgresStore) Summary(ctx context.Context, filter EntryFilter) (Summary, error) {
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

func (s *PostgresStore) AppendTaskReceipt(ctx context.Context, input TaskReceiptInput) (TaskReceipt, error) {
	receipt, err := newTaskReceipt(input, time.Now().UTC())
	if err != nil {
		return TaskReceipt{}, err
	}
	if receipt.SourceEventID != "" {
		existing, found, err := s.loadTaskReceiptBySource(ctx, receipt.AccountID, receipt.WorkspaceID, receipt.TaskID, receipt.SourceEventID)
		if err != nil {
			return TaskReceipt{}, err
		}
		if found {
			if !sameTaskReceiptReplay(existing, receipt) {
				return TaskReceipt{}, ErrIdempotencyConflict
			}
			return existing, nil
		}
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return TaskReceipt{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO task_receipts (id, account_id, workspace_id, task_id, source_event_id, receipt_type, status, payload, created_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6, $7, $8, $9)`,
		receipt.ID,
		receipt.AccountID,
		receipt.WorkspaceID,
		receipt.TaskID,
		receipt.SourceEventID,
		receipt.Type,
		"recorded",
		payload,
		receipt.CreatedAt,
	)
	if err != nil {
		return TaskReceipt{}, err
	}
	return receipt, nil
}

func (s *PostgresStore) loadTaskReceiptBySource(ctx context.Context, accountID string, workspaceID string, taskID string, sourceEventID string) (TaskReceipt, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT payload
		FROM task_receipts
		WHERE account_id = $1
			AND COALESCE(workspace_id, '') = $2
			AND task_id = $3
			AND source_event_id = $4
		ORDER BY created_at, id
		LIMIT 1`,
		accountID,
		workspaceID,
		taskID,
		sourceEventID,
	)
	var payload []byte
	err := row.Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return TaskReceipt{}, false, nil
	}
	if err != nil {
		return TaskReceipt{}, false, err
	}
	var receipt TaskReceipt
	if err := json.Unmarshal(payload, &receipt); err != nil {
		return TaskReceipt{}, false, err
	}
	return receipt, true, nil
}

func (s *PostgresStore) ListTaskReceipts(ctx context.Context, filter TaskReceiptFilter) ([]TaskReceipt, error) {
	if filter.AccountID == "" {
		return nil, errors.New("task_evidence_account_required")
	}
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("workspace_id", filter.WorkspaceID)
	add("task_id", filter.TaskID)
	query := `SELECT payload FROM task_receipts`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, " AND ")
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var receipts []TaskReceipt
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var receipt TaskReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func (s *PostgresStore) AppendReconciliationReport(ctx context.Context, report ReconciliationReport) (ReconciliationReport, error) {
	if report.ID == "" {
		report.ID = randomID()
	}
	if report.Provider == "" {
		report.Provider = "manual"
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(report.Payload)
	if err != nil {
		return ReconciliationReport{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO billing_reconciliation_reports (
			id, provider, account_id, status, expected_amount_cents, actual_amount_cents, difference_cents, payload, created_at
		) VALUES ($1, $2, '', $3, $4, $5, $6, $7, $8)`,
		report.ID,
		report.Provider,
		report.Status,
		report.ExpectedAmountCents,
		report.LedgerAmountCents,
		report.DifferenceCents,
		payload,
		report.CreatedAt,
	)
	if err != nil {
		return ReconciliationReport{}, err
	}
	return report, nil
}

func (s *PostgresStore) LatestReconciliationReport(ctx context.Context) (ReconciliationReport, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, provider, status, expected_amount_cents, actual_amount_cents, difference_cents, payload, created_at
		FROM billing_reconciliation_reports
		ORDER BY created_at DESC, id DESC
		LIMIT 1`)
	var report ReconciliationReport
	var payload []byte
	err := row.Scan(
		&report.ID,
		&report.Provider,
		&report.Status,
		&report.ExpectedAmountCents,
		&report.LedgerAmountCents,
		&report.DifferenceCents,
		&payload,
		&report.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReconciliationReport{}, errors.New("billing_reconciliation_report_missing")
	}
	if err != nil {
		return ReconciliationReport{}, err
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &report.Payload); err != nil {
			return ReconciliationReport{}, err
		}
	}
	return report, nil
}

const selectLedgerEntryColumns = `SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at`
const selectAuditEventColumns = `SELECT id, account_id, workspace_id, actor_id, action, target_kind, target_id, source_event_id, payload, created_at`

func ledgerEntryWhere(filter EntryFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("user_id", filter.UserID)
	add("workspace_id", filter.WorkspaceID)
	add("compute_id", filter.ComputeID)
	add("storage_id", filter.StorageID)
	add("attachment_id", filter.AttachmentID)
	add("source_event_id", filter.SourceEventID)
	return strings.Join(clauses, " AND "), args
}

func walletTransactionWhere(filter WalletTransactionFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("user_id", filter.UserID)
	add("workspace_id", filter.WorkspaceID)
	add("transaction_type", string(filter.Type))
	add("source_event_id", filter.SourceEventID)
	add("ledger_entry_id", filter.LedgerEntryID)
	add("usage_log_id", filter.UsageLogID)
	add("funding_source", filter.FundingSource)
	return strings.Join(clauses, " AND "), args
}

func auditEventWhere(filter AuditEventFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("workspace_id", filter.WorkspaceID)
	add("action", filter.Action)
	add("source_event_id", filter.SourceEventID)
	return strings.Join(clauses, " AND "), args
}

func evidenceRecordWhere(filter EvidenceRecordFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("workspace_id", filter.WorkspaceID)
	add("evidence_type", filter.Type)
	add("source_event_id", filter.SourceEventID)
	return strings.Join(clauses, " AND "), args
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func scanAuditEvents(rows *sql.Rows) ([]AuditEvent, error) {
	var events []AuditEvent
	for rows.Next() {
		event, err := scanAuditEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func scanEvidenceRecords(rows *sql.Rows) ([]EvidenceRecord, error) {
	var records []EvidenceRecord
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var record EvidenceRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

type ledgerEntryScanner interface {
	Scan(dest ...any) error
}

type auditEventScanner interface {
	Scan(dest ...any) error
}

func scanEntry(scanner ledgerEntryScanner) (Entry, error) {
	var entry Entry
	var accountID sql.NullString
	var userID sql.NullString
	var workspaceID sql.NullString
	var computeID sql.NullString
	var storageID sql.NullString
	var attachmentID sql.NullString
	var sourceEventID sql.NullString
	var requestFingerprint sql.NullString
	err := scanner.Scan(
		&entry.ID,
		&entry.EventType,
		&accountID,
		&userID,
		&workspaceID,
		&computeID,
		&storageID,
		&attachmentID,
		&sourceEventID,
		&requestFingerprint,
		&entry.AmountCents,
		&entry.Currency,
		&entry.CreatedAt,
	)
	if err != nil {
		return Entry{}, err
	}
	entry.AccountID = accountID.String
	entry.UserID = userID.String
	entry.WorkspaceID = workspaceID.String
	entry.ComputeID = computeID.String
	entry.StorageID = storageID.String
	entry.AttachmentID = attachmentID.String
	entry.SourceEventID = sourceEventID.String
	entry.RequestFingerprint = requestFingerprint.String
	return entry, nil
}

func scanAuditEvent(scanner auditEventScanner) (AuditEvent, error) {
	var event AuditEvent
	var accountID sql.NullString
	var workspaceID sql.NullString
	var actorID sql.NullString
	var targetID sql.NullString
	var sourceEventID sql.NullString
	var payload []byte
	err := scanner.Scan(
		&event.ID,
		&accountID,
		&workspaceID,
		&actorID,
		&event.Action,
		&event.TargetKind,
		&targetID,
		&sourceEventID,
		&payload,
		&event.CreatedAt,
	)
	if err != nil {
		return AuditEvent{}, err
	}
	event.AccountID = accountID.String
	event.WorkspaceID = workspaceID.String
	event.ActorID = actorID.String
	event.TargetID = targetID.String
	event.SourceEventID = sourceEventID.String
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &event.Payload); err != nil {
			return AuditEvent{}, err
		}
	}
	return event, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
