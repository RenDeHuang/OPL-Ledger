package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type TransactionType string

const (
	TransactionCredit      TransactionType = "credit"
	TransactionHold        TransactionType = "hold"
	TransactionDebit       TransactionType = "debit"
	TransactionHoldRelease TransactionType = "hold_release"
	TransactionAdjustment  TransactionType = "adjustment"
)

type TransactionInput struct {
	ID                  string
	UserID              string
	AccountID           string
	WorkspaceID         string
	Type                TransactionType
	AmountCents         int64
	Currency            string
	SourceEventID       string
	LedgerEntryID       string
	UsageLogID          string
	FundingSource       string
	BalanceBeforeCents  int64
	BalanceAfterCents   int64
	FrozenBeforeCents   int64
	FrozenAfterCents    int64
	AvailableAfterCents int64
	Metadata            map[string]any
	CreatedAt           time.Time
}

type Transaction struct {
	ID                  string          `json:"id"`
	UserID              string          `json:"userId"`
	AccountID           string          `json:"accountId"`
	WorkspaceID         string          `json:"workspaceId,omitempty"`
	Type                TransactionType `json:"type"`
	AmountCents         int64           `json:"amountCents"`
	Currency            string          `json:"currency"`
	SourceEventID       string          `json:"sourceEventId,omitempty"`
	LedgerEntryID       string          `json:"ledgerEntryId,omitempty"`
	UsageLogID          string          `json:"usageLogId,omitempty"`
	FundingSource       string          `json:"fundingSource,omitempty"`
	BalanceBeforeCents  int64           `json:"balanceBeforeCents"`
	BalanceAfterCents   int64           `json:"balanceAfterCents"`
	FrozenBeforeCents   int64           `json:"frozenBeforeCents"`
	FrozenAfterCents    int64           `json:"frozenAfterCents"`
	AvailableAfterCents int64           `json:"availableAfterCents"`
	Metadata            map[string]any  `json:"metadata,omitempty"`
	CreatedAt           time.Time       `json:"createdAt"`
}

func NewTransaction(input TransactionInput) Transaction {
	if input.ID == "" {
		input.ID = randomTransactionID()
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	return Transaction{
		ID:                  input.ID,
		UserID:              input.UserID,
		AccountID:           input.AccountID,
		WorkspaceID:         input.WorkspaceID,
		Type:                input.Type,
		AmountCents:         input.AmountCents,
		Currency:            input.Currency,
		SourceEventID:       input.SourceEventID,
		LedgerEntryID:       input.LedgerEntryID,
		UsageLogID:          input.UsageLogID,
		FundingSource:       input.FundingSource,
		BalanceBeforeCents:  input.BalanceBeforeCents,
		BalanceAfterCents:   input.BalanceAfterCents,
		FrozenBeforeCents:   input.FrozenBeforeCents,
		FrozenAfterCents:    input.FrozenAfterCents,
		AvailableAfterCents: input.AvailableAfterCents,
		Metadata:            cloneMetadata(input.Metadata),
		CreatedAt:           input.CreatedAt,
	}
}

func IsValidTransactionType(kind TransactionType) bool {
	switch kind {
	case TransactionCredit, TransactionHold, TransactionDebit, TransactionHoldRelease, TransactionAdjustment:
		return true
	default:
		return false
	}
}

func cloneMetadata(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func randomTransactionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "wtx_" + hex.EncodeToString(b[:])
}
