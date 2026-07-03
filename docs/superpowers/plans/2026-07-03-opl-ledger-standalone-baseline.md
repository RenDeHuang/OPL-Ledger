# OPL Ledger Standalone Baseline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first runnable `OPL-Ledger` baseline as a standalone Go/PostgreSQL service with copied Ledger contracts, idempotent ledger append/list APIs, Kubernetes evidence collection through `client-go`, and a minimal React + TypeScript operator UI.

**Architecture:** Keep Ledger independent from Console and Fabric. The Go API owns append-first ledger records, audit/evidence receipts, reconciliation primitives, and operator evidence collection; PostgreSQL migrations define the production schema; the React UI is an operator/admin read surface only. Kubernetes access is read-only evidence collection and never provisions compute, storage, routes, or Workspaces.

**Tech Stack:** Go 1.22+, PostgreSQL migrations as SQL files, Kubernetes `client-go`, React + TypeScript + Vite, Node for frontend tooling only.

---

## File Structure

Create these files in `RenDeHuang/OPL-Ledger`:

- `go.mod`: Go module and dependencies.
- `cmd/opl-ledger-api/main.go`: HTTP API process entrypoint.
- `cmd/opl-ledger-worker/main.go`: worker process entrypoint for scheduled jobs introduced after the baseline.
- `internal/version/version.go`: build/version metadata used by health checks.
- `internal/db/migrations.go`: embeds migration SQL for tests and runtime migration access.
- `internal/db/migrations_test.go`: asserts required Ledger tables exist.
- `internal/db/migrations/0001_initial.sql`: embedded PostgreSQL schema for append-first Ledger tables.
- `internal/ledger/types.go`: Ledger domain types.
- `internal/ledger/memory_store.go`: in-memory store for first API tests and local dev.
- `internal/ledger/store_test.go`: idempotent append/list/summary tests.
- `internal/api/server.go`: HTTP routes and JSON handling.
- `internal/api/server_test.go`: API contract tests.
- `internal/reconciliation/tencent.go`: normalized Tencent bill reconciliation.
- `internal/reconciliation/tencent_test.go`: reconciliation pass/fail tests.
- `internal/k8s/evidence.go`: read-only Kubernetes evidence collector.
- `internal/k8s/evidence_test.go`: fake `client-go` collector tests.
- `contracts/README.md`: Ledger contract index and source mapping.
- `contracts/opl-cloud-billing-ledger-contract.json`: copied from original OPL Cloud.
- `contracts/opl-cloud-evidence-ledger-contract.json`: copied from original OPL Cloud.
- `contracts/opl-cloud-storage-backup-contract.json`: copied from original OPL Cloud for receipt evidence references.
- `contracts/opl-cloud-deployment-contract.json`: copied from original OPL Cloud for deployment evidence references.
- `web/package.json`: React + TypeScript scripts and dependencies.
- `web/tsconfig.json`: TypeScript config.
- `web/vite.config.ts`: Vite config.
- `web/index.html`: Vite HTML entry.
- `web/src/App.tsx`: operator Ledger UI shell.
- `web/src/App.test.tsx`: UI smoke test.
- `web/src/main.tsx`: React mount entry.
- `web/src/styles.css`: operator UI styles.
- `README.md`: project positioning and local commands.
- `.gitignore`: generated and local state exclusions.

Do not create Node backend code. Do not copy OPL Console API files into this repository.

---

### Task 1: Repository Baseline

**Files:**
- Create: `.gitignore`
- Create: `README.md`
- Create: `go.mod`
- Create: `internal/version/version.go`
- Test: `go test ./...`

- [ ] **Step 1: Write the baseline files**

Create `.gitignore`:

```gitignore
.DS_Store
.env
.runtime/
bin/
coverage/
dist/
node_modules/
web/dist/
```

Create `README.md`:

```markdown
# OPL Ledger

`OPL-Ledger` is the standalone Ledger service split from `RenDeHuang/OPL-Cloud`.

It owns billing ledger events, audit events, evidence records, receipts, idempotency, usage records, Tencent bill reconciliation, and read-only Kubernetes runtime evidence snapshots.

It does not own OPL Console workspace lifecycle screens, OPL Fabric provisioning, OPL Gateway internals, One Person Lab framework internals, or `one-person-lab-app` runtime behavior.

## Stack

- Frontend: React + TypeScript
- Backend: Go
- Database: PostgreSQL
- Kubernetes: Go `client-go`

## First Verification

```bash
go test ./...
git diff --check
```
```

