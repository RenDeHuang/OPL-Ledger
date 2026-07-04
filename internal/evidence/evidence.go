package evidence

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

type Record struct {
	ID            string           `json:"id"`
	Type          string           `json:"type"`
	AccountID     string           `json:"accountId"`
	WorkspaceID   string           `json:"workspaceId"`
	Actor         map[string]any   `json:"actor"`
	Plan          map[string]any   `json:"plan"`
	Approval      map[string]any   `json:"approval"`
	Environment   map[string]any   `json:"environment"`
	ResourceRefs  map[string]any   `json:"resourceRefs"`
	BillingRefs   []map[string]any `json:"billingRefs"`
	InputRefs     []map[string]any `json:"inputRefs"`
	ExecutionRefs []map[string]any `json:"executionRefs"`
	OutputRefs    []map[string]any `json:"outputRefs"`
	ReviewResults []map[string]any `json:"reviewResults"`
	Continuation  map[string]any   `json:"continuation,omitempty"`
	SourceEventID string           `json:"sourceEventId,omitempty"`
	CreatedAt     time.Time        `json:"createdAt"`
}

type RecordInput struct {
	Type          string           `json:"type"`
	AccountID     string           `json:"accountId"`
	WorkspaceID   string           `json:"workspaceId"`
	Actor         map[string]any   `json:"actor,omitempty"`
	Plan          map[string]any   `json:"plan"`
	Approval      map[string]any   `json:"approval"`
	Environment   map[string]any   `json:"environment"`
	ResourceRefs  map[string]any   `json:"resourceRefs,omitempty"`
	BillingRefs   []map[string]any `json:"billingRefs,omitempty"`
	InputRefs     []map[string]any `json:"inputRefs,omitempty"`
	ExecutionRefs []map[string]any `json:"executionRefs,omitempty"`
	OutputRefs    []map[string]any `json:"outputRefs,omitempty"`
	ReviewResults []map[string]any `json:"reviewResults,omitempty"`
	Continuation  map[string]any   `json:"continuation,omitempty"`
	SourceEventID string           `json:"sourceEventId,omitempty"`
	CreatedAt     time.Time        `json:"createdAt,omitempty"`
}

type RecordFilter struct {
	AccountID     string
	WorkspaceID   string
	Type          string
	SourceEventID string
}

func NewRecord(input RecordInput) (Record, error) {
	if input.Type == "" {
		return Record{}, errors.New("evidence_type_required")
	}
	if input.AccountID == "" {
		return Record{}, errors.New("evidence_account_required")
	}
	if input.WorkspaceID == "" {
		return Record{}, errors.New("evidence_workspace_required")
	}
	if len(input.Plan) == 0 {
		return Record{}, errors.New("evidence_plan_required")
	}
	if len(input.Approval) == 0 {
		return Record{}, errors.New("evidence_approval_required")
	}
	if len(input.Environment) == 0 {
		return Record{}, errors.New("evidence_environment_required")
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return Record{
		ID:            randomEvidenceID(),
		Type:          input.Type,
		AccountID:     input.AccountID,
		WorkspaceID:   input.WorkspaceID,
		Actor:         mapOrDefault(input.Actor, map[string]any{"type": "system", "id": "opl-ledger"}),
		Plan:          cloneMap(input.Plan),
		Approval:      cloneMap(input.Approval),
		Environment:   cloneMap(input.Environment),
		ResourceRefs:  cloneMap(input.ResourceRefs),
		BillingRefs:   cloneMapSlice(input.BillingRefs),
		InputRefs:     cloneMapSlice(input.InputRefs),
		ExecutionRefs: cloneMapSlice(input.ExecutionRefs),
		OutputRefs:    cloneMapSlice(input.OutputRefs),
		ReviewResults: cloneMapSlice(input.ReviewResults),
		Continuation:  cloneMap(input.Continuation),
		SourceEventID: input.SourceEventID,
		CreatedAt:     createdAt,
	}, nil
}

func Matches(record Record, filter RecordFilter) bool {
	if filter.AccountID != "" && record.AccountID != filter.AccountID {
		return false
	}
	if filter.WorkspaceID != "" && record.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.Type != "" && record.Type != filter.Type {
		return false
	}
	if filter.SourceEventID != "" && record.SourceEventID != filter.SourceEventID {
		return false
	}
	return true
}

func CloneRecord(record Record) Record {
	record.Actor = cloneMap(record.Actor)
	record.Plan = cloneMap(record.Plan)
	record.Approval = cloneMap(record.Approval)
	record.Environment = cloneMap(record.Environment)
	record.ResourceRefs = cloneMap(record.ResourceRefs)
	record.BillingRefs = cloneMapSlice(record.BillingRefs)
	record.InputRefs = cloneMapSlice(record.InputRefs)
	record.ExecutionRefs = cloneMapSlice(record.ExecutionRefs)
	record.OutputRefs = cloneMapSlice(record.OutputRefs)
	record.ReviewResults = cloneMapSlice(record.ReviewResults)
	record.Continuation = cloneMap(record.Continuation)
	return record
}

func mapOrDefault(value map[string]any, fallback map[string]any) map[string]any {
	if len(value) == 0 {
		return cloneMap(fallback)
	}
	return cloneMap(value)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func cloneMapSlice(value []map[string]any) []map[string]any {
	if value == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(value))
	for _, item := range value {
		out = append(out, cloneMap(item))
	}
	return out
}

func randomEvidenceID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "evd_" + hex.EncodeToString(b[:])
}
