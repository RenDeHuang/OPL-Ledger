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