Create `go.mod`:

```go
module github.com/RenDeHuang/OPL-Ledger

go 1.22
```

Create `internal/version/version.go`:

```go
package version

const ServiceName = "opl-ledger"
const APIVersion = "v1"
```

- [ ] **Step 2: Run baseline Go tests**

Run:

```bash
go test ./...
```

Expected: PASS with output including `?    github.com/RenDeHuang/OPL-Ledger/internal/version`.

- [ ] **Step 3: Commit**

```bash
git add .gitignore README.md go.mod internal/version/version.go
git commit -m "chore: establish opl ledger baseline"
```

---

### Task 2: Ledger Contracts

**Files:**
- Create: `contracts/README.md`
- Create: `contracts/opl-cloud-billing-ledger-contract.json`
- Create: `contracts/opl-cloud-evidence-ledger-contract.json`
- Create: `contracts/opl-cloud-storage-backup-contract.json`
- Create: `contracts/opl-cloud-deployment-contract.json`
- Test: `test -s contracts/opl-cloud-billing-ledger-contract.json`

- [ ] **Step 1: Copy source contracts from original OPL Cloud**

Run:

```bash
mkdir -p contracts
cp /home/dev/opl-cloud/packages/contracts/opl-cloud-billing-ledger-contract.json contracts/
cp /home/dev/opl-cloud/packages/contracts/opl-cloud-evidence-ledger-contract.json contracts/
cp /home/dev/opl-cloud/packages/contracts/opl-cloud-storage-backup-contract.json contracts/
cp /home/dev/opl-cloud/packages/contracts/opl-cloud-deployment-contract.json contracts/
```

Create `contracts/README.md`:

```markdown
# OPL Ledger Contracts

These contracts are copied from the original `RenDeHuang/OPL-Cloud` implementation and form the first machine-readable Ledger boundary for `RenDeHuang/OPL-Ledger`.

## Included

- `opl-cloud-billing-ledger-contract.json`: billing ledger semantics.
- `opl-cloud-evidence-ledger-contract.json`: evidence and receipt semantics.
- `opl-cloud-storage-backup-contract.json`: backup and restore receipt evidence references.
- `opl-cloud-deployment-contract.json`: deployment and runtime verification evidence references.

## Ownership

`OPL-Ledger` owns Ledger records, receipts, evidence, idempotency, and reconciliation behavior derived from these contracts.

`OPL-Ledger` does not own OPL Console, OPL Fabric, OPL Gateway, OPL Workspace runtime, or One Person Lab framework internals.
```

- [ ] **Step 2: Verify copied contract files exist**

Run:

```bash
test -s contracts/opl-cloud-billing-ledger-contract.json
test -s contracts/opl-cloud-evidence-ledger-contract.json
test -s contracts/opl-cloud-storage-backup-contract.json
test -s contracts/opl-cloud-deployment-contract.json
```

Expected: all commands exit `0`.

- [ ] **Step 3: Commit**

```bash
git add contracts
git commit -m "docs: import ledger source contracts"
```

---

### Task 3: PostgreSQL Migration Baseline

**Files:**
- Create: `internal/db/migrations/0001_initial.sql`
- Create: `internal/db/migrations.go`
- Create: `internal/db/migrations_test.go`

- [ ] **Step 1: Write the failing migration coverage test**

Create `internal/db/migrations_test.go`:

```go
package db

import (
	"strings"
	"testing"
)

func TestInitialMigrationDefinesLedgerTables(t *testing.T) {
	sqlBytes, err := Migrations.ReadFile("migrations/0001_initial.sql")
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	sql := string(sqlBytes)
	required := []string{
		"CREATE TABLE IF NOT EXISTS ledger_entries",
		"CREATE TABLE IF NOT EXISTS audit_events",
		"CREATE TABLE IF NOT EXISTS evidence_records",
		"CREATE TABLE IF NOT EXISTS task_receipts",
		"CREATE TABLE IF NOT EXISTS request_usage_logs",
		"CREATE TABLE IF NOT EXISTS resource_usage_logs",
		"CREATE TABLE IF NOT EXISTS wallet_transactions",
		"CREATE TABLE IF NOT EXISTS manual_topups",
		"CREATE TABLE IF NOT EXISTS billing_reconciliation_reports",
		"CREATE TABLE IF NOT EXISTS idempotency_keys",
		"CREATE TABLE IF NOT EXISTS kubernetes_evidence_snapshots",
	}
	for _, needle := range required {
		if !strings.Contains(sql, needle) {
			t.Fatalf("migration missing %q", needle)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/db
```

