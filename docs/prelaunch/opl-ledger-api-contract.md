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

Status: implemented, idempotency still planned.

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

## Planned

- Wallet read API.
- Wallet transaction list API.
- Compute/storage hold create/release APIs. Pricing, hold creation, and hold release business rules are implemented locally; API/PostgreSQL transaction wiring is still planned.
- Hourly settlement API. Core compute/storage debit calculation, available-balance-first charging, hold capture, hold-exhaustion intents, and no-negative-balance rules are implemented locally; API/PostgreSQL transaction wiring is still planned.
- Resource usage log API/store wiring. Compute and storage resource usage log shapes are implemented locally with workspace/resource ids.
- Reconciliation guard API.
- Audit event append/list API.
- Evidence record append/list API.
- Kubernetes evidence snapshot API.
