# OPL Ledger Data Migration Dry Run

This document defines the local dry-run plan for moving billing truth from
`medopl-3` / `OPL-Cloud` into `OPL-Ledger`. It is not a production migration
script and must not write production data.

## Safety Rules

- Use local exports, fixtures, or approved staging snapshots only.
- Write dry-run output to an ignored local directory such as `.local/migration-dry-run/`.
- Do not call production Console, production Ledger, Tencent Cloud APIs, or Kubernetes.
- Do not commit real account ids, user ids, tokens, or production secrets.
- Do not run a real migration until this dry run produces a reviewed output report.

## Required Inputs

Export these local JSON files from `medopl-3` state, or copy the local single
state snapshot into the dry-run input directory as `opl-cloud-state.json`.

Supported single-file source:

- `/home/dev/medopl-3/.runtime/opl-cloud-state.json` copied to
  `.local/migration-dry-run/input/opl-cloud-state.json`

Supported split-file source:

- `users.json`
- `accounts.json`
- `billingLedger.json`
- `walletTransactions.json`
- `manualTopups.json`
- `requestUsageLogs.json`
- `requestUsageDedup.json`
- `resourceUsageLogs.json`
- `audit.json`
- `evidenceLedger.json`
- `workspaces.json`
- `computeResources.json`
- `storageVolumes.json`
- `storageAttachments.json`

## Target Outputs

The dry run must produce:

- `wallets.preview.json`
- `ledger_entries.preview.json`
- `wallet_transactions.preview.json`
- `manual_topups.preview.json`
- `request_usage_logs.preview.json`
- `request_usage_dedup.preview.json`
- `resource_usage_logs.preview.json`
- `audit_events.preview.json`
- `evidence_records.preview.json`
- `task_receipts.preview.json`
- `kubernetes_evidence_snapshots.preview.json`
- `migration-report.json`

## Local Command

Run the local preview tool from the `opl-ledger` repository root:

```bash
go run ./cmd/opl-ledger-migration-dry-run \
  -input .local/migration-dry-run/input \
  -output .local/migration-dry-run
```

The command reads only local JSON files, writes only local preview/report files,
and does not connect to production Console, Ledger, Tencent Cloud, Kubernetes,
or PostgreSQL.

The input directory may contain either split export files such as `users.json`
and `billingLedger.json`, or a copied `opl-cloud-state.json` with matching
top-level keys such as `users`, `billingLedger`, `walletTransactions`,
`manualTopups`, and `audit`. When both exist, split files take precedence for
that dataset.

Current executable coverage:

- `wallets.preview.json`
- `ledger_entries.preview.json`
- `wallet_transactions.preview.json`
- `manual_topups.preview.json`
- `request_usage_logs.preview.json`
- `request_usage_dedup.preview.json`
- `audit_events.preview.json`
- `migration-report.json`

The remaining preview files in this document are still required before final
cutover approval; this executable slice covers the manual top-up accounting loop
and request usage replay inputs with their wallet/ledger/transaction/audit or
dedup references.

## Wallets

Source:

- `users[].wallets[]`
- account-level wallet helpers in `/home/dev/medopl-3/packages/console/src/services/wallet-service.js`

Target:

- `wallets`

Mapping:

- `accountId` -> `account_id`
- `userId` -> `user_id`
- `balance` or balance cents -> `balance_cents`
- frozen total -> `frozen_cents`
- holds map -> `holds`
- total recharged -> `total_recharged_cents`

Validation:

- `balance_cents - frozen_cents - sum(holds)` equals `availableCents`.
- `frozen_cents` equals the sum of all hold amounts.
- `availableCents`, when present in the source export, equals `balance_cents - frozen_cents`.
- `total_recharged_cents` is not lower than `balance_cents`.
- all money values are integer cents.

## Billing Ledger

Source:

- `state.billingLedger`

Target:

- `ledger_entries`

Mapping:

- `id` may be retained in preview, but cutover matching must use stable source keys.
- `type` -> `event_type`
- `accountId` -> `account_id`
- `userId` -> `user_id`
- `workspaceId` -> `workspace_id`
- `computeId` -> `compute_id`
- `storageId` -> `storage_id`
- `attachmentId` -> `attachment_id`
- `sourceEventId` -> `source_event_id`
- `amount` -> `amount_cents`
- `currency` -> `currency`
- remaining fields -> `payload`

Rules:

- debit rows must remain negative in `ledger_entries`.
- request usage replay must also preserve `requestFingerprint` when available.
- generated ids are not acceptable as the only replay key.

## Wallet Transactions

Source:

- `state.walletTransactions`

Target:

- `wallet_transactions`

Mapping:

- `id` -> preview id
- `accountId`, `userId`, `workspaceId`
- `type` -> `transaction_type`
- `amount` -> `amount_cents`
- `currency`
- `sourceEventId`
- `ledgerEntryId`
- `usageLogId`
- `fundingSource`
- before/after balance and frozen fields
- full source transaction -> `payload`