Expected: FAIL because package `internal/db` has no `Migrations` symbol or migration file.

- [ ] **Step 3: Implement embedded migrations and schema**

Create `internal/db/migrations.go`:

```go
package db

import "embed"

// Migrations embeds SQL files from the package-local migrations directory.
//
//go:embed migrations/*.sql
var Migrations embed.FS
```

Create `internal/db/migrations/0001_initial.sql`:

```sql
CREATE TABLE IF NOT EXISTS ledger_entries (
  id UUID PRIMARY KEY,
  event_type TEXT NOT NULL,
  account_id TEXT,
  user_id TEXT,
  workspace_id TEXT,
  compute_id TEXT,
  storage_id TEXT,
  attachment_id TEXT,
  source_event_id TEXT,
  request_fingerprint TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'CNY',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ledger_entries_source_event_idx
  ON ledger_entries(source_event_id)
  WHERE source_event_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ledger_entries_request_fingerprint_idx
  ON ledger_entries(request_fingerprint)
  WHERE request_fingerprint IS NOT NULL;

CREATE TABLE IF NOT EXISTS audit_events (
  id UUID PRIMARY KEY,
  actor_id TEXT,
  action TEXT NOT NULL,
  target_kind TEXT NOT NULL,
  target_id TEXT,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS evidence_records (
  id UUID PRIMARY KEY,
  evidence_type TEXT NOT NULL,
  account_id TEXT,
  workspace_id TEXT,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS task_receipts (
  id UUID PRIMARY KEY,
  task_id TEXT NOT NULL,
  receipt_type TEXT NOT NULL,
  status TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS request_usage_logs (
  id UUID PRIMARY KEY,
  account_id TEXT,
  user_id TEXT,
  workspace_id TEXT,
  request_fingerprint TEXT NOT NULL,
  units BIGINT NOT NULL DEFAULT 1,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(request_fingerprint)
);

CREATE TABLE IF NOT EXISTS resource_usage_logs (
  id UUID PRIMARY KEY,
  account_id TEXT,
  workspace_id TEXT,
  compute_id TEXT,
  storage_id TEXT,
  resource_kind TEXT NOT NULL,
  quantity NUMERIC NOT NULL,
  unit TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS wallet_transactions (
  id UUID PRIMARY KEY,
  account_id TEXT,
  user_id TEXT,
  transaction_type TEXT NOT NULL,
  amount_cents BIGINT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  ledger_entry_id UUID REFERENCES ledger_entries(id),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS manual_topups (
  id UUID PRIMARY KEY,
  account_id TEXT NOT NULL,
  user_id TEXT,
  operator_id TEXT NOT NULL,
  amount_cents BIGINT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  audit_event_id UUID REFERENCES audit_events(id),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_reconciliation_reports (
  id UUID PRIMARY KEY,
  provider TEXT NOT NULL,
  account_id TEXT,
  status TEXT NOT NULL,
  expected_amount_cents BIGINT NOT NULL,
  actual_amount_cents BIGINT NOT NULL,
  difference_cents BIGINT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
  key TEXT PRIMARY KEY,
  operation TEXT NOT NULL,
  result_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kubernetes_evidence_snapshots (
  id UUID PRIMARY KEY,
  cluster_id TEXT NOT NULL,
  namespace TEXT NOT NULL,
  object_kind TEXT NOT NULL,
  object_name TEXT NOT NULL,
  workspace_id TEXT,
  resource_version TEXT,
  observed_generation BIGINT,
  readiness_status TEXT NOT NULL,
  redacted_object JSONB NOT NULL DEFAULT '{}'::jsonb,
  collected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- [ ] **Step 4: Run migration test**

Run:

```bash
go test ./internal/db
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add migrations internal/db
git commit -m "feat: add postgres ledger schema"
```

---

### Task 4: Idempotent Ledger Store

**Files:**
- Create: `internal/ledger/types.go`
- Create: `internal/ledger/memory_store.go`
- Create: `internal/ledger/store_test.go`

- [ ] **Step 1: Write failing store tests**

Create `internal/ledger/store_test.go`:

```go
package ledger

