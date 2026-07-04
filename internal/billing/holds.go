package billing

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

type HoldType string

const (
	HoldTypeCompute HoldType = "compute"
	HoldTypeStorage HoldType = "storage"
)

type HoldInput struct {
	Wallet        wallet.Wallet
	HoldType      HoldType
	AmountCents   int64
	SourceEventID string
	ResourceID    string
	WorkspaceID   string
	PackageID     string
	Metadata      map[string]any
}

type HoldResult struct {
	Wallet      wallet.Snapshot    `json:"wallet"`
	Entry       ledger.Entry       `json:"entry"`
	Transaction wallet.Transaction `json:"transaction"`
}

type ReleaseHoldInput struct {
	Wallet        wallet.Wallet
	HoldTypes     []HoldType
	SourceEventID string
	ComputeID     string
	StorageID     string
	WorkspaceID   string
	Reason        string
}

type ReleaseHoldResult struct {
	Wallet       wallet.Snapshot      `json:"wallet"`
	Entries      []ledger.Entry       `json:"entries"`
	Transactions []wallet.Transaction `json:"transactions"`
}

func CreateHold(input HoldInput) (HoldResult, error) {
	w := input.Wallet
	before := w.Snapshot()
	if err := w.AddHold(string(input.HoldType), input.AmountCents); err != nil {
		return HoldResult{}, err
	}
	after := w.Snapshot()
	createdAt := time.Now().UTC()
	entry := ledger.Entry{
		ID:            randomBillingID("led"),
		EventType:     string(input.HoldType) + "_hold",
		AccountID:     w.AccountID,
		UserID:        w.UserID,
		WorkspaceID:   workspaceOrResource(input.WorkspaceID),
		SourceEventID: input.SourceEventID,
		AmountCents:   input.AmountCents,
		Currency:      "CNY",
		CreatedAt:     createdAt,
	}
	if input.HoldType == HoldTypeCompute {
		entry.ComputeID = input.ResourceID
	}
	if input.HoldType == HoldTypeStorage {
		entry.StorageID = input.ResourceID
	}
	transaction := wallet.NewTransaction(wallet.TransactionInput{
		UserID:              w.UserID,
		AccountID:           w.AccountID,
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
		Metadata:            input.Metadata,
		CreatedAt:           createdAt,
	})
	return HoldResult{
		Wallet:      after,
		Entry:       entry,
		Transaction: transaction,
	}, nil
}

func ReleaseHolds(input ReleaseHoldInput) ReleaseHoldResult {
	w := input.Wallet
	var entries []ledger.Entry
	var transactions []wallet.Transaction
	for _, holdType := range input.HoldTypes {
		before := w.Snapshot()
		released := w.ReleaseHold(string(holdType), before.Holds[string(holdType)])
		if released <= 0 {
			continue
		}
		after := w.Snapshot()
		createdAt := time.Now().UTC()
		entry := ledger.Entry{
			ID:            randomBillingID("led"),
			EventType:     string(holdType) + "_hold_released",
			AccountID:     w.AccountID,
			UserID:        w.UserID,
			WorkspaceID:   workspaceOrResource(input.WorkspaceID),
			SourceEventID: input.SourceEventID,
			AmountCents:   -released,
			Currency:      "CNY",
			CreatedAt:     createdAt,
		}
		if holdType == HoldTypeCompute {
			entry.ComputeID = input.ComputeID
		}
		if holdType == HoldTypeStorage {
			entry.StorageID = input.StorageID
		}
		transaction := wallet.NewTransaction(wallet.TransactionInput{
			UserID:              w.UserID,
			AccountID:           w.AccountID,
			WorkspaceID:         entry.WorkspaceID,
			Type:                wallet.TransactionHoldRelease,
			AmountCents:         -released,
			Currency:            "CNY",
			SourceEventID:       input.SourceEventID,
			LedgerEntryID:       entry.ID,
			BalanceBeforeCents:  before.BalanceCents,
			BalanceAfterCents:   after.BalanceCents,
			FrozenBeforeCents:   before.FrozenCents,
			FrozenAfterCents:    after.FrozenCents,
			AvailableAfterCents: after.AvailableCents,
			Metadata: map[string]any{
				"reason":   input.Reason,
				"holdType": string(holdType),
			},
			CreatedAt: createdAt,
		})
		entries = append(entries, entry)
		transactions = append(transactions, transaction)
	}
	return ReleaseHoldResult{
		Wallet:       w.Snapshot(),
		Entries:      entries,
		Transactions: transactions,
	}
}

func workspaceOrResource(workspaceID string) string {
	if workspaceID == "" {
		return "resource"
	}
	return workspaceID
}

func randomBillingID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
