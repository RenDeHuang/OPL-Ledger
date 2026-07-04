package ledger

const defaultManualTopUpSourceEventID = "owner_credit"

func manualTopUpSourceEventID(input ManualTopUpInput) string {
	if input.SourceEventID != "" {
		return input.SourceEventID
	}
	if input.Reason != "" {
		return input.Reason
	}
	return defaultManualTopUpSourceEventID
}

func manualTopUpReason(input ManualTopUpInput, sourceEventID string) string {
	if input.Reason != "" {
		return input.Reason
	}
	return sourceEventID
}

func matchesManualTopUp(topup ManualTopUp, filter ManualTopUpFilter) bool {
	if filter.AccountID != "" && topup.TargetAccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && topup.TargetUserID != filter.UserID {
		return false
	}
	if filter.OperatorUserID != "" && topup.OperatorUserID != filter.OperatorUserID {
		return false
	}
	if filter.OperatorAccountID != "" && topup.OperatorAccountID != filter.OperatorAccountID {
		return false
	}
	if filter.SourceEventID != "" && topup.SourceEventID != filter.SourceEventID {
		return false
	}
	if filter.Status != "" && topup.Status != filter.Status {
		return false
	}
	return true
}
