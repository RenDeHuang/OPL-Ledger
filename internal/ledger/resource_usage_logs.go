package ledger

import "github.com/RenDeHuang/OPL-Ledger/internal/usage"

func matchesResourceUsageLog(log usage.ResourceUsageLog, filter ResourceUsageFilter) bool {
	if filter.AccountID != "" && log.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && log.UserID != filter.UserID {
		return false
	}
	if filter.WorkspaceID != "" && log.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.ComputeID != "" && log.ComputeID != filter.ComputeID {
		return false
	}
	if filter.StorageID != "" && log.StorageID != filter.StorageID {
		return false
	}
	if filter.AttachmentID != "" && log.AttachmentID != filter.AttachmentID {
		return false
	}
	if filter.ResourceKind != "" && log.ResourceKind != filter.ResourceKind {
		return false
	}
	if filter.SourceEventID != "" && log.SourceEventID != filter.SourceEventID {
		return false
	}
	return true
}

func cloneResourceUsageLog(log usage.ResourceUsageLog) usage.ResourceUsageLog {
	out := log
	if log.Metadata != nil {
		out.Metadata = make(map[string]any, len(log.Metadata))
		for key, value := range log.Metadata {
			out.Metadata[key] = value
		}
	}
	return out
}
