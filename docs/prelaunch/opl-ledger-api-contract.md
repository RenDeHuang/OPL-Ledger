# OPL Ledger API Contract

Pre-launch contract for `OPL-Cloud` / `medopl-3` consumers. Production traffic remains on `medopl-3` until explicit cutover.

## Mutating Endpoint Rules

- All mutating endpoints must be idempotent.
- `sourceEventId`, `requestFingerprint`, or endpoint-specific source fields are part of the replay contract.
- Exact replay returns `200 OK` and the existing record.
- Conflicting replay returns `409 Conflict`.
- Amounts are integer cents in `CNY`; no floating point money values.

## Authentication

- Local/dev mode: if `OPL_LEDGER_SERVICE_TOKEN` or `OPL_LEDGER_ADMIN_TOKEN` is empty, the corresponding route class skips token enforcement.
- Mutating endpoints: when `OPL_LEDGER_SERVICE_TOKEN` is configured, callers must send `Authorization: Bearer <service-token>`.
- Operator evidence reads: when `OPL_LEDGER_ADMIN_TOKEN` is configured, audit, evidence, task receipt, and reconciliation guard reads require `Authorization: Bearer <admin-token>`.
- Missing token returns `401`; invalid token returns `403`.

## Implemented

### `POST /api/v1/billing/topups`

Purpose: manual wallet credit from Console/admin operation.

Source behavior: `/home/dev/medopl-3/packages/console/src/services/billing-service.js#manualTopUp`.

Idempotency: `reason` is used as the top-up source event id. If omitted, the source event id is `owner_credit`.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "amountCents": 25000,
  "reason": "owner_credit_1",
  "operatorUserId": "usr_admin",
  "operatorAccountId": "acct_admin"
}
```

Response:

```json
{
  "created": true,
  "wallet": {
    "userId": "usr_1",
    "accountId": "acct_1",
    "balanceCents": 25000,
    "frozenCents": 0,
    "availableCents": 25000,
    "holds": {},
    "totalRechargedCents": 25000
  },
  "entry": {},
  "transaction": {},
  "topUp": {},
  "auditEvent": {}
}
```

Persistence requirements:

- `wallets` snapshot is updated.
- `ledger_entries` receives a `credit` entry.
- `wallet_transactions` receives a `credit` transaction with before/after balances and `ledgerEntryId`.
- `manual_topups` records operator, target account/user, before/after balances, ledger id, wallet transaction id, and audit id.
- `audit_events` records `account.credit_granted`.
- PostgreSQL path performs these writes in one SQL transaction.

### `GET /api/v1/billing/wallet-transactions`

Purpose: list wallet money movement records for operator review, Console billing history, and reconciliation investigation.

Status: implemented for in-memory and PostgreSQL stores.

Authorization: operator evidence read; when `OPL_LEDGER_ADMIN_TOKEN` is configured, callers must send `Authorization: Bearer <admin-token>`.

Filters:

- `accountId`
- `userId`
- `workspaceId`
- `type`: `credit`, `hold`, `debit`, `hold_release`, or `adjustment`
- `sourceEventId`
- `ledgerEntryId`
- `usageLogId`
- `fundingSource`

Response:

```json
[
  {
    "id": "wtx_...",
    "accountId": "acct_1",
    "userId": "usr_1",
    "workspaceId": "ws_1",
    "type": "hold",
    "amountCents": 600,
    "currency": "CNY",
    "sourceEventId": "compute_resource:compute_1:created",
    "ledgerEntryId": "led_...",
    "balanceBeforeCents": 1000,
    "balanceAfterCents": 1000,
    "frozenBeforeCents": 0,
    "frozenAfterCents": 600,
    "availableAfterCents": 400,
    "createdAt": "2026-07-04T12:00:00Z"
  }
]
```

### `POST /api/v1/billing/holds`

Purpose: create a prepaid compute or storage hold before opening or resuming a paid resource.

Status: implemented for compute/storage wallet hold creation, idempotent replay, wallet frozen/available updates, ledger entry, wallet transaction, and PostgreSQL transaction wiring.

Idempotency: `sourceEventId` is the replay key. Exact replay returns the existing hold result with `200 OK`; conflicting replay returns `409 Conflict`.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "holdType": "compute",
  "amountCents": 600,
  "sourceEventId": "compute_resource:compute_1:created",
  "resourceId": "compute_1",
  "packageId": "basic"
}
```

Response:

```json
{
  "created": true,
  "wallet": {
    "accountId": "acct_1",
    "balanceCents": 1000,
    "frozenCents": 600,
    "availableCents": 400,
    "holds": {"compute": 600}
  },
  "entry": {},
  "transaction": {}
}
```

