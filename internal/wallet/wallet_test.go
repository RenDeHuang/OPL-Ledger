package wallet

import "testing"

func TestWalletSnapshotComputesAvailableFromBalanceAndFrozenHolds(t *testing.T) {
	w := Wallet{
		UserID:              "usr_1",
		AccountID:           "acct_1",
		BalanceCents:        25000,
		Holds:               map[string]int64{"compute": 20160, "storage": 56},
		TotalRechargedCents: 25000,
	}

	snapshot := w.Snapshot()

	if snapshot.BalanceCents != 25000 {
		t.Fatalf("balance cents = %d", snapshot.BalanceCents)
	}
	if snapshot.FrozenCents != 20216 {
		t.Fatalf("frozen cents = %d", snapshot.FrozenCents)
	}
	if snapshot.AvailableCents != 4784 {
		t.Fatalf("available cents = %d", snapshot.AvailableCents)
	}
	if snapshot.Holds["compute"] != 20160 || snapshot.Holds["storage"] != 56 {
		t.Fatalf("holds = %+v", snapshot.Holds)
	}
}

func TestWalletCreditIncreasesBalanceAndTotalRecharged(t *testing.T) {
	w := Wallet{UserID: "usr_1", AccountID: "acct_1"}

	w.Credit(25000)

	snapshot := w.Snapshot()
	if snapshot.BalanceCents != 25000 {
		t.Fatalf("balance cents = %d", snapshot.BalanceCents)
	}
	if snapshot.TotalRechargedCents != 25000 {
		t.Fatalf("total recharged cents = %d", snapshot.TotalRechargedCents)
	}
}

func TestWalletHoldRequiresAvailableBalance(t *testing.T) {
	w := Wallet{UserID: "usr_1", AccountID: "acct_1", BalanceCents: 1000}

	if err := w.AddHold("compute", 1200); err == nil {
		t.Fatalf("expected insufficient balance error")
	}
	if err := w.AddHold("compute", 600); err != nil {
		t.Fatalf("add hold: %v", err)
	}

	snapshot := w.Snapshot()
	if snapshot.FrozenCents != 600 {
		t.Fatalf("frozen cents = %d", snapshot.FrozenCents)
	}
	if snapshot.AvailableCents != 400 {
		t.Fatalf("available cents = %d", snapshot.AvailableCents)
	}
}

func TestWalletChargeUsesAvailableBalanceBeforeHold(t *testing.T) {
	w := Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700},
	}

	charge := w.Charge("compute", 500)

	if charge.AvailableCents != 300 {
		t.Fatalf("available charged = %d", charge.AvailableCents)
	}
	if charge.HoldCents != 200 {
		t.Fatalf("hold charged = %d", charge.HoldCents)
	}
	if charge.UnpaidCents != 0 {
		t.Fatalf("unpaid = %d", charge.UnpaidCents)
	}
	snapshot := w.Snapshot()
	if snapshot.BalanceCents != 500 {
		t.Fatalf("balance cents = %d", snapshot.BalanceCents)
	}
	if snapshot.Holds["compute"] != 500 {
		t.Fatalf("compute hold = %d", snapshot.Holds["compute"])
	}
	if snapshot.FrozenCents != 500 {
		t.Fatalf("frozen cents = %d", snapshot.FrozenCents)
	}
	if snapshot.AvailableCents != 0 {
		t.Fatalf("available cents = %d", snapshot.AvailableCents)
	}
}

func TestWalletChargeNeverDebitsBeyondBalanceAndHold(t *testing.T) {
	w := Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700},
	}

	charge := w.Charge("compute", 2000)

	if charge.ChargedCents != 1000 {
		t.Fatalf("charged cents = %d", charge.ChargedCents)
	}
	if charge.UnpaidCents != 1000 {
		t.Fatalf("unpaid cents = %d", charge.UnpaidCents)
	}
	snapshot := w.Snapshot()
	if snapshot.BalanceCents != 0 {
		t.Fatalf("balance cents = %d", snapshot.BalanceCents)
	}
	if snapshot.FrozenCents != 0 {
		t.Fatalf("frozen cents = %d", snapshot.FrozenCents)
	}
	if snapshot.Holds["compute"] != 0 {
		t.Fatalf("compute hold = %d", snapshot.Holds["compute"])
	}
}

func TestWalletReleaseHoldMovesFrozenBackToAvailable(t *testing.T) {
	w := Wallet{
		UserID:       "usr_1",
		AccountID:    "acct_1",
		BalanceCents: 1000,
		Holds:        map[string]int64{"compute": 700},
	}

	released := w.ReleaseHold("compute", 300)

	if released != 300 {
		t.Fatalf("released cents = %d", released)
	}
	snapshot := w.Snapshot()
	if snapshot.Holds["compute"] != 400 {
		t.Fatalf("compute hold = %d", snapshot.Holds["compute"])
	}
	if snapshot.FrozenCents != 400 {
		t.Fatalf("frozen cents = %d", snapshot.FrozenCents)
	}
	if snapshot.AvailableCents != 600 {
		t.Fatalf("available cents = %d", snapshot.AvailableCents)
	}
}
