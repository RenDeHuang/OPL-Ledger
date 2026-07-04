package ledger

func matchesRequestUsageLog(log RequestUsageLog, filter RequestUsageFilter) bool {
	if filter.AccountID != "" && log.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && log.UserID != filter.UserID {
		return false
	}
	if filter.WorkspaceID != "" && log.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.RequestID != "" && log.RequestID != filter.RequestID {
		return false
	}
	if filter.SourceEventID != "" && log.SourceEventID != filter.SourceEventID {
		return false
	}
	if filter.RequestFingerprint != "" && log.RequestFingerprint != filter.RequestFingerprint {
		return false
	}
	if filter.LedgerEntryID != "" && log.LedgerEntryID != filter.LedgerEntryID {
		return false
	}
	if filter.Provider != "" && log.Provider != filter.Provider {
		return false
	}
	if filter.Model != "" && log.Model != filter.Model {
		return false
	}
	return true
}

func cloneRequestUsageLog(log RequestUsageLog) RequestUsageLog {
	out := log
	if log.Quota != nil {
		quota := *log.Quota
		out.Quota = &quota
	}
	return out
}
