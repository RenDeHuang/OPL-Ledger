# OPL Ledger Fabric API Contract

This document defines the stable API surface that `OPL Fabric` should call on
`OPL Ledger`. Fabric owns resource execution: compute, storage, attachments,
provider tasks, and runtime evidence. Ledger owns charging decisions, wallet
holds, settlements, wallet transactions, audit records, and evidence receipts.

Production execution remains on `medopl-3` / `OPL-Cloud` until explicit cutover.
These APIs are for pre-cutover integration and shadow mode.

## Rules

- Fabric never mutates wallet state directly.
- Fabric never decides whether an account can spend money. It asks Ledger.
- Fabric sends stable `sourceEventId` values for every mutating call.
- Fabric sends integer cents only.
- Fabric calls mutating endpoints with
  `Authorization: Bearer <OPL_LEDGER_SERVICE_TOKEN>`.
- Exact replay returns `200 OK`; conflicting replay returns `409 Conflict`.
- Ledger can return action intents such as `stop_compute` or `freeze_storage`.
  Fabric is responsible for executing those intents.

## Event Identity

Recommended source event id format:

```text
fabric:<resource_kind>:<resource_id>:<event_name>:<event_clock>
```

Examples:

```text
fabric:compute:compute_1:create_requested
fabric:compute:compute_1:create_failed
fabric:compute:compute_1:created
fabric:compute:compute_1:settlement:2026070412
fabric:storage:vol_1:created
fabric:storage:vol_1:destroyed
```

## Resource Preflight and Hold

### Canonical route: `POST /api/v1/billing/holds`

### Fabric route: `POST /api/v1/fabric/resource-preflight`

Purpose: reserve spend before Fabric opens or resumes a paid resource.

Fabric-facing semantic name:

```text
resource_preflight_and_hold
```

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "holdType": "compute",
  "amountCents": 20160,
  "sourceEventId": "fabric:compute:compute_1:create_requested",
  "resourceId": "compute_1",
  "packageId": "basic"
}
```

Response:

```json
{
  "created": true,
  "wallet": {},
  "entry": {},
  "transaction": {}
}
```

Ledger guarantees:

- insufficient available balance returns `400` and writes no wallet, ledger, or
  transaction mutation.
- successful hold increases `wallets.frozen_cents`, writes a hold ledger entry,
  and writes a `hold` wallet transaction.
- hold replay does not double-freeze funds.

Fabric behavior:

- call this before creating compute or storage.
- do not create paid resources if Ledger rejects the hold.
- include the hold source event id in later evidence for traceability.

## Resource Create Failed

### Canonical route: `POST /api/v1/billing/holds/release`

### Fabric route: `POST /api/v1/fabric/resource-create-failed`

Purpose: release reserved funds when provider creation fails.

Fabric-facing semantic name:

```text
resource_create_failed_release
```

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "holdTypes": ["compute"],
  "sourceEventId": "fabric:compute:compute_1:create_failed",
  "resourceId": "compute_1",
  "reason": "tencent task failed"
}
```

Ledger guarantees:

- released amount decreases `wallets.frozen_cents`.
- `ledger_entries` receives a hold release event.
- `wallet_transactions` receives `hold_release`.
- replay does not double-release funds.

Fabric behavior:

- call after provider failure is known.
- also write evidence with provider task id and failure reason.

## Resource Created Evidence

### Canonical route: `POST /api/v1/ledger/evidence-records`

### Fabric route: `POST /api/v1/fabric/resource-created`

Purpose: record provider evidence for successful resource creation.

Request:

```json
{
  "accountId": "acct_1",
  "workspaceId": "ws_1",
  "type": "fabric.compute.created",
  "targetKind": "compute",
  "targetId": "compute_1",
  "sourceEventId": "fabric:compute:compute_1:created",
  "payload": {
    "provider": "tencent",
    "providerTaskId": "task_1",
    "packageId": "basic",
    "region": "ap-guangzhou"
  }
}
```

Ledger guarantees:

- evidence records stay separate from billing ledger entries.
- source event replay is idempotent.

Fabric behavior:

- write evidence for successful compute/storage/attachment lifecycle events.
- redact provider secrets before sending payload.

## Resource Usage Log

### Canonical route: `POST /api/v1/billing/resource-usage`