import (
	"context"
	"testing"
)

func TestMemoryStoreAppendIsIdempotentBySourceEvent(t *testing.T) {
	store := NewMemoryStore()
	input := AppendEntryInput{
		EventType:     "compute_debit",
		AccountID:     "acct_1",
		WorkspaceID:   "ws_1",
		ComputeID:     "compute_1",
		SourceEventID: "evt_1",
		AmountCents:   390,
		Currency:      "CNY",
	}
	first, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("first append: %v", err)
	}
	second, err := store.AppendEntry(context.Background(), input)
	if err != nil {
		t.Fatalf("second append: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected idempotent ID %q, got %q", first.ID, second.ID)
	}
	entries, err := store.ListEntries(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("list entries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestMemoryStoreSummaryTotalsAccountBalance(t *testing.T) {
	store := NewMemoryStore()
	_, _ = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType: "manual_topup", AccountID: "acct_1", SourceEventID: "topup_1", AmountCents: 1000, Currency: "CNY",
	})
	_, _ = store.AppendEntry(context.Background(), AppendEntryInput{
		EventType: "compute_debit", AccountID: "acct_1", SourceEventID: "debit_1", AmountCents: -390, Currency: "CNY",
	})
	summary, err := store.Summary(context.Background(), EntryFilter{AccountID: "acct_1"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.BalanceCents != 610 {
		t.Fatalf("expected balance 610, got %d", summary.BalanceCents)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/ledger
```

Expected: FAIL because `NewMemoryStore` and types are not defined.

- [ ] **Step 3: Implement store types and memory store**

Create `internal/ledger/types.go`:

```go
package ledger

import "time"

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

type EntryFilter struct {
	AccountID   string
	UserID      string
	WorkspaceID string
	ComputeID   string
	StorageID   string
	AttachmentID string
	SourceEventID string
}

type Summary struct {
	AccountID     string `json:"accountId,omitempty"`
	BalanceCents int64  `json:"balanceCents"`
	Currency     string `json:"currency"`
	EntryCount   int    `json:"entryCount"`
}
```

Create `internal/ledger/memory_store.go`:

```go
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
	mu                sync.Mutex
	entries           []Entry
	bySourceEvent     map[string]Entry
	byRequestFingerprint map[string]Entry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		bySourceEvent: map[string]Entry{},
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
	if input.SourceEventID != "" {
		if existing, ok := s.bySourceEvent[input.SourceEventID]; ok {
			return existing, nil
		}
	}
	if input.RequestFingerprint != "" {
		if existing, ok := s.byRequestFingerprint[input.RequestFingerprint]; ok {
			return existing, nil
		}
	}
	entry := Entry{
		ID: randomID(),
		EventType: input.EventType,
		AccountID: input.AccountID,
		UserID: input.UserID,
		WorkspaceID: input.WorkspaceID,
		ComputeID: input.ComputeID,
		StorageID: input.StorageID,
		AttachmentID: input.AttachmentID,
		SourceEventID: input.SourceEventID,
		RequestFingerprint: input.RequestFingerprint,
		AmountCents: input.AmountCents,
		Currency: input.Currency,
		CreatedAt: time.Now().UTC(),
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
```

- [ ] **Step 4: Run store tests**

Run:

```bash
go test ./internal/ledger
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ledger
git commit -m "feat: add idempotent ledger store"
```

---

### Task 5: HTTP API Baseline

**Files:**
- Create: `internal/api/server.go`
- Create: `internal/api/server_test.go`
- Create: `cmd/opl-ledger-api/main.go`

- [ ] **Step 1: Write failing API tests**

Create `internal/api/server_test.go`:

```go
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
)

func TestAppendLedgerEntryIsIdempotent(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	body := []byte(`{"eventType":"compute_debit","accountId":"acct_1","workspaceId":"ws_1","sourceEventId":"evt_1","amountCents":-390,"currency":"CNY"}`)
	first := httptest.NewRecorder()
	server.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(body)))
	if first.Code != http.StatusCreated {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	server.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(body)))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d body=%s", second.Code, second.Body.String())
	}
	var a ledger.Entry
	var b ledger.Entry
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a.ID != b.ID {
		t.Fatalf("expected same id, got %q and %q", a.ID, b.ID)
	}
}

