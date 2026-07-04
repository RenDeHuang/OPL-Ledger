package ledger

import (
	"errors"
	"time"

	auditlog "github.com/RenDeHuang/OPL-Ledger/internal/audit"
	evidencelog "github.com/RenDeHuang/OPL-Ledger/internal/evidence"
	k8sevidence "github.com/RenDeHuang/OPL-Ledger/internal/k8s"
	"github.com/RenDeHuang/OPL-Ledger/internal/usage"
	"github.com/RenDeHuang/OPL-Ledger/internal/wallet"
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
	SourceEventID string           `json:"sourceEventId,omitempty"`
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
	SourceEventID string           `json:"sourceEventId,omitempty"`
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

type ReconciliationReportFilter struct {
	Provider string
	Status   string
}

type ReconciliationGuard struct {
	Status             string    `json:"status"`
	BlockNewWorkspaces bool      `json:"blockNewWorkspaces"`
	Reason             string    `json:"reason"`
	CheckedAt          time.Time `json:"checkedAt"`
	GeneratedAt        time.Time `json:"generatedAt,omitempty"`
	AgeHours           float64   `json:"ageHours,omitempty"`
}

type ManualTopUpInput struct {
	AccountID         string `json:"accountId"`
	UserID            string `json:"userId,omitempty"`
	AmountCents       int64  `json:"amountCents"`
	SourceEventID     string `json:"sourceEventId,omitempty"`
	Reason            string `json:"reason,omitempty"`
	OperatorUserID    string `json:"operatorUserId,omitempty"`
	OperatorAccountID string `json:"operatorAccountId,omitempty"`
}

type ManualTopUpResult struct {
	Wallet      wallet.Snapshot    `json:"wallet"`
	Entry       Entry              `json:"entry"`
	Transaction wallet.Transaction `json:"transaction"`
	TopUp       ManualTopUp        `json:"topUp"`
	AuditEvent  AuditEvent         `json:"auditEvent"`
	Created     bool               `json:"created"`
}

type ManualTopUp struct {
	ID                  string    `json:"id"`
	OperatorUserID      string    `json:"operatorUserId,omitempty"`
	OperatorAccountID   string    `json:"operatorAccountId,omitempty"`
	TargetUserID        string    `json:"targetUserId"`
	TargetAccountID     string    `json:"targetAccountId"`
	AmountCents         int64     `json:"amountCents"`
	Currency            string    `json:"currency"`
	SourceEventID       string    `json:"sourceEventId"`
	Reason              string    `json:"reason"`
	Status              string    `json:"status"`
	BalanceBeforeCents  int64     `json:"balanceBeforeCents"`
	BalanceAfterCents   int64     `json:"balanceAfterCents"`
	LedgerEntryID       string    `json:"ledgerEntryId"`
	WalletTransactionID string    `json:"walletTransactionId"`
	AuditEventID        string    `json:"auditEventId"`
	CreatedAt           time.Time `json:"createdAt"`
}

type ManualTopUpFilter struct {
	AccountID         string
	UserID            string
	OperatorUserID    string
	OperatorAccountID string
	SourceEventID     string
	Status            string
}

type WalletFilter struct {
	AccountID string
	UserID    string
}

type HoldInput struct {
	AccountID     string         `json:"accountId"`
	UserID        string         `json:"userId,omitempty"`
	WorkspaceID   string         `json:"workspaceId,omitempty"`
	HoldType      string         `json:"holdType"`
	AmountCents   int64          `json:"amountCents"`
	SourceEventID string         `json:"sourceEventId"`
	ResourceID    string         `json:"resourceId,omitempty"`
	PackageID     string         `json:"packageId,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

type HoldResult struct {
	Wallet      wallet.Snapshot    `json:"wallet"`
	Entry       Entry              `json:"entry"`
	Transaction wallet.Transaction `json:"transaction"`
	Created     bool               `json:"created"`
}

type ReleaseHoldInput struct {
	AccountID     string   `json:"accountId"`
	WorkspaceID   string   `json:"workspaceId,omitempty"`
	HoldTypes     []string `json:"holdTypes"`
	SourceEventID string   `json:"sourceEventId"`
	ComputeID     string   `json:"computeId,omitempty"`
	StorageID     string   `json:"storageId,omitempty"`
	Reason        string   `json:"reason,omitempty"`
}

type ReleaseHoldResult struct {
	Wallet       wallet.Snapshot      `json:"wallet"`
	Entries      []Entry              `json:"entries"`
	Transactions []wallet.Transaction `json:"transactions"`
	Created      bool                 `json:"created"`
}

type SettlementInput struct {
	AccountID          string `json:"accountId"`
	UserID             string `json:"userId,omitempty"`
	WorkspaceID        string `json:"workspaceId"`
	ComputeID          string `json:"computeId,omitempty"`
	StorageID          string `json:"storageId,omitempty"`
	SourceEventID      string `json:"sourceEventId"`
	Hours              int64  `json:"hours"`
	ComputeActive      bool   `json:"computeActive,omitempty"`
	StorageActive      bool   `json:"storageActive,omitempty"`
	ComputeHourlyCents int64  `json:"computeHourlyCents,omitempty"`
	StorageHourlyCents int64  `json:"storageHourlyCents,omitempty"`
}

type SettlementResult struct {
	Wallet       wallet.Snapshot      `json:"wallet"`
	Entries      []Entry              `json:"entries"`
	Transactions []wallet.Transaction `json:"transactions"`
	Intents      []SettlementIntent   `json:"intents,omitempty"`
	UnpaidCents  int64                `json:"unpaidCents"`
	Created      bool                 `json:"created"`
}

type SettlementIntentType string

const (
	IntentComputeAutoStopped   SettlementIntentType = "compute_auto_stopped"
	IntentStorageHoldExhausted SettlementIntentType = "storage_hold_exhausted"
)

type SettlementIntent struct {
	Type          SettlementIntentType `json:"type"`
	AccountID     string               `json:"accountId,omitempty"`
	WorkspaceID   string               `json:"workspaceId,omitempty"`
	ComputeID     string               `json:"computeId,omitempty"`
	StorageID     string               `json:"storageId,omitempty"`
	SourceEventID string               `json:"sourceEventId,omitempty"`
	Reason        string               `json:"reason,omitempty"`
}

type ResourceUsageInput struct {
	AccountID      string             `json:"accountId"`
	UserID         string             `json:"userId,omitempty"`
	WorkspaceID    string             `json:"workspaceId"`
	ComputeID      string             `json:"computeId,omitempty"`
	StorageID      string             `json:"storageId,omitempty"`
	AttachmentID   string             `json:"attachmentId,omitempty"`
	ResourceKind   usage.ResourceKind `json:"resourceKind"`
	Quantity       int64              `json:"quantity"`
	Unit           string             `json:"unit"`
	UnitPriceCents int64              `json:"unitPriceCents"`
	AmountCents    int64              `json:"amountCents"`
	RequestedCents int64              `json:"requestedCents,omitempty"`
	SourceEventID  string             `json:"sourceEventId"`
	Metadata       map[string]any     `json:"metadata,omitempty"`
}

type ResourceUsageResult struct {
	Log     usage.ResourceUsageLog `json:"log"`
	Created bool                   `json:"created"`
}

type ResourceUsageFilter struct {
	AccountID     string
	UserID        string
	WorkspaceID   string
	ComputeID     string
	StorageID     string
	AttachmentID  string
	ResourceKind  usage.ResourceKind
	SourceEventID string
}

type WalletTransactionFilter struct {
	AccountID     string
	UserID        string
	WorkspaceID   string
	Type          wallet.TransactionType
	SourceEventID string
	LedgerEntryID string
	UsageLogID    string
	FundingSource string
}

type AuditEvent = auditlog.Event
type AuditEventInput = auditlog.EventInput
type AuditEventFilter = auditlog.EventFilter

type EvidenceRecord = evidencelog.Record
type EvidenceRecordInput = evidencelog.RecordInput
type EvidenceRecordFilter = evidencelog.RecordFilter

type KubernetesEvidenceSnapshot = k8sevidence.Snapshot

type KubernetesEvidenceSnapshotFilter struct {
	ClusterID   string
	Namespace   string
	ObjectKind  string
	ObjectName  string
	WorkspaceID string
}

type RequestUsageInput struct {
	AccountID          string              `json:"accountId,omitempty"`
	UserID             string              `json:"userId,omitempty"`
	WorkspaceID        string              `json:"workspaceId"`
	RequestID          string              `json:"requestId"`
	Provider           string              `json:"provider,omitempty"`
	Model              string              `json:"model,omitempty"`
	InputTokens        int64               `json:"inputTokens,omitempty"`
	OutputTokens       int64               `json:"outputTokens,omitempty"`
	AmountCents        int64               `json:"amountCents"`
	SourceEventID      string              `json:"sourceEventId,omitempty"`
	RequestFingerprint string              `json:"requestFingerprint,omitempty"`
	RequestQuota       *usage.RequestQuota `json:"requestQuota,omitempty"`
}

type RequestQuotaInput struct {
	AccountID   string             `json:"accountId"`
	UserID      string             `json:"userId"`
	WorkspaceID string             `json:"workspaceId"`
	Quota       usage.RequestQuota `json:"quota"`
}

type RequestQuotaRecord struct {
	ID          string             `json:"id"`
	AccountID   string             `json:"accountId"`
	UserID      string             `json:"userId"`
	WorkspaceID string             `json:"workspaceId"`
	Quota       usage.RequestQuota `json:"quota"`
	CreatedAt   time.Time          `json:"createdAt"`
	UpdatedAt   time.Time          `json:"updatedAt"`
}

type RequestQuotaFilter struct {
	AccountID   string
	UserID      string
	WorkspaceID string
}

type RequestUsageResult struct {
	Log         RequestUsageLog    `json:"log"`
	Wallet      wallet.Snapshot    `json:"wallet"`
	Entry       Entry              `json:"entry,omitempty"`
	Transaction wallet.Transaction `json:"transaction,omitempty"`
	AuditEvent  AuditEvent         `json:"auditEvent"`
	Created     bool               `json:"created"`
}

type RequestUsageLog struct {
	ID                   string              `json:"id"`
	UserID               string              `json:"userId,omitempty"`
	AccountID            string              `json:"accountId"`
	WorkspaceID          string              `json:"workspaceId"`
	RequestID            string              `json:"requestId"`
	Provider             string              `json:"provider,omitempty"`
	Model                string              `json:"model,omitempty"`
	InputTokens          int64               `json:"inputTokens"`
	OutputTokens         int64               `json:"outputTokens"`
	AmountCents          int64               `json:"amountCents"`
	RequestedAmountCents int64               `json:"requestedAmountCents"`
	UnpaidCents          int64               `json:"unpaidCents"`
	Currency             string              `json:"currency"`
	SourceEventID        string              `json:"sourceEventId"`
	RequestFingerprint   string              `json:"requestFingerprint"`
	LedgerEntryID        string              `json:"ledgerEntryId,omitempty"`
	Quota                *usage.RequestQuota `json:"quota,omitempty"`
	CreatedAt            time.Time           `json:"createdAt"`
}

type RequestUsageFilter struct {
	AccountID          string
	UserID             string
	WorkspaceID        string
	RequestID          string
	SourceEventID      string
	RequestFingerprint string
	LedgerEntryID      string
	Provider           string
	Model              string
}