Persistence requirements:

- `wallets.holds` and `wallets.frozen_cents` are updated without changing `balance_cents`.
- `ledger_entries` receives `compute_hold` or `storage_hold`.
- `wallet_transactions` receives a `hold` transaction with before/after frozen and available balances.
- Insufficient available balance returns `400` and writes no ledger, wallet transaction, or wallet mutation.
- PostgreSQL path performs these writes in one SQL transaction.

### `POST /api/v1/billing/holds/release`

Purpose: release prepaid compute/storage holds when a resource is stopped, destroyed, or creation fails.

Status: implemented for compute/storage hold release, idempotent replay, wallet frozen/available updates, ledger entries, wallet transactions, and PostgreSQL transaction wiring.

Idempotency: `sourceEventId` is the replay key for single-hold releases. For multi-hold releases, stored ledger source ids are suffixed with `:<holdType>` to avoid conflicting with the `ledger_entries.source_event_id` uniqueness constraint.

Request:

```json
{
  "accountId": "acct_1",
  "workspaceId": "ws_1",
  "holdTypes": ["compute"],
  "sourceEventId": "compute_resource:compute_1:stopped",
  "computeId": "compute_1",
  "reason": "stop_compute"
}
```

Response:

```json
{
  "created": true,
  "wallet": {
    "accountId": "acct_1",
    "balanceCents": 1000,
    "frozenCents": 0,
    "availableCents": 1000,
    "holds": {"compute": 0}
  },
  "entries": [{}],
  "transactions": [{}]
}
```

Persistence requirements:

- `wallets.holds` and `wallets.frozen_cents` are reduced without changing `balance_cents`.
- Released hold amount writes `compute_hold_released` or `storage_hold_released` ledger entries with negative amount.
- Released hold amount writes `hold_release` wallet transactions with before/after frozen and available balances.
- PostgreSQL path performs these writes in one SQL transaction.

### `POST /api/v1/billing/settlements`

Purpose: settle hourly compute/storage usage against wallet balance and prepaid holds.

Status: implemented for available-balance-first debit, hold capture, no-negative-balance behavior, hold-exhaustion intents, idempotent replay, wallet snapshot update, ledger entries, wallet transactions, and PostgreSQL transaction wiring.

Idempotency: `sourceEventId` identifies the billing tick. Because one tick can produce multiple ledger entries, persisted settlement entries use derived source ids:

- `<sourceEventId>:compute:available_balance`
- `<sourceEventId>:compute:compute_hold`
- `<sourceEventId>:storage:available_balance`
- `<sourceEventId>:storage:storage_hold`

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "computeId": "compute_1",
  "storageId": "storage_1",
  "sourceEventId": "billing_tick_1",
  "hours": 1,
  "computeActive": true,
  "storageActive": true,
  "computeHourlyCents": 500,
  "storageHourlyCents": 1
}
```

Response:

```json
{
  "created": true,
  "wallet": {
    "accountId": "acct_1",
    "balanceCents": 500,
    "frozenCents": 500,
    "availableCents": 0,
    "holds": {"compute": 500}
  },
  "entries": [{}],
  "transactions": [{}],
  "unpaidCents": 0
}
```

Persistence requirements:

- Wallet charge order is available balance first, then matching compute/storage hold.
- Wallet balance never goes below zero; uncovered requested amount is returned as `unpaidCents`.
- Positive charged amount writes `compute_debit` or `storage_debit` ledger entries with negative amount.
- Positive charged amount writes `debit` wallet transactions with `fundingSource` set to `available_balance`, `compute_hold`, or `storage_hold`.
- Exhausted compute hold returns a `compute_auto_stopped` intent.
- Exhausted or unpaid storage returns a `storage_hold_exhausted` intent.
- PostgreSQL path performs these writes in one SQL transaction.

### `POST /api/v1/billing/resource-usage`

Purpose: persist compute/storage resource usage rows for billing evidence and reconciliation.

Status: implemented for compute/storage usage logs, workspace/resource/attachment ids, source-event idempotency, and PostgreSQL persistence.

Idempotency: `sourceEventId` is the replay key. Exact replay returns `200 OK`; conflicting replay returns `409 Conflict`.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "computeId": "compute_1",
  "resourceKind": "compute",
  "quantity": 1,
  "unit": "hour",
  "unitPriceCents": 47,
  "amountCents": 47,
  "sourceEventId": "resource_usage:compute_1:billing_tick_1"
}
```

Response:

