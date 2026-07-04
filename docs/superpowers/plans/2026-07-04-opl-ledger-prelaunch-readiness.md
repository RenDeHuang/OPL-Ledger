# OPL Ledger Pre-Launch Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring `OPL-Ledger` to pre-launch readiness while `OPL-Cloud` / `medopl-3` remains the primary production path.

**Architecture:** Keep Ledger independent from Console and Fabric. `opl-ledger` owns billing truth, wallet money movement evidence, audit, receipts, reconciliation guards, and read-only runtime evidence; Console/Fabric continue owning user operations and provider mutation until explicit cutover.

**Tech Stack:** Go API, PostgreSQL, React + TypeScript operator UI, Go `client-go`, local-only pre-launch verification.

---

## Source Inputs

Read these first before implementation work:

- `/home/dev/medopl-3/.env.example`
- `/home/dev/medopl-3/deploy/tke/opl-cloud-production.env.example`
- `/home/dev/medopl-3/docs/runtime/production-runbook.md`
- `/home/dev/medopl-3/packages/console/src/services/billing-service.js`
- `/home/dev/medopl-3/packages/console/src/services/wallet-service.js`
- `/home/dev/medopl-3/packages/console/src/services/ledger-evidence-service.js`
- `/home/dev/medopl-3/packages/ledger/src/billing-reconciliation.js`
- `/home/dev/medopl-3/packages/ledger/src/task-evidence.js`
- `/home/dev/medopl-3/packages/contracts/opl-cloud-billing-ledger-contract.json`
- `/home/dev/medopl-3/packages/contracts/opl-cloud-evidence-ledger-contract.json`

## Non-Cloud Rule

Do not run cloud deployment or mutation commands from this plan. No `kubectl apply/create/delete`, no image push, no production secret writes, no DNS/TLS/TCR/TKE changes.

## Tasks

### Task 1: Commit Current Baseline

**Files:** current working tree

- [ ] Review `git diff --stat`.
- [ ] Run `go test ./...`.
- [ ] Run `npm test --prefix web`.
- [ ] Run `npm run build --prefix web`.
- [ ] Run `git diff --check`.
- [ ] Commit current PostgreSQL/API baseline with message `feat: prepare ledger postgres api baseline`.

### Task 2: Record Active Source Alignment

**Files:**
- Modify: `docs/alignment/opl-cloud-source.md`

- [ ] Record active `/home/dev/medopl-3` commit hash.
- [ ] Record active `/home/dev/opl-cloud` commit hash.
- [ ] State that `medopl-3` remains production-primary until cutover.

### Task 3: Freeze Pre-Launch Configuration

**Files:**
- Modify: `.env.example`
- Modify: `docs/prelaunch/opl-ledger-prelaunch-readiness.md`

- [ ] Keep `PORT` and `DATABASE_URL` as currently implemented.
- [ ] Keep billing price variables aligned with `medopl-3`.
- [ ] Keep service auth and shadow-mode variables marked as pre-cutover requirements.
- [ ] Confirm no real secret value is present.

### Task 4: Add API Contract Document

**Files:**
- Create: `docs/prelaunch/opl-ledger-api-contract.md`

- [x] Define external API paths.
- [x] Mark each endpoint as `implemented`, `partial`, or `planned`.
- [x] Map each endpoint to the `medopl-3` source behavior.
- [x] Include idempotency requirements for mutating endpoints.

### Task 5: Split Wallet Package

**Files:**
- Create: `internal/wallet/types.go`
- Create: `internal/wallet/wallet_test.go`
- Create: `internal/wallet/wallet.go`

- [x] Test wallet snapshot fields: `balanceCents`, `frozenCents`, `holds`, `availableCents`, `totalRechargedCents`.
- [x] Implement balance/frozen/hold arithmetic without floating point.

### Task 6: Add Wallet PostgreSQL Tables

**Files:**
- Modify: `internal/db/migrations/0001_initial.sql`
- Modify: `internal/db/migrations_test.go`

- [x] Add `wallets`.
- [x] Add indexes by `account_id` and `user_id`.
- [x] Test migration contains wallet fields needed by `medopl-3`.