func TestLedgerSummary(t *testing.T) {
	server := NewServer(ledger.NewMemoryStore())
	events := [][]byte{
		[]byte(`{"eventType":"manual_topup","accountId":"acct_1","sourceEventId":"topup_1","amountCents":1000,"currency":"CNY"}`),
		[]byte(`{"eventType":"compute_debit","accountId":"acct_1","sourceEventId":"debit_1","amountCents":-390,"currency":"CNY"}`),
	}
	for _, event := range events {
		rec := httptest.NewRecorder()
		server.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/ledger/entries", bytes.NewReader(event)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("append status = %d body=%s", rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/summary?accountId=acct_1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("summary status = %d body=%s", rec.Code, rec.Body.String())
	}
	var summary ledger.Summary
	_ = json.Unmarshal(rec.Body.Bytes(), &summary)
	if summary.BalanceCents != 610 {
		t.Fatalf("expected balance 610, got %d", summary.BalanceCents)
	}
}
```

- [ ] **Step 2: Run API tests to verify failure**

Run:

```bash
go test ./internal/api
```

Expected: FAIL because package `internal/api` has no `NewServer`.

- [ ] **Step 3: Implement HTTP server**

Create `internal/api/server.go`:

```go
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
		AccountID: q.Get("accountId"),
		UserID: q.Get("userId"),
		WorkspaceID: q.Get("workspaceId"),
		ComputeID: q.Get("computeId"),
		StorageID: q.Get("storageId"),
		AttachmentID: q.Get("attachmentId"),
		SourceEventID: q.Get("sourceEventId"),
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
```

Create `cmd/opl-ledger-api/main.go`:

```go
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/RenDeHuang/OPL-Ledger/internal/api"
	"github.com/RenDeHuang/OPL-Ledger/internal/ledger"
)

