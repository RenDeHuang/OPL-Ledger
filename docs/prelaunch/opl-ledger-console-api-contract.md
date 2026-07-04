# OPL Ledger Console API Contract

This document defines the stable API surface that `OPL Console` should call or
read from `OPL Ledger`. Console owns user operations, organization/account
context, billing pages, operator actions, and payment UI. Ledger owns wallet
state, accounting entries, wallet transactions, top-up records, audit records,
and reconciliation evidence.

Production traffic remains on `medopl-3` / `OPL-Cloud` until explicit cutover.
These APIs are the pre-cutover contract for integration and shadow mode.

## Rules

- Console must not mutate wallet balances directly.
- Console sends integer cents only; no floating point money values.
- Mutating calls use `Authorization: Bearer <OPL_LEDGER_SERVICE_TOKEN>`.
- Operator reads use `Authorization: Bearer <OPL_LEDGER_ADMIN_TOKEN>`.
- Every mutating call has a stable `sourceEventId`.
- Exact replay returns `200 OK` and the original accounting result.
- Conflicting replay returns `409 Conflict`.
- Ledger returns complete accounting objects so Console can show audit trails
  without reconstructing them.

## Manual Top-Up

### `POST /api/v1/billing/topups`

Purpose: operator/admin wallet credit.

Caller: Console admin operation or future payment-credit worker after payment
success has been verified.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "amountCents": 25000,
  "currency": "CNY",
  "sourceEventId": "console_manual_topup_20260704_0001",
  "reason": "initial launch credit",
  "operatorUserId": "usr_admin",
  "operatorAccountId": "acct_admin"
}
```

Response:

```json
{
  "created": true,
  "wallet": {},
  "entry": {},
  "transaction": {},
  "topUp": {},
  "auditEvent": {}
}
```

Ledger guarantees:

- `wallets.balance_cents` and `wallets.total_recharged_cents` increase.
- `ledger_entries` receives a `credit` row.
- `wallet_transactions` receives a `credit` row linked to the ledger entry.
- `manual_topups` stores ledger, wallet transaction, and audit ids.
- `audit_events` receives `account.credit_granted`.
- PostgreSQL writes happen in one SQL transaction.

Console behavior:

- Show `reason` as operator-facing text.
- Use `sourceEventId` as replay identity, not display text.
- Treat `409 Conflict` as a serious operator workflow error.

## Payment-Credit Integration

Ledger does not need to own checkout UX in the first integration phase. Console
or a payment worker may own payment order creation and provider webhooks. After
provider signature verification and successful payment state are recorded, the
payment worker calls the same top-up endpoint.

Recommended source event format:

```text
payment:<provider>:<provider_order_id>:credit
```

Recommended request payload:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "amountCents": 100000,
  "currency": "CNY",
  "sourceEventId": "payment:stripe:pi_123:credit",
  "reason": "Stripe payment pi_123",
  "operatorUserId": "system_payment",
  "operatorAccountId": "system"
}
```

Borrowed from the `sub2api` pattern:

- payment order state and wallet credit are separate steps.
- webhook verification happens before Ledger is called.
- payment success with Ledger credit failure is retried with the same
  `sourceEventId`.
- the credit endpoint is idempotent.

OPL-specific constraint:

- Ledger records integer cents only. Payment providers may use decimals, but the
  integration layer must convert and approve cents before calling Ledger.

## Wallet Reads

### `GET /api/v1/billing/wallets`

Purpose: Console billing overview and admin account page.

Query filters:

- `accountId`
- `userId`

Response:

```json
[
  {
    "userId": "usr_1",
    "accountId": "acct_1",
    "balanceCents": 25000,
    "frozenCents": 6000,
    "availableCents": 19000,
    "holds": {
      "compute": 5000,
      "storage": 1000
    },
    "totalRechargedCents": 25000
  }
]
```

Console behavior:

- Display `availableCents` as the spendable balance.
- Display `frozenCents` and `holds` as reserved funds for active resources.
- Do not calculate available balance independently except as a UI consistency
  check.

## Wallet Transaction Reads