```json
{
  "created": true,
  "log": {
    "id": "usage_res_...",
    "accountId": "acct_1",
    "workspaceId": "ws_1",
    "computeId": "compute_1",
    "resourceKind": "compute",
    "quantity": 1,
    "unit": "hour",
    "unitPriceCents": 47,
    "amountCents": 47,
    "currency": "CNY",
    "sourceEventId": "resource_usage:compute_1:billing_tick_1"
  }
}
```

Persistence requirements:

- `resource_usage_logs` stores account/user/workspace ids, compute/storage/attachment ids, resource kind, quantity, unit, unit price, amount, requested amount, currency, source event, payload, and timestamp.
- `resource_usage_logs.source_event_id` is unique when present.
- Resource usage records are evidence logs; settlement/debit writes remain in settlement and wallet transaction endpoints.

### `POST /api/v1/ledger/entries`

Purpose: append low-level ledger entry.

Status: implemented as a compatibility primitive. Prefer domain endpoints for billing workflows.

Idempotency: `sourceEventId` or `requestFingerprint`.

### `GET /api/v1/ledger/entries`

Purpose: list ledger entries by account, user, workspace, resource, or source event.

### `GET /api/v1/ledger/summary`

Purpose: summarize ledger entry amount totals.

### `POST /api/v1/billing/reconciliation`

Purpose: store a Tencent reconciliation report generated from supplied rows.

Normalization: local reconciliation helpers can normalize raw Tencent bill rows using direct `workspaceId` fields or `workspace_id`/`workspaceId` tags, classify compute/storage resources, and fail closed when workspace id is missing or rows mix currencies.

### `GET /api/v1/billing/reconciliation/latest`

Purpose: return latest stored reconciliation report.

### `GET /api/v1/billing/reconciliation/guard`

Purpose: return whether new workspace creation should be blocked by billing reconciliation state.

Default policy: missing report, stale report, or failed report returns `status: "blocked"` and `blockNewWorkspaces: true`. A recent `pass` report returns `status: "ok"`.

Query:

- `maxAgeHours`: optional positive number; defaults to `30`.

Response:

```json
{
  "status": "blocked",
  "blockNewWorkspaces": true,
  "reason": "billing_reconciliation_report_missing",
  "checkedAt": "2026-07-04T12:00:00Z"
}
```

### `POST /api/v1/ledger/task-receipts`

Purpose: record task evidence receipt.

Status: implemented with PostgreSQL idempotency for `accountId + workspaceId + taskId + sourceEventId`.

Idempotency: when `sourceEventId` is supplied, exact replay returns the existing receipt. Replay with the same idempotency tuple and different payload returns `409 Conflict`.

Ownership: when the API server is configured with the Console-provided workspace ownership resolver, `workspaceId` must belong to `accountId`. A missing workspace or mismatched owner returns `404` with `workspace_not_found`, matching the `medopl-3` no-leak behavior. If no resolver is configured, pre-cutover local/dev paths skip this validation.

### `GET /api/v1/ledger/task-receipts`

Purpose: list task receipts by account/workspace/task.

### `POST /api/v1/billing/request-usage`

Purpose: record request usage and debit available wallet balance.

Status: implemented for request usage dedup, quota checks, and available-balance billing.

Source behavior: `/home/dev/medopl-3/packages/console/src/services/billing-service.js#recordRequestUsage`.

Idempotency: `workspaceId + sourceEventId` and `workspaceId + requestId` are replay keys. `requestFingerprint` must match on replay.

Optional request quota input mirrors `medopl-3` user `requestQuota`:

```json
{
  "requestQuota": {
    "limit": 1000,
    "used": 12,
    "windowLimit": 100,
    "windowUsed": 4,
    "windowSeconds": 3600,
    "windowStartedAt": "2026-07-04T12:00:00Z"
  }
}
```

Persistence requirements:

- `requestQuota` is checked before request usage dedup or wallet mutation.
- Quota rejection returns `request_quota_exceeded` and writes no dedup, wallet, ledger, usage, transaction, or audit state.
- `request_usage_dedup` is inserted before wallet mutation.
- `wallets` snapshot is debited from available balance only.
- `request_usage_logs` records requested, charged, and unpaid cents.
- `request_usage_logs.payload` records the incremented quota when a quota was supplied.
- Positive charged amount writes a `request_debit` ledger entry.
- Positive charged amount writes a `debit` wallet transaction linked to the usage log and ledger entry.
- `audit_events` records `billing.request_usage_recorded`.
- PostgreSQL path performs these writes in one SQL transaction.

### `POST /api/v1/audit/events`