func main() {
	addr := ":8788"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	handler := api.NewServer(ledger.NewMemoryStore())
	log.Printf("opl-ledger-api listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 4: Run API tests and full Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api cmd/opl-ledger-api
git commit -m "feat: add ledger http api baseline"
```

---

### Task 6: Tencent Reconciliation Primitive

**Files:**
- Create: `internal/reconciliation/tencent.go`
- Create: `internal/reconciliation/tencent_test.go`

- [ ] **Step 1: Write failing reconciliation tests**

Create `internal/reconciliation/tencent_test.go`:

```go
package reconciliation

import "testing"

func TestTencentReconciliationPassesWhenLedgerCoversCostPlusMarkup(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate: 0.20,
		LedgerRows: []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1200}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "pass" {
		t.Fatalf("expected pass, got %s diff=%d", report.Status, report.DifferenceCents)
	}
}

func TestTencentReconciliationFailsWhenLedgerUndercharges(t *testing.T) {
	report := ReconcileTencentBills(Input{
		MarkupRate: 0.20,
		LedgerRows: []LedgerRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
		TencentRows: []TencentRow{{WorkspaceID: "ws_1", ResourceType: "compute", AmountCents: 1000}},
	})
	if report.Status != "fail" {
		t.Fatalf("expected fail, got %s", report.Status)
	}
	if report.DifferenceCents != -200 {
		t.Fatalf("expected -200 difference, got %d", report.DifferenceCents)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:

```bash
go test ./internal/reconciliation
```

Expected: FAIL because reconciliation types/functions are undefined.

- [ ] **Step 3: Implement reconciliation**

Create `internal/reconciliation/tencent.go`:

```go
package reconciliation

import "math"

type LedgerRow struct {
	WorkspaceID  string
	ResourceType string
	AmountCents  int64
}

type TencentRow struct {
	WorkspaceID  string
	ResourceType string
	AmountCents  int64
}

type Input struct {
	MarkupRate  float64
	LedgerRows  []LedgerRow
	TencentRows []TencentRow
}

type Report struct {
	Status              string `json:"status"`
	LedgerAmountCents   int64  `json:"ledgerAmountCents"`
	ExpectedAmountCents int64  `json:"expectedAmountCents"`
	DifferenceCents     int64  `json:"differenceCents"`
}

func ReconcileTencentBills(input Input) Report {
	var ledgerTotal int64
	for _, row := range input.LedgerRows {
		ledgerTotal += row.AmountCents
	}
	var tencentTotal int64
	for _, row := range input.TencentRows {
		tencentTotal += row.AmountCents
	}
	expected := int64(math.Round(float64(tencentTotal) * (1 + input.MarkupRate)))
	diff := ledgerTotal - expected
	status := "pass"
	if diff < 0 {
		status = "fail"
	}
	return Report{
		Status: status,
		LedgerAmountCents: ledgerTotal,
		ExpectedAmountCents: expected,
		DifferenceCents: diff,
	}
}
```

- [ ] **Step 4: Run reconciliation tests**

Run:

```bash
go test ./internal/reconciliation
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/reconciliation
git commit -m "feat: add tencent reconciliation primitive"
```

---

### Task 7: Read-Only Kubernetes Evidence Collector

**Files:**
- Create: `internal/k8s/evidence.go`
- Create: `internal/k8s/evidence_test.go`
- Modify: `go.mod`

- [ ] **Step 1: Add client-go dependencies**

Run:

```bash
go get k8s.io/client-go@v0.30.0 k8s.io/api@v0.30.0 k8s.io/apimachinery@v0.30.0
```

Expected: `go.mod` and `go.sum` include Kubernetes dependencies.

- [ ] **Step 2: Write failing fake client test**

Create `internal/k8s/evidence_test.go`:

```go
package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectDeploymentEvidenceReadsStatusWithoutSecrets(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "opl-ws-1",
				Namespace: "opl-cloud",
				ResourceVersion: "42",
				Labels: map[string]string{"oplcloud.cn/workspace-id": "ws_1"},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Replicas: 1, ObservedGeneration: 7},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "opl-ws-1-token", Namespace: "opl-cloud"},
			Data: map[string][]byte{"token": []byte("secret-value")},
		},
	)
	collector := NewCollector(client)
	snapshot, err := collector.CollectDeployment(context.Background(), "cluster_1", "opl-cloud", "opl-ws-1")
	if err != nil {
		t.Fatalf("collect deployment: %v", err)
	}
	if snapshot.ReadinessStatus != "ready" {
		t.Fatalf("expected ready, got %s", snapshot.ReadinessStatus)
	}
	if snapshot.WorkspaceID != "ws_1" {
		t.Fatalf("expected ws_1, got %s", snapshot.WorkspaceID)
	}
	if snapshot.RedactedObject["secretData"] != nil {
		t.Fatalf("snapshot leaked secret data")
	}
}
```

- [ ] **Step 3: Run test to verify failure**

Run:

```bash
go test ./internal/k8s
```

Expected: FAIL because `NewCollector` is undefined.

- [ ] **Step 4: Implement collector**

Create `internal/k8s/evidence.go`:

```go
package k8s

import (
	"context"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Snapshot struct {
	ClusterID          string         `json:"clusterId"`
	Namespace          string         `json:"namespace"`
	ObjectKind         string         `json:"objectKind"`
	ObjectName         string         `json:"objectName"`
	WorkspaceID        string         `json:"workspaceId,omitempty"`
	ResourceVersion    string         `json:"resourceVersion"`
	ObservedGeneration int64          `json:"observedGeneration"`
	ReadinessStatus    string         `json:"readinessStatus"`
	CollectedAt        time.Time      `json:"collectedAt"`
	RedactedObject     map[string]any `json:"redactedObject"`
}

type Collector struct {
	client kubernetes.Interface
}

func NewCollector(client kubernetes.Interface) *Collector {
	return &Collector{client: client}
}

func (c *Collector) CollectDeployment(ctx context.Context, clusterID string, namespace string, name string) (Snapshot, error) {
	deployment, err := c.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	status := "not_ready"
	if deployment.Status.ReadyReplicas >= 1 && deployment.Status.ReadyReplicas == deployment.Status.Replicas {
		status = "ready"
	}
	labels := map[string]string{}
	for key, value := range deployment.Labels {
		labels[key] = value
	}
	return Snapshot{
		ClusterID: clusterID,
		Namespace: namespace,
		ObjectKind: "Deployment",
		ObjectName: name,
		WorkspaceID: labels["oplcloud.cn/workspace-id"],
		ResourceVersion: deployment.ResourceVersion,
		ObservedGeneration: deployment.Status.ObservedGeneration,
		ReadinessStatus: status,
		CollectedAt: time.Now().UTC(),
		RedactedObject: map[string]any{
			"apiVersion": "apps/v1",
			"kind": "Deployment",
			"name": deployment.Name,
			"namespace": deployment.Namespace,
			"labels": labels,
			"readyReplicas": deployment.Status.ReadyReplicas,
			"replicas": deployment.Status.Replicas,
		},
	}, nil
}
```

- [ ] **Step 5: Run collector tests**

Run:

```bash
go test ./internal/k8s
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/k8s
git commit -m "feat: add read only kubernetes evidence collector"
```

---

### Task 8: Worker Entrypoint

**Files:**
- Create: `cmd/opl-ledger-worker/main.go`
- Test: `go test ./...`

- [ ] **Step 1: Create worker entrypoint**

Create `cmd/opl-ledger-worker/main.go`:

```go
package main

import (
	"log"

	"github.com/RenDeHuang/OPL-Ledger/internal/version"
)

func main() {
	log.Printf("%s worker %s ready; scheduled reconciliation and evidence jobs are outside the baseline scope", version.ServiceName, version.APIVersion)
}
```

- [ ] **Step 2: Run all Go tests**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/opl-ledger-worker
git commit -m "feat: add ledger worker entrypoint"
```

---

### Task 9: React + TypeScript Operator UI Baseline

**Files:**
- Create: `web/package.json`
- Create: `web/tsconfig.json`
- Create: `web/vite.config.ts`
- Create: `web/index.html`
- Create: `web/src/App.tsx`
- Create: `web/src/App.test.tsx`
- Create: `web/src/main.tsx`
- Create: `web/src/styles.css`

- [ ] **Step 1: Create frontend package**

Create `web/package.json`:

```json
{
  "name": "opl-ledger-web",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "vite --host 127.0.0.1",
    "build": "tsc && vite build",
    "test": "vitest run"
  },
  "dependencies": {
    "@vitejs/plugin-react": "^4.3.4",
    "vite": "^6.0.7",
    "typescript": "^5.7.2",
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "lucide-react": "^0.468.0"
  },
  "devDependencies": {
    "@testing-library/react": "^16.1.0",
    "@testing-library/jest-dom": "^6.6.3",
    "jsdom": "^25.0.1",
    "vitest": "^2.1.8"
  }
}
```

Create `web/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForClassFields": true,
    "lib": ["DOM", "DOM.Iterable", "ES2020"],
    "allowJs": false,
    "skipLibCheck": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "forceConsistentCasingInFileNames": true,
    "module": "ESNext",
    "moduleResolution": "Node",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "react-jsx"
  },
  "include": ["src"],
  "references": []
}
```

Create `web/vite.config.ts`:

```ts
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: true
  }
});
```

Create `web/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>OPL Ledger</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

