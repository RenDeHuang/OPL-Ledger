package billing

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

type FundingSource string

const (
	FundingSourceAvailableBalance FundingSource = "available_balance"
	FundingSourceComputeHold      FundingSource = "compute_hold"
	FundingSourceStorageHold      FundingSource = "storage_hold"
)

type SettlementInput struct {
	Wallet             wallet.Wallet
	AccountID          string
	UserID             string
	WorkspaceID        string
	ComputeID          string
	StorageID          string
	SourceEventID      string
	Hours              int64
	ComputeActive      bool
	StorageActive      bool
	ComputeHourlyCents int64
	StorageHourlyCents int64
	ExistingEntries    []SettlementEntry
}

type SettlementResult struct {
	Wallet       wallet.Snapshot      `json:"wallet"`
	Entries      []SettlementEntry    `json:"entries"`
	Transactions []wallet.Transaction `json:"transactions"`
	Intents      []SettlementIntent   `json:"intents,omitempty"`
	UnpaidCents  int64                `json:"unpaidCents"`
	Created      bool                 `json:"created"`
}

type SettlementIntentType string

const (
	IntentComputeAutoStopped   SettlementIntentType = "compute_auto_stopped"
	IntentStorageHoldExhausted SettlementIntentType = "storage_hold_exhausted"
)

type SettlementIntent struct {
	Type          SettlementIntentType `json:"type"`
	AccountID     string               `json:"accountId,omitempty"`
	WorkspaceID   string               `json:"workspaceId,omitempty"`
	ComputeID     string               `json:"computeId,omitempty"`
	StorageID     string               `json:"storageId,omitempty"`
	SourceEventID string               `json:"sourceEventId,omitempty"`
	Reason        string               `json:"reason,omitempty"`
}

type SettlementEntry struct {
	ID             string        `json:"id"`
	EventType      string        `json:"eventType"`
	AccountID      string        `json:"accountId,omitempty"`
	UserID         string        `json:"userId,omitempty"`
	WorkspaceID    string        `json:"workspaceId,omitempty"`
	ComputeID      string        `json:"computeId,omitempty"`
	StorageID      string        `json:"storageId,omitempty"`
	SourceEventID  string        `json:"sourceEventId,omitempty"`
	AmountCents    int64         `json:"amountCents"`
	Currency       string        `json:"currency"`
	FundingSource  FundingSource `json:"fundingSource,omitempty"`
	BillableHours  int64         `json:"billableHours,omitempty"`
	RequestedCents int64         `json:"requestedCents,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`
}

