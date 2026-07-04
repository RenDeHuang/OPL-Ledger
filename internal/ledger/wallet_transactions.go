package ledger

import "github.com/RenDeHuang/OPL-Ledger/internal/wallet"

func matchesWalletTransaction(transaction wallet.Transaction, filter WalletTransactionFilter) bool {
	if filter.AccountID != "" && transaction.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && transaction.UserID != filter.UserID {
		return false
	}
	if filter.WorkspaceID != "" && transaction.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.Type != "" && transaction.Type != filter.Type {
		return false
	}
	if filter.SourceEventID != "" && transaction.SourceEventID != filter.SourceEventID {
		return false
	}
	if filter.LedgerEntryID != "" && transaction.LedgerEntryID != filter.LedgerEntryID {
		return false
	}
	if filter.UsageLogID != "" && transaction.UsageLogID != filter.UsageLogID {
		return false
	}
	if filter.FundingSource != "" && transaction.FundingSource != filter.FundingSource {
		return false
	}
	return true
}

func cloneWalletTransaction(transaction wallet.Transaction) wallet.Transaction {
	out := transaction
	if transaction.Metadata != nil {
		out.Metadata = make(map[string]any, len(transaction.Metadata))
		for key, value := range transaction.Metadata {
			out.Metadata[key] = value
		}
	}
	return out
}
