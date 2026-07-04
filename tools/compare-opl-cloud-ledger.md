# OPL Cloud to OPL Ledger Local Comparison Tool

This is the local-only shadow-mode comparison procedure for checking whether
`medopl-3` / `OPL-Cloud` state matches the new `OPL-Ledger` records before any
cutover.

## Default Safety Rule

The comparison must not call production by default.

Allowed inputs:

- local `medopl-3` JSON exports;
- local `opl-ledger` API responses from `127.0.0.1` or `localhost`;
- checked-in contract fixtures;
- redacted local PostgreSQL dumps created for dry-run verification.

Forbidden by default:

- production Console URLs;
- production Ledger URLs;
- Tencent Cloud APIs;
- `kubectl` against a real cluster;
- real service/admin tokens in command history or committed files.

## Required Inputs

Prepare these files under an ignored local directory such as
`.local/shadow-mode/`:

- `opl-cloud-wallets.json`
- `opl-cloud-billing-ledger.json`
- `opl-cloud-wallet-transactions.json`
- `opl-cloud-request-usage.json`
- `opl-cloud-resource-usage.json`
- `opl-cloud-audit.json`
- `opl-cloud-evidence.json`
- `opl-ledger-wallets.json`
- `opl-ledger-ledger-entries.json`
- `opl-ledger-request-usage.json`
- `opl-ledger-resource-usage.json`
- `opl-ledger-audit-events.json`
- `opl-ledger-evidence-records.json`

## Comparison Keys

Use stable source keys first:

- manual topups: `sourceEventId` / `reason`;
- request usage: `workspaceId + requestId`, `workspaceId + sourceEventId`, and `requestFingerprint`;
- wallet transactions: `sourceEventId + transactionType + amountCents`;
- resource usage: `workspaceId + resourceKind + sourceEventId` when present;
- audit: `sourceEventId + action + targetKind`;
- evidence: `sourceEventId + type + workspaceId`;
- task receipts: `accountId + workspaceId + taskId + sourceEventId`.

Fallback matching by generated IDs is not acceptable for cutover readiness.

## Required Checks

Report these sections:

- wallet balance parity: `balanceCents`, `frozenCents`, `availableCents`, `holds`, `totalRechargedCents`;
- billing ledger parity: event type, amount cents, currency, account, user, workspace, compute, storage, attachment, source event;
- request usage parity: requested, charged, unpaid, quota payload, request fingerprint;
- resource usage parity: compute/storage identifiers and workspace id;
- audit parity: account, workspace, actor, action, target, source event;
- evidence parity: type, account, workspace, provenance refs, billing refs;
- task receipt parity: idempotency tuple and provenance payload;
- reconciliation parity: latest status, expected amount, ledger amount, difference, guard decision.

## Output Format

Write a local report to `.local/shadow-mode/report.json`:

```json
{
  "checkedAt": "2026-07-04T12:00:00Z",
  "source": "local",
  "status": "pass",
  "mismatches": [],
  "totals": {
    "walletsCompared": 0,
    "ledgerEntriesCompared": 0,
    "auditEventsCompared": 0,
    "evidenceRecordsCompared": 0
  }
}
```

Any mismatch must include:

- `kind`;
- `key`;
- `oplCloud`;
- `oplLedger`;
- `reason`.

## Cutover Gate

Shadow mode is not ready until:

- the report is generated from local or approved staging data;
- no production endpoint was called by default;
- mismatches are zero or documented and accepted;
- `go test ./...`, `npm test --prefix web`, and `npm run build --prefix web` pass;
- a human explicitly approves moving beyond shadow mode.
