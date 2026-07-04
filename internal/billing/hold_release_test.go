package billing

import (
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

func TestReleaseComputeHoldForStopCompute(t *testing.T) {
	w := wallet.Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700, "storage": 100},
	}

	result := ReleaseHolds(ReleaseHoldInput{
		Wallet:        w,
		HoldTypes:     []HoldType{HoldTypeCompute},
		SourceEventID: "compute_resource:compute_1:stopped",
		ComputeID:     "compute_1",
		WorkspaceID:   "resource",
		Reason:        "stop_compute",
	})

	if result.Wallet.FrozenCents != 100 || result.Wallet.AvailableCents != 900 {
		t.Fatalf("wallet = %+v", result.Wallet)
	}
	if len(result.Entries) != 1 || result.Entries[0].EventType != "compute_hold_released" || result.Entries[0].AmountCents != -700 {
		t.Fatalf("entries = %+v", result.Entries)
	}
	if len(result.Transactions) != 1 || result.Transactions[0].Type != wallet.TransactionHoldRelease || result.Transactions[0].AmountCents != -700 {
		t.Fatalf("transactions = %+v", result.Transactions)
	}
}

func TestReleaseComputeAndStorageHoldsForDestroyStorageWhenApplicable(t *testing.T) {
	w := wallet.Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700, "storage": 100},
	}

	result := ReleaseHolds(ReleaseHoldInput{
		Wallet:        w,
		HoldTypes:     []HoldType{HoldTypeCompute, HoldTypeStorage},
		SourceEventID: "storage_volume:storage_1:destroyed",
		ComputeID:     "compute_1",
		StorageID:     "storage_1",
		WorkspaceID:   "resource",
		Reason:        "destroy_storage",
	})

	if result.Wallet.FrozenCents != 0 || result.Wallet.AvailableCents != 1000 {
		t.Fatalf("wallet = %+v", result.Wallet)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries = %+v", result.Entries)
	}
	if result.Entries[0].EventType != "compute_hold_released" || result.Entries[1].EventType != "storage_hold_released" {
		t.Fatalf("entries = %+v", result.Entries)
	}
}

func TestReleaseHoldForCreateFailure(t *testing.T) {
	w := wallet.Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"storage": 100},
	}

	result := ReleaseHolds(ReleaseHoldInput{
		Wallet:        w,
		HoldTypes:     []HoldType{HoldTypeStorage},
		SourceEventID: "storage_volume:storage_1:create_failed",
		StorageID:     "storage_1",
		WorkspaceID:   "resource",
		Reason:        "create_failure",
	})

	if result.Wallet.FrozenCents != 0 || result.Wallet.AvailableCents != 1000 {
		t.Fatalf("wallet = %+v", result.Wallet)
	}
	if len(result.Entries) != 1 || result.Entries[0].EventType != "storage_hold_released" {
		t.Fatalf("entries = %+v", result.Entries)
	}
}
