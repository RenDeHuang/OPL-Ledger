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
