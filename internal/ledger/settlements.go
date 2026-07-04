package ledger

import (
	"errors"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

const (
	fundingSourceAvailableBalance = "available_balance"
	fundingSourceComputeHold      = "compute_hold"
	fundingSourceStorageHold      = "storage_hold"
)

func validateSettlementInput(input SettlementInput) error {
	if input.AccountID == "" {
		return errors.New("account_required")
	}
	if input.WorkspaceID == "" {
		return errors.New("workspace_required")
	}
	if input.SourceEventID == "" {
		return errors.New("source_event_required")
	}
	if input.Hours <= 0 {
		return errors.New("positive_hours_required")
	}
	if !input.ComputeActive && !input.StorageActive {
		return errors.New("active_resource_required")
	}
	if input.ComputeActive && input.ComputeHourlyCents < 0 {
		return errors.New("non_negative_compute_rate_required")
	}
	if input.StorageActive && input.StorageHourlyCents < 0 {
		return errors.New("non_negative_storage_rate_required")
	}
	return nil
}

type settlementChargeInput struct {
	holdType       string
	resourceKind   string
	eventType      string
	accountID      string
	userID         string
	workspaceID    string
	resourceID     string
	sourceEventID  string
	requestedCents int64
	billableHours  int64
}

type settlementChargeResult struct {
	entries       []Entry
	transactions  []wallet.Transaction
	unpaidCents   int64
	exhaustedHold bool
}

func settleCharge(w *wallet.Wallet, input settlementChargeInput, createdAt time.Time) settlementChargeResult {
	before := w.Snapshot()
	charge := w.Charge(input.holdType, input.requestedCents)
	after := w.Snapshot()
	var entries []Entry
	var transactions []wallet.Transaction
	if charge.AvailableCents > 0 {
		entry := newSettlementEntry(input, charge.AvailableCents, fundingSourceAvailableBalance, createdAt)
		entries = append(entries, entry)
		transactions = append(transactions, settlementTransaction(*w, before, after, entry, charge.AvailableCents, fundingSourceAvailableBalance, createdAt))
	}
	if charge.HoldCents > 0 {
		source := fundingSourceComputeHold
		if input.holdType == "storage" {
			source = fundingSourceStorageHold
		}
		entry := newSettlementEntry(input, charge.HoldCents, source, createdAt)
		entries = append(entries, entry)
		transactions = append(transactions, settlementTransaction(*w, before, after, entry, charge.HoldCents, source, createdAt))
	}
	return settlementChargeResult{
		entries:       entries,
		transactions:  transactions,
		unpaidCents:   charge.UnpaidCents,
		exhaustedHold: charge.ExhaustedHold,
	}
}

func newSettlementEntry(input settlementChargeInput, amountCents int64, fundingSource string, createdAt time.Time) Entry {
	entry := Entry{
		ID:            randomID(),
		EventType:     input.eventType,
		AccountID:     input.accountID,
		UserID:        input.userID,
		WorkspaceID:   input.workspaceID,
		SourceEventID: settlementSourceEventID(input.sourceEventID, input.resourceKind, fundingSource),
		AmountCents:   -amountCents,
		Currency:      "CNY",
		CreatedAt:     createdAt,
	}
	if input.holdType == "compute" {
		entry.ComputeID = input.resourceID
	}
	if input.holdType == "storage" {
		entry.StorageID = input.resourceID
	}
	return entry
}

func settlementTransaction(w wallet.Wallet, before wallet.Snapshot, after wallet.Snapshot, entry Entry, amountCents int64, fundingSource string, createdAt time.Time) wallet.Transaction {
	return wallet.NewTransaction(wallet.TransactionInput{
		UserID:              w.UserID,
		AccountID:           w.AccountID,
		WorkspaceID:         entry.WorkspaceID,
		Type:                wallet.TransactionDebit,
		AmountCents:         -amountCents,
		Currency:            "CNY",
		SourceEventID:       entry.SourceEventID,
		LedgerEntryID:       entry.ID,
		FundingSource:       fundingSource,
		BalanceBeforeCents:  before.BalanceCents,
		BalanceAfterCents:   after.BalanceCents,
		FrozenBeforeCents:   before.FrozenCents,
		FrozenAfterCents:    after.FrozenCents,
		AvailableAfterCents: after.AvailableCents,
		CreatedAt:           createdAt,
	})
}

func settlementSourceEventID(sourceEventID string, resourceKind string, fundingSource string) string {
	return sourceEventID + ":" + resourceKind + ":" + fundingSource
}