- [ ] **Step 2: Write failing UI smoke test**

Create `web/src/App.test.tsx`:

```tsx
import '@testing-library/jest-dom/vitest';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { App } from './App';

describe('App', () => {
  it('renders Ledger operator surfaces', () => {
    render(<App />);
    expect(screen.getByRole('heading', { name: 'OPL Ledger' })).toBeInTheDocument();
    expect(screen.getByText('Billing ledger')).toBeInTheDocument();
    expect(screen.getByText('Evidence records')).toBeInTheDocument();
    expect(screen.getByText('Tencent reconciliation')).toBeInTheDocument();
    expect(screen.getByText('Kubernetes evidence')).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run UI test to verify failure**

Run:

```bash
npm install --prefix web
npm test --prefix web
```

Expected: FAIL because `web/src/App.tsx` does not exist.

- [ ] **Step 4: Implement UI shell**

Create `web/src/App.tsx`:

```tsx
import { Activity, ClipboardList, Database, ShieldCheck } from 'lucide-react';
import './styles.css';

const surfaces = [
  { title: 'Billing ledger', detail: 'Append-first money movement and usage records.', icon: Database },
  { title: 'Evidence records', detail: 'Receipts, audit events, and task evidence.', icon: ClipboardList },
  { title: 'Tencent reconciliation', detail: 'Compare OPL debits against Tencent cost plus markup.', icon: ShieldCheck },
  { title: 'Kubernetes evidence', detail: 'Read-only runtime snapshots collected through client-go.', icon: Activity }
];

