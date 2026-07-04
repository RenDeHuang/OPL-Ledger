package ledger

import (
	"errors"

	"github.com/RenDeHuang/OPL-Ledger/internal/usage"
)

func validateResourceUsageInput(input ResourceUsageInput) error {
	if input.WorkspaceID == "" {
		return errors.New("workspace_required")
	}
	if input.SourceEventID == "" {
		return errors.New("source_event_required")
	}
	if input.ResourceKind != usage.ResourceKindCompute && input.ResourceKind != usage.ResourceKindStorage {
		return errors.New("supported_resource_kind_required")
	}
	if input.Quantity <= 0 {
		return errors.New("positive_quantity_required")
	}
	if input.Unit == "" {
		return errors.New("unit_required")
	}
	if input.UnitPriceCents < 0 || input.AmountCents < 0 || input.RequestedCents < 0 {
		return errors.New("non_negative_amount_required")
	}
	if input.ResourceKind == usage.ResourceKindCompute && input.ComputeID == "" {
		return errors.New("compute_required")
	}
	if input.ResourceKind == usage.ResourceKindStorage && input.StorageID == "" {
		return errors.New("storage_required")
	}
	return nil
}

func toUsageResourceInput(input ResourceUsageInput) usage.ResourceUsageInput {
	return usage.ResourceUsageInput{
		AccountID:      input.AccountID,
		UserID:         input.UserID,
		WorkspaceID:    input.WorkspaceID,
		ComputeID:      input.ComputeID,
		StorageID:      input.StorageID,
		AttachmentID:   input.AttachmentID,
		ResourceKind:   input.ResourceKind,
		Quantity:       input.Quantity,
		Unit:           input.Unit,
		UnitPriceCents: input.UnitPriceCents,
		AmountCents:    input.AmountCents,
		RequestedCents: input.RequestedCents,
		SourceEventID:  input.SourceEventID,
		Metadata:       input.Metadata,
	}
}

func sameResourceUsageReplay(existing usage.ResourceUsageLog, input ResourceUsageInput) bool {
	requested := input.RequestedCents
	if requested == 0 {
		requested = input.AmountCents
	}
	return existing.AccountID == input.AccountID &&
		existing.UserID == input.UserID &&
		existing.WorkspaceID == input.WorkspaceID &&
		existing.ComputeID == input.ComputeID &&
		existing.StorageID == input.StorageID &&
		existing.AttachmentID == input.AttachmentID &&
		existing.ResourceKind == input.ResourceKind &&
		existing.Quantity == input.Quantity &&
		existing.Unit == input.Unit &&
		existing.UnitPriceCents == input.UnitPriceCents &&
		existing.AmountCents == input.AmountCents &&
		existing.RequestedCents == requested &&
		existing.SourceEventID == input.SourceEventID
}
