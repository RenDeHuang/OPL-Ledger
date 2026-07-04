package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type MemoryStore struct {
	mu                    sync.Mutex
	entries               []Entry
	taskReceipts          []TaskReceipt
	reconciliationReports []ReconciliationReport
	bySourceEvent         map[string]Entry
	byRequestFingerprint  map[string]Entry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bySourceEvent:        map[string]Entry{},
		byRequestFingerprint: map[string]Entry{},
	}
}

func (s *MemoryStore) AppendEntry(_ context.Context, input AppendEntryInput) (AppendEntryResult, error) {
	if input.EventType == "" {
		return AppendEntryResult{}, errors.New("eventType is required")
	}
	if input.SourceEventID == "" && input.RequestFingerprint == "" {
		return AppendEntryResult{}, errors.New("sourceEventId or requestFingerprint is required")
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var sourceEntry Entry
	var sourceFound bool
	if input.SourceEventID != "" {
		sourceEntry, sourceFound = s.bySourceEvent[input.SourceEventID]
	}
	var fingerprintEntry Entry
	var fingerprintFound bool
	if input.RequestFingerprint != "" {
		fingerprintEntry, fingerprintFound = s.byRequestFingerprint[input.RequestFingerprint]
	}
	if sourceFound && fingerprintFound && sourceEntry.ID != fingerprintEntry.ID {
		return AppendEntryResult{}, ErrIdempotencyConflict
	}
	if sourceFound {
		entry := sourceEntry
		if !sameReplayPayload(entry, input) {
			return AppendEntryResult{}, ErrIdempotencyConflict
		}
		if input.RequestFingerprint != "" {
			if entry.RequestFingerprint != "" && entry.RequestFingerprint != input.RequestFingerprint {
				return AppendEntryResult{}, ErrIdempotencyConflict
			}
			if !fingerprintFound {
				entry = s.bindRequestFingerprint(entry, input.RequestFingerprint)
			}
		}
		return AppendEntryResult{Entry: entry, Created: false}, nil
	}
	if fingerprintFound {
		entry := fingerprintEntry
		if !sameReplayPayload(entry, input) {
			return AppendEntryResult{}, ErrIdempotencyConflict
		}
		if input.SourceEventID != "" {
			if entry.SourceEventID != "" && entry.SourceEventID != input.SourceEventID {
				return AppendEntryResult{}, ErrIdempotencyConflict
			}
			if !sourceFound {
				entry = s.bindSourceEvent(entry, input.SourceEventID)
			}
		}
		return AppendEntryResult{Entry: entry, Created: false}, nil
	}
	entry := Entry{
		ID:                 randomID(),
		EventType:          input.EventType,
		AccountID:          input.AccountID,
		UserID:             input.UserID,
		WorkspaceID:        input.WorkspaceID,
		ComputeID:          input.ComputeID,
		StorageID:          input.StorageID,
		AttachmentID:       input.AttachmentID,
		SourceEventID:      input.SourceEventID,
		RequestFingerprint: input.RequestFingerprint,
		AmountCents:        input.AmountCents,
		Currency:           input.Currency,
		CreatedAt:          time.Now().UTC(),
	}
	s.entries = append(s.entries, entry)
	if entry.SourceEventID != "" {
		s.bySourceEvent[entry.SourceEventID] = entry
	}
	if entry.RequestFingerprint != "" {
		s.byRequestFingerprint[entry.RequestFingerprint] = entry
	}
	return AppendEntryResult{Entry: entry, Created: true}, nil
}

func sameReplayPayload(entry Entry, input AppendEntryInput) bool {
	return entry.EventType == input.EventType &&
		entry.AccountID == input.AccountID &&
		entry.UserID == input.UserID &&
		entry.WorkspaceID == input.WorkspaceID &&
		entry.ComputeID == input.ComputeID &&
		entry.StorageID == input.StorageID &&
		entry.AttachmentID == input.AttachmentID &&
		entry.AmountCents == input.AmountCents &&
		entry.Currency == input.Currency
}

func (s *MemoryStore) bindRequestFingerprint(entry Entry, requestFingerprint string) Entry {
	entry.RequestFingerprint = requestFingerprint
	for i := range s.entries {
		if s.entries[i].ID == entry.ID {
			s.entries[i].RequestFingerprint = requestFingerprint
			entry = s.entries[i]
			break
		}
	}
	s.byRequestFingerprint[requestFingerprint] = entry
	if entry.SourceEventID != "" {
		s.bySourceEvent[entry.SourceEventID] = entry
	}
	return entry
}

func (s *MemoryStore) bindSourceEvent(entry Entry, sourceEventID string) Entry {
	entry.SourceEventID = sourceEventID
	for i := range s.entries {
		if s.entries[i].ID == entry.ID {
			s.entries[i].SourceEventID = sourceEventID
			entry = s.entries[i]
			break
		}
	}
	s.bySourceEvent[sourceEventID] = entry
	if entry.RequestFingerprint != "" {
		s.byRequestFingerprint[entry.RequestFingerprint] = entry
	}
	return entry
}

func (s *MemoryStore) ListEntries(_ context.Context, filter EntryFilter) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, entry := range s.entries {
		if matches(entry, filter) {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *MemoryStore) Summary(ctx context.Context, filter EntryFilter) (Summary, error) {
	entries, err := s.ListEntries(ctx, filter)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{AccountID: filter.AccountID, Currency: "CNY", EntryCount: len(entries)}
	for _, entry := range entries {
		summary.BalanceCents += entry.AmountCents
		if entry.Currency != "" {
			summary.Currency = entry.Currency
		}
	}
	return summary, nil
}

func (s *MemoryStore) AppendTaskReceipt(_ context.Context, input TaskReceiptInput) (TaskReceipt, error) {
	if input.AccountID == "" {
		return TaskReceipt{}, errors.New("task_evidence_account_required")
	}
	if input.TaskID == "" {
		return TaskReceipt{}, errors.New("task_evidence_task_required")
	}
	if len(input.Plan) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_plan_required")
	}
	if len(input.Approval) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_approval_required")
	}
	if len(input.Environment) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_environment_required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	receipt := TaskReceipt{
		ID:            randomID(),
		Type:          "task.evidence.v1",
		AccountID:     input.AccountID,
		WorkspaceID:   input.WorkspaceID,
		TaskID:        input.TaskID,
		Actor:         mapOrDefault(input.Actor, map[string]any{"type": "system", "id": "opl-ledger"}),
		Plan:          cloneMap(input.Plan),
		Approval:      cloneMap(input.Approval),
		Environment:   cloneMap(input.Environment),
		InputRefs:     cloneMapSlice(input.InputRefs),
		ExecutionRefs: cloneMapSlice(input.ExecutionRefs),
		OutputRefs:    cloneMapSlice(input.OutputRefs),
		ReviewResults: cloneMapSlice(input.ReviewResults),
		Continuation:  cloneMap(input.Continuation),
		Metadata:      cloneMap(input.Metadata),
		CreatedAt:     time.Now().UTC(),
	}
	s.taskReceipts = append(s.taskReceipts, receipt)
	return receipt, nil
}

