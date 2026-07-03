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
	mu                   sync.Mutex
	entries              []Entry
	bySourceEvent        map[string]Entry
	byRequestFingerprint map[string]Entry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bySourceEvent:        map[string]Entry{},
		byRequestFingerprint: map[string]Entry{},
	}
}

func (s *MemoryStore) AppendEntry(_ context.Context, input AppendEntryInput) (Entry, error) {
	if input.EventType == "" {
		return Entry{}, errors.New("eventType is required")
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
		return Entry{}, errors.New("idempotency keys resolve to different ledger entries")
	}
	if sourceFound {
		entry := sourceEntry
		if input.RequestFingerprint != "" && !fingerprintFound {
			entry = s.bindRequestFingerprint(entry, input.RequestFingerprint)
		}
		return entry, nil
	}
	if fingerprintFound {
		entry := fingerprintEntry
		if input.SourceEventID != "" && !sourceFound {
			entry = s.bindSourceEvent(entry, input.SourceEventID)
		}
		return entry, nil
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
	return entry, nil
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

func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "led_" + hex.EncodeToString(b[:])
}
