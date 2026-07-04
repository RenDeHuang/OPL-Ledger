package ledger

import "github.com/RenDeHuang/OPL-Ledger/internal/wallet"

func matchesWallet(w wallet.Wallet, filter WalletFilter) bool {
	if filter.AccountID != "" && w.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && w.UserID != filter.UserID {
		return false
	}
	return true
}
