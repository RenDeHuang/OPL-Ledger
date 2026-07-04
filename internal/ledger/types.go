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

type TaskReceiptInput struct {
	AccountID     string           `json:"accountId"`
	WorkspaceID   string           `json:"workspaceId,omitempty"`
	TaskID        string           `json:"taskId"`
	Actor         map[string]any   `json:"actor,omitempty"`
	Plan          map[string]any   `json:"plan"`
	Approval      map[string]any   `json:"approval"`
	Environment   map[string]any   `json:"environment"`
	InputRefs     []map[string]any `json:"inputRefs,omitempty"`
	ExecutionRefs []map[string]any `json:"executionRefs,omitempty"`
	OutputRefs    []map[string]any `json:"outputRefs,omitempty"`
	ReviewResults []map[string]any `json:"reviewResults,omitempty"`
	Continuation  map[string]any   `json:"continuation,omitempty"`
	Metadata      map[string]any   `json:"metadata,omitempty"`
}

type TaskReceipt struct {
	ID            string           `json:"id"`
	Type          string           `json:"type"`
	AccountID     string           `json:"accountId"`
	WorkspaceID   string           `json:"workspaceId,omitempty"`
	TaskID        string           `json:"taskId"`
	Actor         map[string]any   `json:"actor"`
	Plan          map[string]any   `json:"plan"`
	Approval      map[string]any   `json:"approval"`
	Environment   map[string]any   `json:"environment"`
	InputRefs     []map[string]any `json:"inputRefs"`
	ExecutionRefs []map[string]any `json:"executionRefs"`
	OutputRefs    []map[string]any `json:"outputRefs"`
	ReviewResults []map[string]any `json:"reviewResults"`
	Continuation  map[string]any   `json:"continuation,omitempty"`
	Metadata      map[string]any   `json:"metadata,omitempty"`
	CreatedAt     time.Time        `json:"createdAt"`
}

type TaskReceiptFilter struct {
	AccountID   string
	WorkspaceID string
	TaskID      string
}

type ReconciliationReport struct {
	ID                  string         `json:"id"`
	Provider            string         `json:"provider"`
	Status              string         `json:"status"`
	LedgerAmountCents   int64          `json:"ledgerAmountCents"`
	ExpectedAmountCents int64          `json:"expectedAmountCents"`
	DifferenceCents     int64          `json:"differenceCents"`
	Payload             map[string]any `json:"payload"`
	CreatedAt           time.Time      `json:"createdAt"`
}
