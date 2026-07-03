package ledger

import (
	"errors"
	"time"
)

var ErrIdempotencyConflict = errors.New("idempotency keys resolve to different ledger entries")

type Entry struct {
	ID                 string    `json:"id"`
	EventType          string    `json:"eventType"`
	AccountID          string    `json:"accountId,omitempty"`
	UserID             string    `json:"userId,omitempty"`
	WorkspaceID        string    `json:"workspaceId,omitempty"`
	ComputeID          string    `json:"computeId,omitempty"`
	StorageID          string    `json:"storageId,omitempty"`
	AttachmentID       string    `json:"attachmentId,omitempty"`
	SourceEventID      string    `json:"sourceEventId,omitempty"`
	RequestFingerprint string    `json:"requestFingerprint,omitempty"`
	AmountCents        int64     `json:"amountCents"`
	Currency           string    `json:"currency"`
	CreatedAt          time.Time `json:"createdAt"`
}

type AppendEntryInput struct {
	EventType          string `json:"eventType"`
	AccountID          string `json:"accountId,omitempty"`
	UserID             string `json:"userId,omitempty"`
	WorkspaceID        string `json:"workspaceId,omitempty"`
	ComputeID          string `json:"computeId,omitempty"`
	StorageID          string `json:"storageId,omitempty"`
	AttachmentID       string `json:"attachmentId,omitempty"`
	SourceEventID      string `json:"sourceEventId,omitempty"`
	RequestFingerprint string `json:"requestFingerprint,omitempty"`
	AmountCents        int64  `json:"amountCents"`
	Currency           string `json:"currency"`
}

type AppendEntryResult struct {
	Entry
	Created bool
}

type EntryFilter struct {
	AccountID     string
	UserID        string
	WorkspaceID   string
	ComputeID     string
	StorageID     string
	AttachmentID  string
	SourceEventID string
}

type Summary struct {
	AccountID    string `json:"accountId,omitempty"`
	BalanceCents int64  `json:"balanceCents"`
	Currency     string `json:"currency"`
	EntryCount   int    `json:"entryCount"`
}
