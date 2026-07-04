package billing

import (
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
)

func TestSettleWorkspaceUsageWritesComputeAndStorageDebits(t *testing.T) {
	w := wallet.Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 10000,
		Holds:        map[string]int64{"compute": 7862, "storage": 101},
	}

	result, err := SettleWorkspaceUsage(SettlementInput{
		Wallet:             w,
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		ComputeID:          "compute_1",
		StorageID:          "storage_1",
		SourceEventID:      "billing_tick_1",
		Hours:              1,
		ComputeActive:      true,
		StorageActive:      true,
		ComputeHourlyCents: 47,
		StorageHourlyCents: 1,
	})
	if err != nil {
		t.Fatalf("settle workspace usage: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("entries = %+v", result.Entries)
	}
	if result.Entries[0].EventType != "compute_debit" || result.Entries[0].AmountCents != -47 || result.Entries[0].ComputeID != "compute_1" {
		t.Fatalf("compute entry = %+v", result.Entries[0])
	}
	if result.Entries[1].EventType != "storage_debit" || result.Entries[1].AmountCents != -1 || result.Entries[1].StorageID != "storage_1" {
		t.Fatalf("storage entry = %+v", result.Entries[1])
	}
	if len(result.Transactions) != 2 {
		t.Fatalf("transactions = %+v", result.Transactions)
	}
	if result.Wallet.BalanceCents != 9952 || result.Wallet.AvailableCents != 1989 {
		t.Fatalf("wallet = %+v", result.Wallet)
	}
}

func TestSettleWorkspaceUsageChargesAvailableBeforeHold(t *testing.T) {
	w := wallet.Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700},
	}

	result, err := SettleWorkspaceUsage(SettlementInput{
		Wallet:             w,
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		ComputeID:          "compute_1",
		SourceEventID:      "billing_tick_1",
		Hours:              1,
		ComputeActive:      true,
		ComputeHourlyCents: 500,
	})
	if err != nil {
		t.Fatalf("settle workspace usage: %v", err)
	}

	if len(result.Entries) != 2 {
		t.Fatalf("entries = %+v", result.Entries)
	}
	if result.Entries[0].AmountCents != -300 || result.Entries[0].FundingSource != FundingSourceAvailableBalance {
		t.Fatalf("available entry = %+v", result.Entries[0])
	}
	if result.Entries[1].AmountCents != -200 || result.Entries[1].FundingSource != FundingSourceComputeHold {
		t.Fatalf("hold entry = %+v", result.Entries[1])
	}
	if result.Wallet.BalanceCents != 500 || result.Wallet.FrozenCents != 500 || result.Wallet.AvailableCents != 0 {
		t.Fatalf("wallet = %+v", result.Wallet)
	}
}

func TestSettleWorkspaceUsageNeverDebitsBelowBalance(t *testing.T) {
	w := wallet.Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700},
	}

	result, err := SettleWorkspaceUsage(SettlementInput{
		Wallet:             w,
		AccountID:          "acct_1",
		UserID:             "usr_1",
		WorkspaceID:        "ws_1",
		ComputeID:          "compute_1",
		SourceEventID:      "billing_tick_1",
		Hours:              1,
		ComputeActive:      true,
		ComputeHourlyCents: 2000,
	})
	if err != nil {
		t.Fatalf("settle workspace usage: %v", err)
	}

	if result.Wallet.BalanceCents != 0 || result.Wallet.FrozenCents != 0 || result.Wallet.AvailableCents != 0 {
		t.Fatalf("wallet = %+v", result.Wallet)
	}
	if result.UnpaidCents != 1000 {
		t.Fatalf("unpaid cents = %d", result.UnpaidCents)
	}
}

func TestSettleWorkspaceUsageReturnsExistingEntriesForReplay(t *testing.T) {
	existing := []SettlementEntry{{
		ID:            "led_1",
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		SourceEventID: "billing_tick_1",
		AmountCents:   -47,
	}}

	result, err := SettleWorkspaceUsage(SettlementInput{
		Wallet:          wallet.Wallet{UserID: "usr_1", AccountID: "acct_1", BalanceCents: 10000},
		SourceEventID:   "billing_tick_1",
		ExistingEntries: existing,
	})
	if err != nil {
		t.Fatalf("settle replay: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != "led_1" {
		t.Fatalf("replay entries = %+v", result.Entries)
	}
	if result.Created {
		t.Fatalf("expected replay result")
	}
}
