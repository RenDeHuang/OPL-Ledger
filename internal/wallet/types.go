package wallet

type Wallet struct {
	UserID              string
	AccountID           string
	BalanceCents        int64
	Holds               map[string]int64
	TotalRechargedCents int64
}

type Snapshot struct {
	UserID              string           `json:"userId"`
	AccountID           string           `json:"accountId"`
	BalanceCents        int64            `json:"balanceCents"`
	FrozenCents         int64            `json:"frozenCents"`
	AvailableCents      int64            `json:"availableCents"`
	Holds               map[string]int64 `json:"holds"`
	TotalRechargedCents int64            `json:"totalRechargedCents"`
}

type ChargeResult struct {
	RequestedCents int64
	AvailableCents int64
	HoldCents      int64
	ChargedCents   int64
	UnpaidCents    int64
	UsedHold       bool
	ExhaustedHold  bool
}