func SettleWorkspaceUsage(input SettlementInput) (SettlementResult, error) {
	if len(input.ExistingEntries) > 0 {
		return SettlementResult{
			Wallet:  input.Wallet.Snapshot(),
			Entries: cloneSettlementEntries(input.ExistingEntries),
			Created: false,
		}, nil
	}
	if input.Hours <= 0 {
		return SettlementResult{}, errors.New("positive_hours_required")
	}
	w := input.Wallet
	if w.AccountID == "" {
		w.AccountID = input.AccountID
	}
	if w.UserID == "" {
		w.UserID = input.UserID
	}
	var entries []SettlementEntry
	var transactions []wallet.Transaction
	var intents []SettlementIntent
	var unpaid int64
	if input.ComputeActive {
		requested := input.ComputeHourlyCents * input.Hours
		result := settleCharge(&w, settleChargeInput{
			holdType:       string(HoldTypeCompute),
			eventType:      "compute_debit",
			accountID:      input.AccountID,
			userID:         w.UserID,
			workspaceID:    input.WorkspaceID,
			resourceID:     input.ComputeID,
			sourceEventID:  input.SourceEventID,
			requestedCents: requested,
			billableHours:  input.Hours,
		})
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
		requested := input.StorageHourlyCents * input.Hours
		result := settleCharge(&w, settleChargeInput{
			holdType:       string(HoldTypeStorage),
			eventType:      "storage_debit",
			accountID:      input.AccountID,
			userID:         w.UserID,
			workspaceID:    input.WorkspaceID,
			resourceID:     input.StorageID,
			sourceEventID:  input.SourceEventID,
			requestedCents: requested,
			billableHours:  input.Hours,
		})
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
	return SettlementResult{
		Wallet:       w.Snapshot(),
		Entries:      entries,
		Transactions: transactions,
		Intents:      intents,
		UnpaidCents:  unpaid,
		Created:      true,
	}, nil
}

type settleChargeInput struct {
	holdType       string
	eventType      string
	accountID      string
	userID         string
	workspaceID    string
	resourceID     string
	sourceEventID  string
	requestedCents int64
	billableHours  int64
}

type settleChargeResult struct {
	entries       []SettlementEntry
	transactions  []wallet.Transaction
	unpaidCents   int64
	exhaustedHold bool
}

func settleCharge(w *wallet.Wallet, input settleChargeInput) settleChargeResult {
	before := w.Snapshot()
	charge := w.Charge(input.holdType, input.requestedCents)
	after := w.Snapshot()
	createdAt := time.Now().UTC()
	var entries []SettlementEntry
	var transactions []wallet.Transaction
	if charge.AvailableCents > 0 {
		entry := newSettlementEntry(input, charge.AvailableCents, FundingSourceAvailableBalance, createdAt)
		entries = append(entries, entry)
		transactions = append(transactions, settlementTransaction(*w, before, after, entry, charge.AvailableCents, createdAt))
	}
	if charge.HoldCents > 0 {
		source := FundingSourceComputeHold
		if input.holdType == string(HoldTypeStorage) {
			source = FundingSourceStorageHold
		}
		entry := newSettlementEntry(input, charge.HoldCents, source, createdAt)
		entries = append(entries, entry)
		transactions = append(transactions, settlementTransaction(*w, before, after, entry, charge.HoldCents, createdAt))
	}
	return settleChargeResult{
		entries:       entries,
		transactions:  transactions,
		unpaidCents:   charge.UnpaidCents,
		exhaustedHold: charge.ExhaustedHold,
	}
}

func newSettlementEntry(input settleChargeInput, amountCents int64, source FundingSource, createdAt time.Time) SettlementEntry {
	entry := SettlementEntry{
		ID:             randomSettlementID("led"),
		EventType:      input.eventType,
		AccountID:      input.accountID,
		UserID:         input.userID,
		WorkspaceID:    input.workspaceID,
		SourceEventID:  input.sourceEventID,
		AmountCents:    -amountCents,
		Currency:       "CNY",
		FundingSource:  source,
		BillableHours:  input.billableHours,
		RequestedCents: input.requestedCents,
		CreatedAt:      createdAt,
	}
	if input.holdType == string(HoldTypeCompute) {
		entry.ComputeID = input.resourceID
	}
	if input.holdType == string(HoldTypeStorage) {
		entry.StorageID = input.resourceID
	}
	return entry
}

func settlementTransaction(w wallet.Wallet, before wallet.Snapshot, after wallet.Snapshot, entry SettlementEntry, amountCents int64, createdAt time.Time) wallet.Transaction {
	return wallet.NewTransaction(wallet.TransactionInput{
		UserID:              w.UserID,
		AccountID:           w.AccountID,
		WorkspaceID:         entry.WorkspaceID,
		Type:                wallet.TransactionDebit,
		AmountCents:         -amountCents,
		Currency:            "CNY",
		SourceEventID:       entry.SourceEventID,
		LedgerEntryID:       entry.ID,
		FundingSource:       string(entry.FundingSource),
		BalanceBeforeCents:  before.BalanceCents,
		BalanceAfterCents:   after.BalanceCents,
		FrozenBeforeCents:   before.FrozenCents,
		FrozenAfterCents:    after.FrozenCents,
		AvailableAfterCents: after.AvailableCents,
		CreatedAt:           createdAt,
	})
}

func cloneSettlementEntries(entries []SettlementEntry) []SettlementEntry {
	out := make([]SettlementEntry, len(entries))
	copy(out, entries)
	return out
}

func randomSettlementID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
