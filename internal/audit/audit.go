package audit

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

type Event struct {
	ID            string         `json:"id"`
	AccountID     string         `json:"accountId,omitempty"`
	WorkspaceID   string         `json:"workspaceId,omitempty"`
	ActorID       string         `json:"actorId,omitempty"`
	Action        string         `json:"action"`
	TargetKind    string         `json:"targetKind"`
	TargetID      string         `json:"targetId,omitempty"`
	SourceEventID string         `json:"sourceEventId,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
}

type EventInput struct {
	AccountID     string         `json:"accountId,omitempty"`
	WorkspaceID   string         `json:"workspaceId,omitempty"`
	ActorID       string         `json:"actorId,omitempty"`
	Action        string         `json:"action"`
	TargetKind    string         `json:"targetKind"`
	TargetID      string         `json:"targetId,omitempty"`
	SourceEventID string         `json:"sourceEventId,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
	CreatedAt     time.Time      `json:"createdAt,omitempty"`
}

type EventFilter struct {
	AccountID     string
	WorkspaceID   string
	Action        string
	SourceEventID string
}

type MemoryStore struct {
	mu     sync.Mutex
	events []Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (s *MemoryStore) Append(input EventInput) (Event, error) {
	event, err := NewEvent(input)
	if err != nil {
		return Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return event, nil
}

func (s *MemoryStore) List(filter EventFilter) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Event
	for _, event := range s.events {
		if Matches(event, filter) {
			out = append(out, cloneEvent(event))
		}
	}
	return out
}

func NewEvent(input EventInput) (Event, error) {
	if input.Action == "" {
		return Event{}, errors.New("audit_action_required")
	}
	if input.TargetKind == "" {
		return Event{}, errors.New("audit_target_kind_required")
	}
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	return Event{
		ID:            randomAuditID(),
		AccountID:     input.AccountID,
		WorkspaceID:   input.WorkspaceID,
		ActorID:       input.ActorID,
		Action:        input.Action,
		TargetKind:    input.TargetKind,
		TargetID:      input.TargetID,
		SourceEventID: input.SourceEventID,
		Payload:       clonePayload(input.Payload),
		CreatedAt:     createdAt,
	}, nil
}

func Matches(event Event, filter EventFilter) bool {
	if filter.AccountID != "" && event.AccountID != filter.AccountID {
		return false
	}
	if filter.WorkspaceID != "" && event.WorkspaceID != filter.WorkspaceID {
		return false
	}
	if filter.Action != "" && event.Action != filter.Action {
		return false
	}
	if filter.SourceEventID != "" && event.SourceEventID != filter.SourceEventID {
		return false
	}
	return true
}

func cloneEvent(event Event) Event {
	event.Payload = clonePayload(event.Payload)
	return event
}

func clonePayload(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = item
	}
	return out
}

func randomAuditID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "aud_" + hex.EncodeToString(b[:])
}
