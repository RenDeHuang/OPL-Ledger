package ledger

import "errors"

func validateHoldInput(input HoldInput) error {
	if input.AccountID == "" {
		return errors.New("account_required")
	}
	if input.HoldType != "compute" && input.HoldType != "storage" {
		return errors.New("supported_hold_type_required")
	}
	if input.AmountCents <= 0 {
		return errors.New("positive_hold_required")
	}
	if input.SourceEventID == "" {
		return errors.New("source_event_required")
	}
	return nil
}

func validateReleaseHoldInput(input ReleaseHoldInput) error {
	if input.AccountID == "" {
		return errors.New("account_required")
	}
	if input.SourceEventID == "" {
		return errors.New("source_event_required")
	}
	if len(input.HoldTypes) == 0 {
		return errors.New("hold_types_required")
	}
	seen := map[string]bool{}
	for _, holdType := range input.HoldTypes {
		if holdType != "compute" && holdType != "storage" {
			return errors.New("supported_hold_type_required")
		}
		if seen[holdType] {
			return errors.New("duplicate_hold_type")
		}
		seen[holdType] = true
	}
	return nil
}

func holdAppendInput(input HoldInput) AppendEntryInput {
	entry := AppendEntryInput{
		EventType:     input.HoldType + "_hold",
		AccountID:     input.AccountID,
		UserID:        input.UserID,
		WorkspaceID:   workspaceOrResource(input.WorkspaceID),
		SourceEventID: input.SourceEventID,
		AmountCents:   input.AmountCents,
		Currency:      "CNY",
	}
	if input.HoldType == "compute" {
		entry.ComputeID = input.ResourceID
	}
	if input.HoldType == "storage" {
		entry.StorageID = input.ResourceID
	}
	return entry
}

func workspaceOrResource(workspaceID string) string {
	if workspaceID == "" {
		return "resource"
	}
	return workspaceID
}

func holdReleaseSourceEventID(sourceEventID string, holdType string, multi bool) string {
	if !multi {
		return sourceEventID
	}
	return sourceEventID + ":" + holdType
}

func cloneMetadataMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}