export function App() {
  return (
    <main className="shell">
      <header className="topbar">
        <div>
          <p className="eyebrow">Operator console</p>
          <h1>OPL Ledger</h1>
        </div>
        <span className="status">Standalone baseline</span>
      </header>
      <section className="grid" aria-label="Ledger surfaces">
        {surfaces.map((surface) => {
          const Icon = surface.icon;
          return (
            <article className="surface" key={surface.title}>
              <Icon aria-hidden="true" size={22} />
              <h2>{surface.title}</h2>
              <p>{surface.detail}</p>
            </article>
          );
        })}
      </section>
    </main>
  );
}
```

Create `web/src/main.tsx`:

```tsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';

createRoot(document.getElementById('root') as HTMLElement).render(
  <StrictMode>
    <App />
  </StrictMode>
);
```

Create `web/src/styles.css`:

```css
:root {
  color: #18202b;
  background: #f5f7fb;
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

body {
  margin: 0;
}

.shell {
  min-height: 100vh;
  padding: 32px;
}

.topbar {
  align-items: center;
  display: flex;
  justify-content: space-between;
  gap: 24px;
  margin: 0 auto 28px;
  max-width: 1120px;
}

.eyebrow {
  color: #5b6675;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0;
  margin: 0 0 6px;
  text-transform: uppercase;
}

h1 {
  font-size: 34px;
  line-height: 1.1;
  margin: 0;
}

.status {
  background: #e9f8ef;
  border: 1px solid #b6e4c5;
  border-radius: 999px;
  color: #176d38;
  font-size: 14px;
  font-weight: 700;
  padding: 8px 12px;
  white-space: nowrap;
}

.grid {
  display: grid;
  gap: 16px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  margin: 0 auto;
  max-width: 1120px;
}

.surface {
  background: #ffffff;
  border: 1px solid #dfe5ee;
  border-radius: 8px;
  min-height: 150px;
  padding: 18px;
}

.surface svg {
  color: #2563eb;
}

.surface h2 {
  font-size: 17px;
  margin: 14px 0 8px;
}

.surface p {
  color: #526071;
  font-size: 14px;
  line-height: 1.5;
  margin: 0;
}

@media (max-width: 860px) {
  .shell {
    padding: 20px;
  }

  .topbar {
    align-items: flex-start;
    flex-direction: column;
  }

  .grid {
    grid-template-columns: 1fr;
  }
}
```

- [ ] **Step 5: Run frontend tests and build**

Run:

```bash
npm test --prefix web
npm run build --prefix web
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web
git commit -m "feat: add ledger operator ui baseline"
```

---

### Task 10: Final Verification And Documentation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update README with run commands**

Replace `README.md` with:

```markdown
# OPL Ledger

`OPL-Ledger` is the standalone Ledger service split from `RenDeHuang/OPL-Cloud`.

It owns billing ledger events, audit events, evidence records, receipts, idempotency, usage records, Tencent bill reconciliation, and read-only Kubernetes runtime evidence snapshots.

It does not own OPL Console workspace lifecycle screens, OPL Fabric provisioning, OPL Gateway internals, One Person Lab framework internals, or `one-person-lab-app` runtime behavior.

## Stack

- Frontend: React + TypeScript
- Backend: Go
- Database: PostgreSQL
- Kubernetes: Go `client-go`

## Run API

```bash
go run ./cmd/opl-ledger-api
```

The API listens on `:8788` by default. Set `PORT` to override.

Health:

```bash
curl http://127.0.0.1:8788/healthz
```

## Run Frontend

```bash
npm install --prefix web
npm run dev --prefix web
```

## Verify

```bash
go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```

## Source Boundaries

The first contracts are copied from the original OPL Cloud contract package. JavaScript from OPL Cloud is migration reference only; this repository's backend implementation is Go.
```

- [ ] **Step 2: Run full verification**

Run:

```bash
go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```

Expected: all commands PASS.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document ledger baseline commands"
```

---

## Self-Review Notes

- Spec coverage: this plan covers standalone Ledger positioning, fixed stack, source contracts, PostgreSQL schema, idempotent ledger append/list/summary, Tencent reconciliation, read-only Kubernetes evidence, frontend operator UI, and verification.
- Follow-up plan scope: durable PostgreSQL store implementation, authentication/RBAC, full audit/evidence endpoints, VolumeSnapshot dynamic client support, OpenAPI generation, and OPL Cloud integration changes.
- Boundary check: no task creates Console workspace lifecycle, Fabric provisioning, Gateway internals, or one-person-lab-app behavior.