Validation:

- every transaction linked to a ledger entry must resolve to a preview `ledger_entries` row.
- before/after balances must match wallet replay.

## Manual Topups

Source:

- `state.manualTopups`

Target:

- `manual_topups`

Mapping:

- `operatorUserId` -> `operator_id`
- `operatorAccountId`
- `targetUserId`
- `targetAccountId`
- `sourceEventId` -> `source_event_id`
- legacy `reason` or top-up source -> `source_event_id` only when `sourceEventId` is absent
- `reason` -> payload `reason`
- `amount` -> `amount_cents`
- `currency`
- `status`
- before/after balance fields
- `ledgerEntryId`
- `walletTransactionId`
- `auditEventId`

Validation:

- `source_event_id` is unique.
- preview rows preserve distinct `sourceEventId` and operator-visible `reason` when both exist.
- linked wallet transaction, ledger entry, and audit event exist.
- linked ledger entry is `credit` and matches top-up `source_event_id`, `amount_cents`, and account id.
- linked wallet transaction is `credit` and matches top-up source, amount, account id, and ledger entry id.
- linked audit event is `account.credit_granted` and targets the manual top-up id.

## Request Usage

Source:

- `state.requestUsageLogs`
- `state.requestUsageDedup`

Target:

- `request_usage_logs`
- `request_usage_dedup`

Mapping:

- `requestId`
- `workspaceId`
- `accountId`
- `userId`
- `provider`
- `model`
- token counts
- requested/charged/unpaid cents
- `sourceEventId`
- `requestFingerprint`
- `ledgerEntryId`
- quota snapshot -> `payload`

Validation:

- `requestFingerprint` is unique.
- dedup rows resolve by `workspaceId + sourceEventId` and `workspaceId + requestId`.
- every `request_usage_dedup.usage_log_id` resolves to a preview `request_usage_logs` row.
- linked dedup and log rows agree on workspace id, request id, source event id, and request fingerprint.
- quota rejections do not create wallet, ledger, usage, transaction, or audit rows.

## Resource Usage

Source:

- `state.resourceUsageLogs`

Target:

- `resource_usage_logs`

Mapping:

- `accountId`
- `workspaceId`
- `computeId`
- `storageId`
- `attachmentId` in payload until table column support is added
- `resourceType` -> `resource_kind`
- quantity and unit
- full source payload

Validation:

- compute usage includes `computeId` and `workspaceId`.
- storage usage includes `storageId`, optional `attachmentId`, and `workspaceId`.

## Audit

Source:

- `state.audit`

Target:

- `audit_events`

Mapping:

- `accountId`
- `workspaceId`
- `actorId` if present
- `type` -> `action`
- target object inference -> `target_kind`, `target_id`
- `sourceEventId`
- full source audit -> `payload`

Validation:

- billing-related audit events resolve to a source event.
- audit rows do not appear in `ledger_entries`.

## Evidence

Source:

- `state.evidenceLedger`

Targets:

- `evidence_records`
- `task_receipts`

Mapping:

- `task.evidence.v1` -> `task_receipts`
- all other workspace/control-plane evidence -> `evidence_records`
- `type`
- `accountId`
- `workspaceId`
- `taskId` when present
- `sourceEventId` when present
- actor, plan, approval, environment, refs, review results, continuation -> payload columns

Validation:

- task receipt idempotency key is `accountId + workspaceId + taskId + sourceEventId`.
- evidence rows do not create billing ledger rows.

## Kubernetes Evidence

Source:

- local or approved staging read-only snapshots only.

Target:

- `kubernetes_evidence_snapshots`

Mapping:

- cluster, namespace, object kind, object name
- workspace label -> `workspace_id`
- resource version
- observed generation
- readiness status
- redacted object

Validation:

- no secret values appear in `redacted_object`.
- no Kubernetes apply/create/delete command is part of the dry run.

## Required Report

The dry run must write `.local/migration-dry-run/migration-report.json`:

```json
{
  "generatedAt": "2026-07-04T12:00:00Z",
  "source": "local",
  "status": "pass",
  "rowCounts": {},
  "mismatches": [],
  "blockedReasons": []
}
```

The report must include:

- row counts for every source and target preview file;
- duplicate source keys;
- missing foreign references;
- non-integer money values;
- mixed currencies;
- missing workspace ids;
- secret scan result for evidence snapshots.

## Gate Before Real Migration

A real migration is blocked until:

- the dry-run report exists locally;
- `status` is `pass`;
- all mismatches are resolved or explicitly accepted;
- `go test ./...`, `npm test --prefix web`, and `npm run build --prefix web` pass;
- rollback and cutover checklists are complete;
- a human explicitly approves production migration.