func (s *MemoryStore) ListTaskReceipts(_ context.Context, filter TaskReceiptFilter) ([]TaskReceipt, error) {
	if filter.AccountID == "" {
		return nil, errors.New("task_evidence_account_required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []TaskReceipt
	for _, receipt := range s.taskReceipts {
		if receipt.AccountID != filter.AccountID {
			continue
		}
		if filter.WorkspaceID != "" && receipt.WorkspaceID != filter.WorkspaceID {
			continue
		}
		if filter.TaskID != "" && receipt.TaskID != filter.TaskID {
			continue
		}
		out = append(out, receipt)
	}
	return out, nil
}

func (s *MemoryStore) AppendReconciliationReport(_ context.Context, report ReconciliationReport) (ReconciliationReport, error) {
	if report.ID == "" {
		report.ID = randomID()
	}
	if report.Provider == "" {
		report.Provider = "manual"
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reconciliationReports = append(s.reconciliationReports, report)
	return report, nil
}

func (s *MemoryStore) LatestReconciliationReport(_ context.Context) (ReconciliationReport, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.reconciliationReports) == 0 {
		return ReconciliationReport{}, errors.New("billing_reconciliation_report_missing")
	}
	return s.reconciliationReports[len(s.reconciliationReports)-1], nil
}

func matches(entry Entry, filter EntryFilter) bool {
	if filter.AccountID != "" && entry.AccountID != filter.AccountID {
		return false
	}
	if filter.UserID != "" && entry.UserID != filter.UserID {
		return false
	}
	if filter.WorkspaceID != "" && entry.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.ComputeID != "" && entry.ComputeID != filter.ComputeID {
		return false
	}
	if filter.StorageID != "" && entry.StorageID != filter.StorageID {
		return false
	}
	if filter.AttachmentID != "" && entry.AttachmentID != filter.AttachmentID {
		return false
	}
	if filter.SourceEventID != "" && entry.SourceEventID != filter.SourceEventID {
		return false
	}
	return true
}

func mapOrDefault(value map[string]any, fallback map[string]any) map[string]any {
	if len(value) == 0 {
		return cloneMap(fallback)
	}
	return cloneMap(value)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
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

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "led_" + hex.EncodeToString(b[:])
}