### Task 7: Implement Complete Manual Top-Up

**Files:**
- Modify: `internal/api/server_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/ledger/types.go`
- Modify: `internal/ledger/postgres_store.go`
- Modify: `internal/ledger/memory_store.go`

- [x] Test top-up writes wallet snapshot, ledger credit, wallet transaction, manual topup, and audit event.
- [x] Test replay by source event does not double-credit.
- [x] Implement one SQL transaction.

### Task 8: Add Wallet Transactions

**Files:**
- Modify: `internal/db/migrations/0001_initial.sql`
- Create: `internal/wallet/transactions_test.go`
- Create: `internal/wallet/transactions.go`

- [x] Test before/after balance and frozen fields.
- [x] Test transaction links to ledger entry id.
- [x] Test transaction type set includes credit, hold, debit, hold release, adjustment.

### Task 9: Implement Request Usage Dedup

**Files:**
- Modify: `internal/api/server_test.go`
- Modify: `internal/ledger/postgres_store.go`
- Modify: `internal/db/migrations/0001_initial.sql`

- [x] Test same request fingerprint returns existing usage row.
- [x] Test conflicting replay returns conflict.
- [x] Test request usage writes dedup row before wallet mutation.

### Task 10: Implement Request Quota Checks

**Files:**
- Create: `internal/usage/quota_test.go`
- Create: `internal/usage/quota.go`

- [x] Test quota exceeded returns error.
- [x] Test quota rejection does not change wallet, ledger, usage log, or dedup state.

### Task 11: Implement Request Usage Billing Transaction

**Files:**
- Create: `internal/usage/request_usage_test.go`
- Create: `internal/usage/request_usage.go`

- [x] Test successful request writes request usage log, request debit ledger entry, wallet transaction, and audit event.
- [x] Test amount is bounded by available balance rules.

### Task 12: Implement Prepaid Hold Calculation

**Files:**
- Create: `internal/billing/pricing_test.go`
- Create: `internal/billing/pricing.go`

- [x] Test 7-day compute and storage hold using `OPL_BILLING_MARKUP`.
- [x] Match `medopl-3` expected values for basic and pro packages.

### Task 13: Implement Hold Creation

**Files:**
- Create: `internal/billing/holds_test.go`
- Create: `internal/billing/holds.go`

- [x] Test compute hold and storage hold require sufficient available balance.
- [x] Test hold rows write ledger entries and wallet transactions.

### Task 14: Implement Settlement

**Files:**
- Create: `internal/billing/settlement_test.go`
- Create: `internal/billing/settlement.go`
- Modify: `internal/api/server.go`

- [x] Test hourly compute debit.
- [x] Test hourly storage debit.
- [x] Test repeated source event returns existing entries.

### Task 15: Enforce Debit Ordering

**Files:**
- Modify: `internal/billing/settlement_test.go`
- Modify: `internal/billing/settlement.go`

- [x] Test available balance is charged before frozen hold.
- [x] Test wallet balance never goes below zero.

### Task 16: Implement Hold Exhaustion Results

**Files:**
- Modify: `internal/billing/settlement_test.go`
- Modify: `internal/billing/settlement.go`

- [x] Test compute hold exhaustion returns `compute_auto_stopped` action intent.
- [x] Test storage hold exhaustion returns `storage_hold_exhausted` state intent.

### Task 17: Implement Hold Release

**Files:**
- Create: `internal/billing/hold_release_test.go`
- Modify: `internal/billing/holds.go`

- [x] Test stop compute releases compute hold only.
- [x] Test destroy storage releases compute and storage holds when applicable.
- [x] Test create failure releases holds.

### Task 18: Implement Resource Usage Logs

**Files:**
- Create: `internal/usage/resource_usage_test.go`
- Create: `internal/usage/resource_usage.go`

- [x] Test compute usage log carries `computeId` and `workspaceId`.
- [x] Test storage usage log carries `storageId`, `attachmentId`, and `workspaceId`.

### Task 19: Implement Audit Event Store

