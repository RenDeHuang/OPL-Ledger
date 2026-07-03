package api

import (
	"encoding/json"
	"net/http"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
	"github.com/RenDeHuang/OPL-Ledger/internal/version"
)

type Server struct {
	store *ledger.MemoryStore
	mux   *http.ServeMux
}

func NewServer(store *ledger.MemoryStore) http.Handler {
	s := &Server{store: store, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.healthz)
	s.mux.HandleFunc("POST /api/v1/ledger/entries", s.appendEntry)
	s.mux.HandleFunc("GET /api/v1/ledger/entries", s.listEntries)
	s.mux.HandleFunc("GET /api/v1/ledger/summary", s.summary)
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"service": version.ServiceName, "apiVersion": version.APIVersion})
}

func (s *Server) appendEntry(w http.ResponseWriter, r *http.Request) {
	var input ledger.AppendEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	before, _ := s.store.ListEntries(r.Context(), ledger.EntryFilter{SourceEventID: input.SourceEventID})
	entry, err := s.store.AppendEntry(r.Context(), input)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	status := http.StatusCreated
	if input.SourceEventID != "" && len(before) > 0 {
		status = http.StatusOK
	}
	writeJSON(w, status, entry)
}

func (s *Server) listEntries(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.ListEntries(r.Context(), filterFromQuery(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.store.Summary(r.Context(), filterFromQuery(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func filterFromQuery(r *http.Request) ledger.EntryFilter {
	q := r.URL.Query()
	return ledger.EntryFilter{
		AccountID:     q.Get("accountId"),
		UserID:        q.Get("userId"),
		WorkspaceID:   q.Get("workspaceId"),
		ComputeID:     q.Get("computeId"),
		StorageID:     q.Get("storageId"),
		AttachmentID:  q.Get("attachmentId"),
		SourceEventID: q.Get("sourceEventId"),
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