Purpose: append an audit event emitted by Console, Fabric, or Ledger-owned billing workflows.

Source behavior: aligns with `medopl-3` audit/evidence trails used by billing and workspace lifecycle operations.

Idempotency: `sourceEventId` identifies the source operation for later query and reconciliation. Exact replay conflict prevention is still planned at the persistence constraint level.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "action": "workspace.created",
  "targetKind": "workspace",
  "targetId": "ws_1",
  "sourceEventId": "console_workspace_create_1",
  "payload": {
    "provider": "tke"
  }
}
```

Response:

```json
{
  "id": "audit_...",
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "action": "workspace.created",
  "targetKind": "workspace",
  "targetId": "ws_1",
  "sourceEventId": "console_workspace_create_1",
  "payload": {},
  "createdAt": "2026-07-04T12:00:00Z"
}
```

Persistence requirements:

- `action` and `targetKind` are required.
- `audit_events` records account, user, workspace, action, target, source event, payload, and timestamp.
- PostgreSQL and in-memory stores support the same append path.

### `GET /api/v1/audit/events`

Purpose: list audit events for operator review, reconciliation, and shadow-mode comparison.

Filters: `accountId`, `userId`, `workspaceId`, `action`, `sourceEventId`.

Response:

```json
[
  {
    "id": "audit_...",
    "accountId": "acct_1",
    "workspaceId": "ws_1",
    "action": "workspace.created",
    "targetKind": "workspace",
    "targetId": "ws_1",
    "sourceEventId": "console_workspace_create_1",
    "payload": {},
    "createdAt": "2026-07-04T12:00:00Z"
  }
]
```

### `POST /api/v1/ledger/evidence-records`

Purpose: append control-plane evidence receipts for Workspace lifecycle, storage backup/restore, access token, and other non-billing provenance events.

Source behavior: `/home/dev/medopl-3/packages/console/src/services/ledger-evidence-service.js#recordEvidence`.

Idempotency: `sourceEventId` is persisted and queryable for shadow-mode comparison. Exact replay conflict prevention is still planned at the persistence constraint level.

Request:

```json
{
  "type": "workspace.created",
  "accountId": "acct_1",
  "workspaceId": "ws_1",
  "sourceEventId": "workspace_create_1",
  "actor": {"type": "user", "id": "usr_1"},
  "plan": {
    "workspaceName": "Lab",
    "packageId": "basic",
    "computeProfile": "basic",
    "storageGb": 50
  },
  "approval": {"status": "implicit_console_policy"},
  "environment": {
    "runtimeProvider": "tencent-tke",
    "workspaceImage": "opl/workspace:latest"
  },
  "resourceRefs": {
    "serverId": "ins_1",
    "storageId": "disk_1"
  },
  "billingRefs": [
    {"id": "hold_1", "type": "compute_hold", "amountCents": 1200, "currency": "CNY"}
  ],
  "continuation": {"type": "open_workspace_url"}
}
```

Response:

```json
{
  "id": "evd_...",
  "type": "workspace.created",
  "accountId": "acct_1",
  "workspaceId": "ws_1",
  "sourceEventId": "workspace_create_1",
  "actor": {},
  "plan": {},
  "approval": {},
  "environment": {},
  "resourceRefs": {},
  "billingRefs": [],
  "inputRefs": [],
  "executionRefs": [],
  "outputRefs": [],
  "reviewResults": [],
  "createdAt": "2026-07-04T12:00:00Z"
}
```

Persistence requirements:

- `type`, `accountId`, `workspaceId`, `plan`, `approval`, and `environment` are required.
- `evidence_records` stores the full evidence receipt payload plus queryable account, workspace, type, source event, and timestamp fields.
- Evidence records do not create `ledger_entries` rows and therefore do not affect billing balances.
- PostgreSQL and in-memory stores support the same append path.

### `GET /api/v1/ledger/evidence-records`

Purpose: list evidence receipts for Console/Fabric shadow-mode comparison and operator review.

Filters: `accountId`, `workspaceId`, `type`, `sourceEventId`.

Response:

```json
[
  {
    "id": "evd_...",
    "type": "workspace.created",
    "accountId": "acct_1",
    "workspaceId": "ws_1",
    "sourceEventId": "workspace_create_1",
    "plan": {},
    "approval": {},
    "environment": {},
    "createdAt": "2026-07-04T12:00:00Z"
  }
]
```

## Planned

- Wallet read API.
- Reconciliation guard API.
- Kubernetes evidence snapshot API. Read-only collector and PostgreSQL persistence primitives are implemented locally; external API wiring is still planned.
