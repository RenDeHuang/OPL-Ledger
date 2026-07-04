# OPL Ledger API Contract

Pre-launch contract for `OPL-Cloud` / `medopl-3` consumers. Production traffic remains on `medopl-3` until explicit cutover.

## Mutating Endpoint Rules

- All mutating endpoints must be idempotent.
- `sourceEventId`, `requestFingerprint`, or endpoint-specific source fields are part of the replay contract.
- Exact replay returns `200 OK` and the existing record.
- Conflicting replay returns `409 Conflict`.
- Amounts are integer cents in `CNY`; no floating point money values.

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

### `GET /api/v1/billing/reconciliation/latest`

Purpose: return latest stored reconciliation report.

### `POST /api/v1/ledger/task-receipts`

Purpose: record task evidence receipt.

Status: implemented with PostgreSQL idempotency for `accountId + workspaceId + taskId + sourceEventId`.

Idempotency: when `sourceEventId` is supplied, exact replay returns the existing receipt. Replay with the same idempotency tuple and different payload returns `409 Conflict`.

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
- Wallet transaction list API.
- Compute/storage hold create/release APIs. Pricing, hold creation, and hold release business rules are implemented locally; API/PostgreSQL transaction wiring is still planned.
- Hourly settlement API. Core compute/storage debit calculation, available-balance-first charging, hold capture, hold-exhaustion intents, and no-negative-balance rules are implemented locally; API/PostgreSQL transaction wiring is still planned.
- Resource usage log API/store wiring. Compute and storage resource usage log shapes are implemented locally with workspace/resource ids.
- Reconciliation guard API.
- Kubernetes evidence snapshot API.