### Fabric route: `POST /api/v1/fabric/resource-usage-tick`

Purpose: record compute/storage usage facts. This is usage evidence, not wallet
movement by itself.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "kind": "compute",
  "computeId": "compute_1",
  "sourceEventId": "fabric:compute:compute_1:usage:2026070412",
  "quantity": 1,
  "unit": "hour",
  "unitPriceCents": 120,
  "amountCents": 120,
  "startedAt": "2026-07-04T12:00:00Z",
  "endedAt": "2026-07-04T13:00:00Z",
  "payload": {
    "packageId": "basic",
    "provider": "tencent"
  }
}
```

Ledger guarantees:

- resource usage is append-only evidence for billing review.
- settlement and wallet debit happen through settlement APIs.

Fabric behavior:

- send one usage log per billable interval.
- include resource ids that let Console show the usage against the workspace.

## Hourly Settlement

### Canonical route: `POST /api/v1/billing/settlements`

### Fabric route: `POST /api/v1/fabric/resource-settlement`

Purpose: charge compute/storage usage against available balance and prepaid
holds.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "computeId": "compute_1",
  "sourceEventId": "fabric:compute:compute_1:settlement:2026070412",
  "usageKind": "compute",
  "amountCents": 120,
  "startedAt": "2026-07-04T12:00:00Z",
  "endedAt": "2026-07-04T13:00:00Z"
}
```

Ledger guarantees:

- available balance is charged first.
- prepaid hold is captured when available balance is insufficient.
- wallet balance never goes negative.
- positive charged amounts write `debit` wallet transactions.
- hold capture writes wallet transactions with funding source `compute_hold` or
  `storage_hold`.
- if funds are exhausted, Ledger returns an action intent.

Example action intent:

```json
{
  "created": true,
  "wallet": {},
  "entries": [],
  "transactions": [],
  "actionIntent": {
    "action": "stop_compute",
    "resourceId": "compute_1",
    "reason": "hold_exhausted"
  }
}
```

Fabric behavior:

- execute action intents promptly.
- report execution evidence back to Ledger.
- do not continue running paid resources after a stop/freeze intent is accepted
  unless Console/operator explicitly overrides the product policy.

## Resource Stopped or Destroyed

### Canonical route: `POST /api/v1/billing/holds/release`

### Fabric routes:

- `POST /api/v1/fabric/resource-stopped`
- `POST /api/v1/fabric/resource-destroyed`

Purpose: release remaining hold when a resource stops or is destroyed.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "holdTypes": ["compute"],
  "sourceEventId": "fabric:compute:compute_1:destroyed",
  "resourceId": "compute_1",
  "reason": "resource destroyed"
}
```

Fabric behavior:

- send release after final settlement for the resource.
- send evidence for the provider stop/destroy result.

## Storage Attachment Events

Storage attachments are evidence and usage inputs. Fabric should send:

- evidence when a volume is attached or detached.
- resource usage logs for billable storage intervals.
- hold and release calls for prepaid storage reservations.

Recommended event types:

```text
fabric.storage.created
fabric.storage.destroyed
fabric.storage.attached
fabric.storage.detached
fabric.storage.snapshot_created
fabric.storage.snapshot_restored
```

## Kubernetes Evidence Snapshots

### Existing route: `POST /api/v1/ledger/kubernetes-evidence-snapshots`

Purpose: record redacted read-only runtime snapshots when Fabric or an operator
collector inspects Kubernetes state.

Fabric behavior:

- use only read-only evidence.
- redact secrets, tokens, image pull credentials, and private env values.
- do not use this endpoint to ask Ledger to apply manifests.

## Reconciliation Input

### Existing route: `POST /api/v1/billing/reconciliation`

Purpose: submit provider billing comparison reports.

Fabric/provider collector behavior:

- normalize Tencent/provider bill rows before submission.
- include provider, status, currency, workspace ids, totals, and mismatches.
- missing workspace tags must fail closed.
- mixed currency reports must fail closed.

## Fabric Does Not Call

Fabric should not call Console-facing manual top-up APIs. Fabric does not own
payment, manual credits, account membership, or operator billing adjustments.

Fabric should not create raw ledger entries directly for resource lifecycle
events when a domain endpoint exists. Use holds, releases, settlements, usage,
audit, and evidence APIs so Ledger can preserve wallet invariants.
