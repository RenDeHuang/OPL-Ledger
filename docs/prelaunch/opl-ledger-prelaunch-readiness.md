# OPL Ledger Pre-Launch Readiness

## Status

`OPL-Ledger` is in pre-launch preparation. `OPL-Cloud` / `medopl-3` remains the primary production path until an explicit cutover is approved.

This document is local preparation only. Do not upload images, apply Kubernetes manifests, create cloud resources, write production secrets, or switch production traffic from this repository.

## Source Checked First

The current pre-launch checklist is derived from these `medopl-3` files:

- `/home/dev/medopl-3/.env.example`
- `/home/dev/medopl-3/deploy/tke/opl-cloud-production.env.example`
- `/home/dev/medopl-3/docs/runtime/production-runbook.md`
- `/home/dev/medopl-3/packages/contracts/opl-cloud-billing-ledger-contract.json`
- `/home/dev/medopl-3/packages/contracts/opl-cloud-evidence-ledger-contract.json`
- `/home/dev/medopl-3/packages/contracts/opl-cloud-route-api-contract.json`
- `/home/dev/medopl-3/packages/README.md`

The same boundary also appears in `/home/dev/opl-cloud`.

## Boundary

`opl-ledger` owns:

- billing ledger events;
- wallet money movement evidence;
- wallet transactions;
- manual top-up audit;
- request usage billing and deduplication;
- resource usage logs;
- compute/storage prepaid hold records;
- hold capture and release records;
- hourly settlement evidence;
- reconciliation reports and guard semantics;
- audit events;
- evidence receipts;
- task evidence receipts;
- read-only Kubernetes/runtime evidence snapshots.

`opl-ledger` does not own:

- Console login, organization, membership, or Lab Owner UI;
- Workspace lifecycle orchestration;
- ComputeResource, StorageVolume, StorageAttachment, URL, or backup user actions;
- TKE, Docker, Ingress, PVC, VolumeSnapshot provisioning;
- Gateway routing, model provider internals, API keys, or quota product surface;
- `one-person-lab-app` runtime behavior;
- production deploy workflows before explicit cutover.

Shared boundaries:

- Console owns the user-visible operation and calls Ledger.
- Fabric owns provider-specific resource mutation and returns provider evidence.
- Ledger records billing/audit/evidence and can return guard decisions or action intents.
- Storage backup evidence is shared: Console initiates, Fabric snapshots/restores, Ledger records receipt/evidence.

## Required Configuration

Use `.env.example` as the tracked template. Real values must stay in local ignored files or secret managers.

Currently implemented by the Go API:

- `PORT`
- `DATABASE_URL`

Required for billing parity before cutover:

- `OPL_BILLING_MARKUP`
- `OPL_BASIC_COMPUTE_HOURLY_CNY`
- `OPL_PRO_COMPUTE_HOURLY_CNY`
- `OPL_STORAGE_GB_MONTH_CNY`
- `OPL_RECONCILIATION_MAX_AGE_HOURS`
- `OPL_RECONCILIATION_TOLERANCE_CNY`

Required before Console/Fabric integration:

- `OPL_LEDGER_INTERNAL_URL`
- `OPL_LEDGER_SERVICE_TOKEN`
- `OPL_LEDGER_ADMIN_TOKEN`
- `OPL_LEDGER_SHADOW_MODE`
- `OPL_LEDGER_CUTOVER_ENABLED`
- `OPL_CLOUD_BASE_URL`

Required only for read-only Kubernetes evidence collection:

- `KUBECONFIG`
- `OPL_K8S_NAMESPACE`
- `TENCENT_DEPLOY_CLUSTER_ID`
- `OPL_LEDGER_K8S_READ_ONLY`
- `OPL_WORKSPACE_ID_LABEL`

## Non-Cloud Preparation Rules

Allowed now:

- local tests;
- local PostgreSQL testing;
- docs;
- API/schema work;
- local-only migration tooling;
- read-only inspection of `medopl-3` and `opl-cloud`;
- drafting Kubernetes manifests without applying them.

Forbidden until explicit approval:

- `kubectl apply`, `kubectl create`, `kubectl delete`;
- pushing container images;
- writing production secrets;
- changing production DNS/TLS/TCR/TKE settings;
- switching Console production traffic to `opl-ledger`;
- running production verifier against real cloud resources.

## Current Implementation Snapshot

Implemented:

