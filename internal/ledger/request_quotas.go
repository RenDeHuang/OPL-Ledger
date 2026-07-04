package ledger

import "errors"

func validateRequestQuotaInput(input RequestQuotaInput) error {
	if input.AccountID == "" {
		return errors.New("account_required")
	}
	if input.UserID == "" {
		return errors.New("user_required")
	}
	if input.WorkspaceID == "" {
		return errors.New("workspace_required")
	}
	if input.Quota.Limit != nil && *input.Quota.Limit < 0 {
		return errors.New("non_negative_limit_required")
	}
	if input.Quota.WindowLimit != nil && *input.Quota.WindowLimit < 0 {
		return errors.New("non_negative_window_limit_required")
	}
	if input.Quota.Used < 0 || input.Quota.WindowUsed < 0 || input.Quota.WindowSeconds < 0 {
		return errors.New("non_negative_quota_usage_required")
	}
	return nil
}

func requestQuotaKey(accountID string, userID string, workspaceID string) string {
	return accountID + "\x00" + userID + "\x00" + workspaceID
}

func matchesRequestQuota(record RequestQuotaRecord, filter RequestQuotaFilter) bool {
	if filter.AccountID != "" && record.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && record.UserID != filter.UserID {
		return false
	}
	if filter.WorkspaceID != "" && record.WorkspaceID != filter.WorkspaceID {
		return false
	}
	return true
}
