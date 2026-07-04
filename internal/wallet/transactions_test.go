package wallet

import "testing"

func TestNewTransactionRecordsBalanceAndFrozenMovement(t *testing.T) {
	tx := NewTransaction(TransactionInput{
		UserID:              "usr_1",
		AccountID:           "acct_1",
		WorkspaceID:         "account",
		Type:                TransactionCredit,
		AmountCents:         25000,
		Currency:            "CNY",
		SourceEventID:       "owner_credit",
		LedgerEntryID:       "led_1",
		BalanceBeforeCents:  0,
		BalanceAfterCents:   25000,
		FrozenBeforeCents:   0,
		FrozenAfterCents:    0,
		AvailableAfterCents: 25000,
	})

	if tx.ID == "" {
		t.Fatalf("expected generated transaction id")
	}
	if tx.Type != TransactionCredit {
		t.Fatalf("transaction type = %q", tx.Type)
	}
	if tx.BalanceBeforeCents != 0 || tx.BalanceAfterCents != 25000 {
		t.Fatalf("balance movement = %d -> %d", tx.BalanceBeforeCents, tx.BalanceAfterCents)
	}
	if tx.FrozenBeforeCents != 0 || tx.FrozenAfterCents != 0 {
		t.Fatalf("frozen movement = %d -> %d", tx.FrozenBeforeCents, tx.FrozenAfterCents)
	}
	if tx.AvailableAfterCents != 25000 {
		t.Fatalf("available after = %d", tx.AvailableAfterCents)
	}
	if tx.LedgerEntryID != "led_1" {
		t.Fatalf("ledger entry id = %q", tx.LedgerEntryID)
	}
}

func TestTransactionTypeSetMatchesBillingLoop(t *testing.T) {
	valid := []TransactionType{
		TransactionCredit,
		TransactionHold,
		TransactionDebit,
		TransactionHoldRelease,
		TransactionAdjustment,
	}
	for _, kind := range valid {
		if !IsValidTransactionType(kind) {
			t.Fatalf("expected %q to be valid", kind)
		}
	}
	if IsValidTransactionType(TransactionType("refund")) {
		t.Fatalf("unexpected refund transaction type")
	}
}