- Go API entrypoint;
- PostgreSQL migration runner;
- PostgreSQL store for ledger entries, task receipts, and reconciliation reports;
- idempotent ledger entry append/list/summary;
- wallet balance/frozen/hold arithmetic;
- wallet snapshot PostgreSQL table;
- wallet snapshot list API with account/user filters for Console/admin reads;
- wallet transaction list API with account/user/workspace/type/source/ledger/usage/funding filters;
- manual top-up API with wallet snapshot, credit ledger entry, wallet transaction, manual topup record, and audit event in one PostgreSQL transaction;
- manual top-up list API with account/user/operator/source/status filters for Console/admin reads;
- request usage API with quota check, dedup-first PostgreSQL transaction, available-balance debit, usage log, wallet transaction, and audit event;
- request usage list API with account/user/workspace/request/source/fingerprint/ledger/provider/model filters for Console/admin reads;
- persisted request quota management API with account/user/workspace scope and PostgreSQL transaction integration for request usage;
- 7-day compute/storage prepaid hold pricing calculation for Basic and Pro package inputs;
- compute/storage hold create/release API with wallet frozen/available updates, ledger entries, wallet transactions, idempotent replay, and PostgreSQL transaction wiring;
- hourly compute/storage settlement API with available-balance-first debit, hold capture, hold-exhaustion intents, idempotent replay, no-negative-balance behavior, wallet transactions, and PostgreSQL transaction wiring;
- resource usage log API with compute/storage/attachment/workspace ids, source-event idempotency, and PostgreSQL persistence;
- resource usage list API with account/user/workspace/compute/storage/attachment/kind/source filters for Console/admin reads;
- audit events append/list API backed by PostgreSQL and in-memory stores;
- evidence records append/list API backed by PostgreSQL and in-memory stores; evidence records stay out of billing ledger entries;
- task receipt record/query with PostgreSQL `accountId/workspaceId/taskId/sourceEventId` idempotency;
- Console-provided workspace ownership validation hook for task receipt writes;
- reconciliation submit/latest;
- reconciliation report list API with provider/status filters;
- reconciliation guard API for missing, stale, failed, and passing recent reports;
- Tencent reconciliation primitive with raw bill row normalization, workspace tag extraction, missing workspace fail-closed, and mixed-currency fail-closed behavior;
- read-only Deployment evidence collector primitive plus external Kubernetes evidence snapshot append/list API with PostgreSQL persistence for redacted snapshots;
- service-to-service Bearer token authentication for mutating endpoints and admin Bearer token authorization for operator billing/evidence reads when tokens are configured;
- local-only shadow mode comparison procedure and safety rules;
- local data migration dry-run mapping, required report format, and executable wallet/manual-topup preview tool;
- cutover and rollback checklists requiring explicit production approval;
- local pre-launch verification commands and PostgreSQL smoke-test procedure;
- React + TypeScript operator UI baseline.

Not complete:

- Console/Fabric integration;
- executed and approved data migration from current Console state.

## Completion Steps

1. Commit the current PostgreSQL/API baseline in `opl-ledger`.
2. Record the active `medopl-3` source commit used for the pre-launch split.
3. Copy or reference the active route, business object, management wallet, billing, and evidence contracts.
4. Publish a local OpenAPI-style API contract for Ledger consumers.
5. Split Go packages into focused `wallet`, `billing`, `usage`, `audit`, `evidence`, `reconciliation`, and `k8s` units.
6. Add PostgreSQL tables and indexes for wallets, wallet transactions, manual topups, request usage, request dedup, resource usage, audit events, evidence records, and notifications.
7. Implement wallet snapshot persistence: balance, frozen, holds, available, total recharged.
8. Implement complete manual top-up transaction.
9. Implement complete request usage transaction.
10. Implement request quota checks without wallet mutation on rejection.
11. Implement seven-day compute/storage prepaid hold creation.
12. Implement hold sufficiency checks before open/resume.
13. Implement compute/storage hourly settlement.
14. Implement available-balance-first debit, then hold capture.
15. Implement bounded debits so wallet balance never goes negative.
16. Implement compute hold exhausted action intent.
17. Implement storage hold exhausted freeze semantics.
18. Implement hold release for stop/destroy/create-failure paths.
19. Implement resource usage logs with compute/storage/attachment/workspace ids.
20. Implement reconciliation report storage with guard state.
21. Implement reconciliation guard API.
22. Implement raw Tencent bill normalization.
23. Implement audit event append/list APIs.
24. Implement evidence record append/list APIs.
25. Complete task receipt PostgreSQL idempotency and ownership checks.
26. Persist read-only Kubernetes evidence snapshots.
27. Add service-to-service authentication and admin authorization.
28. Add shadow-mode integration docs and local comparison tooling.
29. Add migration tooling from `medopl-3`/`opl-cloud` state into `opl-ledger`.
30. Add cutover checklist, rollback checklist, and production deployment manifest templates, but do not apply them.

## Pre-Cutover Gate

Cutover is blocked until all of these are true:

- `go test ./...` passes;
- `npm test --prefix web` passes;
- `npm run build --prefix web` passes;
- PostgreSQL integration tests pass against a local database;
- manual top-up, request usage, hold, settlement, reconciliation, audit, evidence, and task receipt flows match `medopl-3` behavior;
- Console has run in shadow mode without billing mismatches;
- data migration has been tested locally;
- rollback is documented;
- production secret and cloud changes have explicit human approval.