### `GET /api/v1/billing/wallet-transactions`

Purpose: Console billing history, admin audit, and customer support.

Query filters:

- `accountId`
- `userId`
- `workspaceId`
- `type`: `credit`, `hold`, `debit`, `hold_release`, `adjustment`
- `sourceEventId`
- `ledgerEntryId`
- `usageLogId`
- `fundingSource`

Response item:

```json
{
  "id": "wtx_1",
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "type": "debit",
  "amountCents": -120,
  "currency": "CNY",
  "sourceEventId": "fabric:settlement:compute_1:2026070412",
  "ledgerEntryId": "led_1",
  "usageLogId": "usage_1",
  "fundingSource": "available_balance",
  "balanceBeforeCents": 25000,
  "balanceAfterCents": 24880,
  "frozenBeforeCents": 6000,
  "frozenAfterCents": 6000,
  "availableAfterCents": 18880,
  "createdAt": "2026-07-04T12:00:00Z"
}
```

## Manual Top-Up Reads

### `GET /api/v1/billing/topups`

Purpose: operator top-up history and migration reconciliation.

Query filters:

- `accountId`
- `userId`
- `operatorUserId`
- `operatorAccountId`
- `sourceEventId`
- `status`

Console behavior:

- Show top-ups as operator/payment credit records.
- Link each top-up to its wallet transaction, ledger entry, and audit event.

## Request Usage Reads

### `GET /api/v1/billing/request-usage`

Purpose: Console usage pages for Gateway/API calls.

Query filters:

- `accountId`
- `userId`
- `workspaceId`
- `requestId`
- `sourceEventId`
- `requestFingerprint`
- `ledgerEntryId`
- `provider`
- `model`

Console behavior:

- Treat request usage logs as immutable usage evidence.
- Use wallet transactions for money movement display.

## Resource Usage Reads

### `GET /api/v1/billing/resource-usage`

Purpose: Console resource billing pages for compute/storage usage.

Query filters:

- `accountId`
- `userId`
- `workspaceId`
- `computeId`
- `storageId`
- `attachmentId`
- `kind`
- `sourceEventId`

Console behavior:

- Show resource usage logs as usage evidence.
- Show wallet transactions for actual debit/hold/release movement.

## Request Quota Management

### `PUT /api/v1/billing/request-quotas`

Purpose: Console/admin configures account or workspace request spending limits.

Request:

```json
{
  "accountId": "acct_1",
  "userId": "usr_1",
  "workspaceId": "ws_1",
  "dailyLimitCents": 50000,
  "monthlyLimitCents": 1000000,
  "sourceEventId": "console:quota:acct_1:ws_1:20260704"
}
```

### `GET /api/v1/billing/request-quotas`

Purpose: Console/admin reads quota configuration and current usage windows.

Query filters:

- `accountId`
- `userId`
- `workspaceId`

## Audit Reads

### `GET /api/v1/audit/events`

Purpose: operator review and compliance investigation.

Query filters:

- `accountId`
- `workspaceId`
- `action`
- `targetKind`
- `targetId`
- `sourceEventId`

Console behavior:

- Use audit records for operator-facing accountability.
- Do not infer billing state from audit records alone.

## Reconciliation Reads

### `GET /api/v1/billing/reconciliation`

Purpose: admin list of provider reconciliation reports.

### `GET /api/v1/billing/reconciliation/latest`

Purpose: latest provider reconciliation status.

### `GET /api/v1/billing/reconciliation/guard`

Purpose: Console preflight gate before allowing expensive operations.

Console behavior:

- If guard says blocked, show the reason and do not proceed to resource
  creation.
- Console should not override reconciliation guard without explicit operator
  approval outside normal user flows.

## Console Does Not Call

Console should not directly call low-level Fabric resource lifecycle APIs in
Ledger unless it is acting as the lifecycle coordinator for a specific product
flow. In normal operation:

```text
Console user action -> Fabric lifecycle API -> Ledger Fabric-facing billing API
```

Console remains the UI and policy surface. Fabric remains the executor. Ledger
remains the accounting and evidence service.
