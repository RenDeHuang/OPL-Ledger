# OPL Ledger Split Design

## Goal

Build `RenDeHuang/OPL-Ledger` as the standalone OPL Ledger service split out of the original `RenDeHuang/OPL-Cloud` implementation.

The source product model comes from `one-person-lab-cloud`. The source implementation context is `RenDeHuang/OPL-Cloud`. This repository owns only the Ledger slice: billing truth, audit events, receipts, evidence, idempotency, and reconciliation.

## Positioning

`OPL-Ledger` is not the OPL Console, OPL Fabric, OPL Gateway, or OPL Workspace runtime.

It owns:

- billing ledger events;
- wallet-affecting ledger records and immutable money movement evidence;
- request usage and resource usage records;
- idempotency and deduplication for billable events;
- audit events;
- human-readable and machine-readable receipts;
- control-plane evidence records;
- task evidence receipts;
- Tencent bill reconciliation and reconciliation guards;
- Kubernetes runtime evidence snapshots collected through Go `client-go`.

It does not own:

- user-facing Console workspace lifecycle screens;
- ComputeResource, StorageVolume, StorageAttachment, or Workspace creation;
- Kubernetes resource provisioning;
- Local Docker or Tencent TKE Fabric adapters;
- OPL Gateway provider routing, key management, or quota product surface;
- One Person Lab framework internals;
- `one-person-lab-app` WebUI behavior;
- domain evidence judging, artifact registry, or agent run registry internals.

## Fixed Stack

- Frontend: React + TypeScript.
- Backend: Go.
- Database: PostgreSQL.
- Kubernetes: Go `client-go`.

No Node.js backend or JavaScript service code should be introduced for the new Ledger implementation. JavaScript from the original OPL Cloud repository is migration reference only.

## Source Material

Use these original OPL Cloud areas as migration references:

- `packages/ledger/src/*`: billing reconciliation, evidence ledger helpers, task evidence helpers.
- `packages/contracts/opl-cloud-billing-ledger-contract.json`: billing ledger contract.
- `packages/contracts/opl-cloud-evidence-ledger-contract.json`: evidence ledger contract.
- `packages/contracts/opl-cloud-storage-backup-contract.json`: backup and restore receipt evidence that Ledger must record.
- `packages/contracts/opl-cloud-deployment-contract.json`: deployment verification evidence references.
- `packages/console/src/store.js`: PostgreSQL table intent for ledger, audit, notifications, runtime operations, usage logs, wallet transactions, manual top-ups, and request deduplication.
- `tests/ledger/**`, `tests/billing/**`, and reconciliation-related provider tests: behavioral reference for Go tests.

Do not copy the OPL Console implementation wholesale. Extract the Ledger semantics and rewrite them in the fixed stack.

## Service Shape

The repository should become one Go service plus an optional small operator/admin UI.

```text
cmd/
  opl-ledger-api/
  opl-ledger-worker/
internal/
  api/
  auth/
  billing/
  db/
  evidence/
  k8s/
  ledger/
  reconciliation/
  receipts/
web/
  src/
migrations/
contracts/
docs/
```

`opl-ledger-api` serves HTTP APIs for Console/Fabric/operator consumers.

`opl-ledger-worker` runs asynchronous reconciliation and evidence collection jobs. It may be omitted in the first runnable slice if all jobs are manually triggered through API endpoints, but the package layout should not block a worker later.

## API Boundary

The first API surface should cover:

- append billing event;
- append audit event;
- append evidence record;
- append task receipt;
- record request usage;
- record resource usage;
- record manual top-up evidence;
- list ledger entries by account, user, workspace, compute, storage, attachment, or source event;
- get wallet/account ledger summary;
- submit Tencent bill reconciliation input;
- get latest reconciliation status;
- collect Kubernetes evidence snapshot for a referenced workspace/runtime object.

Every mutating endpoint must be idempotent by a caller-supplied source event id, request fingerprint, or explicit idempotency key.

## PostgreSQL Model

The database should be append-first. Updates are allowed only for derived status rows such as reconciliation report status or evidence collection job status.

Initial tables:

- `ledger_entries`
- `audit_events`
- `evidence_records`
- `task_receipts`
- `request_usage_logs`
- `resource_usage_logs`
- `wallet_transactions`
- `manual_topups`
- `billing_reconciliation_reports`
- `idempotency_keys`
- `kubernetes_evidence_snapshots`

Ledger rows should carry the relevant OPL Cloud identities when present:

- `account_id`
- `user_id`
- `workspace_id`
- `compute_id`
- `storage_id`
- `attachment_id`
- `source_event_id`
- `request_fingerprint`

## Kubernetes Evidence

`client-go` is used only for evidence collection, not provisioning.

Allowed reads:

- Deployment
- Pod
- PVC
- Service
- Ingress
- Event
- Secret metadata only, never secret values
- VolumeSnapshot when the CRD is available

The collected snapshot should store:

- object identity;
- observed generation/resource version;
- relevant status conditions;
- selected labels and annotations;
- readiness/result summary;
- collection time;
- collector version;
- raw redacted JSON for operator evidence.

Secrets must never store decoded values in Ledger.

## Frontend

The React + TypeScript UI is an operator/admin Ledger console, not the Lab Owner workspace console.

Initial views:

- ledger overview;
- account/user/workspace ledger search;
- evidence records;
- task receipts;
- reconciliation reports;
- Kubernetes evidence snapshots;
- audit events.

The UI should expose raw operator evidence only to admin/operator users. Lab Owner commercial UI remains an OPL Console responsibility.

## Migration Strategy

1. Copy only machine-readable contracts needed by Ledger into `contracts/`.
2. Rewrite the Ledger data model in PostgreSQL migrations.
3. Port reconciliation and receipt behavior from JS tests into Go tests.
4. Build the Go API around Ledger append/list/reconcile/evidence operations.
5. Add `client-go` evidence collection behind explicit operator endpoints.
6. Build the React + TypeScript operator UI after the API contracts are stable.
7. Update OPL Cloud later to call OPL Ledger through API instead of importing `packages/ledger`.

## Verification

Minimum verification before claiming the scaffold complete:

```bash
go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```

If the first slice does not yet include the frontend, replace the web commands with a documented skipped verification note and keep `go test ./...` plus `git diff --check` mandatory.

## Open Constraints

- `OPL-Ledger` starts from an empty repository, so the first commit should establish documentation, contracts, and skeleton layout rather than attempt a full feature migration in one diff.
- Original OPL Cloud remains the integration source until Console calls the standalone Ledger API.
- Reconciliation starts with Tencent bill import compatibility because that is the existing product billing guard.
- PostgreSQL is the only production persistence target. JSON file persistence is not part of this repository's target architecture.