**Files:**
- Create: `internal/audit/audit_test.go`
- Create: `internal/audit/audit.go`
- Modify: `internal/api/server.go`

- [x] Test append audit event.
- [x] Test list by account, workspace, type, and source event.

### Task 20: Implement Evidence Record Store

**Files:**
- Create: `internal/evidence/evidence_test.go`
- Create: `internal/evidence/evidence.go`
- Modify: `internal/api/server.go`

- [x] Test workspace lifecycle evidence records.
- [x] Test evidence does not appear in billing ledger.

### Task 21: Complete Task Receipt Idempotency

**Files:**
- Modify: `internal/ledger/postgres_store_test.go`
- Modify: `internal/ledger/postgres_store.go`

- [x] Test same `accountId/workspaceId/taskId/sourceEventId` returns existing receipt.
- [x] Test conflicting task receipt returns conflict.

### Task 22: Add Workspace Ownership Validation Hook

**Files:**
- Create: `internal/ownership/ownership_test.go`
- Create: `internal/ownership/ownership.go`

- [x] Define interface for Console-provided workspace ownership.
- [x] Test task receipt rejects workspace owned by another account.

### Task 23: Complete Tencent Bill Normalization

**Files:**
- Modify: `internal/reconciliation/tencent_test.go`
- Modify: `internal/reconciliation/tencent.go`

- [x] Test raw Tencent rows with `workspace_id` tag.
- [x] Test missing workspace id fails closed.
- [x] Test mixed currency fails closed.

### Task 24: Implement Reconciliation Guard API

**Files:**
- Modify: `internal/api/server_test.go`
- Modify: `internal/api/server.go`
- Modify: `internal/ledger/types.go`

- [x] Test missing report blocks new workspaces.
- [x] Test stale report blocks new workspaces.
- [x] Test failed report blocks new workspaces.
- [x] Test passing recent report allows new workspaces.

### Task 25: Persist Kubernetes Evidence Snapshots

**Files:**
- Modify: `internal/k8s/evidence_test.go`
- Modify: `internal/k8s/evidence.go`
- Modify: `internal/ledger/postgres_store.go`

- [x] Test Deployment snapshot stores redacted object.
- [x] Test secret values are never persisted.

### Task 26: Add Service Authentication

**Files:**
- Create: `internal/auth/auth_test.go`
- Create: `internal/auth/auth.go`
- Modify: `internal/api/server.go`

- [x] Test missing token rejects mutating endpoints.
- [x] Test service token allows Console/Fabric calls.
- [x] Test admin token allows operator evidence reads.

### Task 27: Add Shadow Mode Comparison Tool

**Files:**
- Create: `tools/compare-opl-cloud-ledger.md`
- Create: `docs/prelaunch/shadow-mode.md`

- [x] Document local-only comparison between OPL Cloud state and OPL Ledger.
- [x] Do not call production by default.

### Task 28: Add Data Migration Dry-Run Plan

**Files:**
- Create: `docs/prelaunch/data-migration-dry-run.md`

- [ ] Map `medopl-3` billing ledger rows to `opl-ledger` rows.
- [ ] Map wallet transactions, manual topups, request usage, resource usage, audit, and evidence.
- [ ] Require local dry-run output before any real migration.

### Task 29: Add Cutover and Rollback Checklists

**Files:**
- Create: `docs/prelaunch/cutover-checklist.md`
- Create: `docs/prelaunch/rollback-checklist.md`

- [ ] Define cutover gates.
- [ ] Define rollback to OPL Cloud billing path.
- [ ] Require explicit approval for production changes.

### Task 30: Add Local Pre-Launch Verification

**Files:**
- Modify: `README.md`
- Create: `docs/prelaunch/local-verification.md`

- [ ] Document `go test ./...`.
- [ ] Document `npm test --prefix web`.
- [ ] Document `npm run build --prefix web`.
- [ ] Document local PostgreSQL verification with `DATABASE_URL`.
- [ ] Document that no cloud upload/deploy is part of pre-launch.

## Verification

Run after each task group:

```bash
go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```
