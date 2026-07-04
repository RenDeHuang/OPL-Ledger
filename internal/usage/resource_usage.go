package usage

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type ResourceKind string

const (
	ResourceKindCompute ResourceKind = "compute"
	ResourceKindStorage ResourceKind = "storage"
)

type ResourceUsageInput struct {
	AccountID      string
	UserID         string
	WorkspaceID    string
	ComputeID      string
	StorageID      string
	AttachmentID   string
	ResourceKind   ResourceKind
	Quantity       int64
	Unit           string
	UnitPriceCents int64
	AmountCents    int64
	RequestedCents int64
	SourceEventID  string
	Metadata       map[string]any
	CreatedAt      time.Time
}

type ResourceUsageLog struct {
	ID             string         `json:"id"`
	AccountID      string         `json:"accountId,omitempty"`
	UserID         string         `json:"userId,omitempty"`
	WorkspaceID    string         `json:"workspaceId,omitempty"`
	ComputeID      string         `json:"computeId,omitempty"`
	StorageID      string         `json:"storageId,omitempty"`
	AttachmentID   string         `json:"attachmentId,omitempty"`
	ResourceKind   ResourceKind   `json:"resourceKind"`
	Quantity       int64          `json:"quantity"`
	Unit           string         `json:"unit"`
	UnitPriceCents int64          `json:"unitPriceCents"`
	AmountCents    int64          `json:"amountCents"`
	RequestedCents int64          `json:"requestedCents,omitempty"`
	Currency       string         `json:"currency"`
	SourceEventID  string         `json:"sourceEventId,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
}

func NewResourceUsageLog(input ResourceUsageInput) ResourceUsageLog {
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	requested := input.RequestedCents
	if requested == 0 {
		requested = input.AmountCents
	}
	return ResourceUsageLog{
		ID:             randomUsageID("usage_res"),
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
		RequestedCents: requested,
		Currency:       "CNY",
		SourceEventID:  input.SourceEventID,
		Metadata:       cloneUsageMetadata(input.Metadata),
		CreatedAt:      createdAt,
	}
}

func cloneUsageMetadata(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func randomUsageID(prefix string) string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}
