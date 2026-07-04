package billing

import (
	"errors"
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

func TestCreateHoldRequiresAvailableBalance(t *testing.T) {
	w := wallet.Wallet{UserID: "usr_1", AccountID: "acct_1", BalanceCents: 1000}

	_, err := CreateHold(HoldInput{
		Wallet:        w,
		HoldType:      HoldTypeCompute,
		AmountCents:   1200,
		SourceEventID: "compute_resource:compute_1:created",
	})

	if !errors.Is(err, wallet.ErrInsufficientAvailableBalance) {
		t.Fatalf("expected insufficient balance, got %v", err)
	}
	snapshot := w.Snapshot()
	if snapshot.BalanceCents != 1000 || snapshot.FrozenCents != 0 {
		t.Fatalf("wallet mutated on rejection: %+v", snapshot)
	}
}

func TestCreateComputeHoldWritesLedgerEntryAndWalletTransaction(t *testing.T) {
	w := wallet.Wallet{UserID: "usr_1", AccountID: "acct_1", BalanceCents: 1000}

	result, err := CreateHold(HoldInput{
		Wallet:        w,
		HoldType:      HoldTypeCompute,
		AmountCents:   600,
		SourceEventID: "compute_resource:compute_1:created",
		ResourceID:    "compute_1",
	})
	if err != nil {
		t.Fatalf("create hold: %v", err)
	}

	if result.Wallet.FrozenCents != 600 || result.Wallet.AvailableCents != 400 {
		t.Fatalf("wallet snapshot = %+v", result.Wallet)
	}
	if result.Entry.EventType != "compute_hold" || result.Entry.AmountCents != 600 || result.Entry.ComputeID != "compute_1" {
		t.Fatalf("ledger entry = %+v", result.Entry)
	}
	if result.Transaction.Type != wallet.TransactionHold || result.Transaction.AmountCents != 600 || result.Transaction.LedgerEntryID != result.Entry.ID {
		t.Fatalf("wallet transaction = %+v", result.Transaction)
	}
	if result.Transaction.BalanceBeforeCents != 1000 || result.Transaction.BalanceAfterCents != 1000 {
		t.Fatalf("transaction balance movement = %+v", result.Transaction)
	}
	if result.Transaction.FrozenBeforeCents != 0 || result.Transaction.FrozenAfterCents != 600 {
		t.Fatalf("transaction frozen movement = %+v", result.Transaction)
	}
}

func TestCreateStorageHoldWritesLedgerEntryAndWalletTransaction(t *testing.T) {
	w := wallet.Wallet{UserID: "usr_1", AccountID: "acct_1", BalanceCents: 1000}

	result, err := CreateHold(HoldInput{
		Wallet:        w,
		HoldType:      HoldTypeStorage,
		AmountCents:   101,
		SourceEventID: "storage_volume:storage_1:created",
		ResourceID:    "storage_1",
	})
	if err != nil {
		t.Fatalf("create hold: %v", err)
	}
	if result.Entry.EventType != "storage_hold" || result.Entry.StorageID != "storage_1" {
		t.Fatalf("ledger entry = %+v", result.Entry)
	}
	if result.Transaction.Type != wallet.TransactionHold || result.Transaction.AmountCents != 101 {
		t.Fatalf("wallet transaction = %+v", result.Transaction)
	}
}
