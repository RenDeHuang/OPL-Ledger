package wallet

import "errors"

var ErrInsufficientAvailableBalance = errors.New("insufficient_available_balance")

func (w *Wallet) Snapshot() Snapshot {
	w.ensureHolds()
	frozen := w.FrozenCents()
	return Snapshot{
		UserID:              w.UserID,
		AccountID:           w.AccountID,
		BalanceCents:        w.BalanceCents,
		FrozenCents:         frozen,
		AvailableCents:      max(0, w.BalanceCents-frozen),
		Holds:               cloneHolds(w.Holds),
		TotalRechargedCents: w.TotalRechargedCents,
	}
}

func (w *Wallet) Credit(amountCents int64) {
	amount := max(0, amountCents)
	w.BalanceCents += amount
	w.TotalRechargedCents += amount
}

func (w *Wallet) FrozenCents() int64 {
	w.ensureHolds()
	var frozen int64
	for key, amount := range w.Holds {
		if amount < 0 {
			amount = 0
			w.Holds[key] = 0
		}
		frozen += amount
	}
	return min(frozen, max(0, w.BalanceCents))
}

func (w *Wallet) AvailableCents() int64 {
	return max(0, w.BalanceCents-w.FrozenCents())
}

func (w *Wallet) AddHold(holdType string, amountCents int64) error {
	amount := max(0, amountCents)
	if amount == 0 {
		return nil
	}
	if w.AvailableCents() < amount {
		return ErrInsufficientAvailableBalance
	}
	w.ensureHolds()
	w.Holds[holdType] += amount
	return nil
}

func (w *Wallet) ReleaseHold(holdType string, amountCents int64) int64 {
	w.ensureHolds()
	current := max(0, w.Holds[holdType])
	released := min(current, max(0, amountCents))
	w.Holds[holdType] = current - released
	return released
}

func (w *Wallet) Charge(holdType string, amountCents int64) ChargeResult {
	requested := max(0, amountCents)
	available := w.debitAvailable(requested)
	remaining := requested - available
	hold := w.captureHold(holdType, remaining)
	charged := available + hold
	return ChargeResult{
		RequestedCents: requested,
		AvailableCents: available,
		HoldCents:      hold,
		ChargedCents:   charged,
		UnpaidCents:    requested - charged,
		UsedHold:       hold > 0,
		ExhaustedHold:  hold > 0 && w.HoldCents(holdType) == 0,
	}
}

func (w *Wallet) HoldCents(holdType string) int64 {
	w.ensureHolds()
	return max(0, w.Holds[holdType])
}

func (w *Wallet) debitAvailable(amountCents int64) int64 {
	debit := min(w.AvailableCents(), max(0, amountCents))
	w.BalanceCents -= debit
	return debit
}

func (w *Wallet) captureHold(holdType string, amountCents int64) int64 {
	w.ensureHolds()
	current := max(0, w.Holds[holdType])
	captured := min(current, max(0, amountCents))
	if captured == 0 {
		return 0
	}
	w.Holds[holdType] = current - captured
	w.BalanceCents -= captured
	if w.BalanceCents < 0 {
		w.BalanceCents = 0
	}
	return captured
}

func (w *Wallet) ensureHolds() {
	if w.Holds == nil {
		w.Holds = map[string]int64{}
	}
}

func cloneHolds(holds map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(holds))
	for key, amount := range holds {
		out[key] = max(0, amount)
	}
	return out
}

func max(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func min(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
