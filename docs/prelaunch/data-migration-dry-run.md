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
- `wallet_transactions.backfill.preview.json`
- `money_normalization.preview.json`
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
- `wallet_transactions.backfill.preview.json`
- `money_normalization.preview.json`
- `manual_topups.preview.json`
- `request_usage_logs.preview.json`
- `request_usage_dedup.preview.json`
- `resource_usage_logs.preview.json`
- `audit_events.preview.json`
- `migration-report.json`

The remaining preview files in this document are still required before final
cutover approval; this executable slice covers the manual top-up accounting loop
plus request and resource usage replay inputs with their
wallet/ledger/transaction/audit or dedup references.

## Current Local `medopl-3` Snapshot Findings

The local snapshot at `/home/dev/medopl-3/.runtime/opl-cloud-state.json` was
checked with the dry-run tool on 2026-07-04. The report is currently blocked
before migration approval.

Current row counts from that local snapshot:

- `wallets.preview.json`: 3
- `ledger_entries.preview.json`: 6
- `wallet_transactions.preview.json`: 1
- `wallet_transactions.backfill.preview.json`: 5
- `money_normalization.preview.json`: 6
- `manual_topups.preview.json`: 1
- `request_usage_logs.preview.json`: 0
- `request_usage_dedup.preview.json`: 0
- `resource_usage_logs.preview.json`: 0
- `audit_events.preview.json`: 4

Blocking findings:

- `non_integer_money_values`:
  - `usr-pi-demo` wallet balance `5498.7967`
  - `ledger-141yg9o` amount `-0.0033`
  - `wallet-tx-1eht7im` balance before/after values `498.7967` and `5498.7967`
  - `manual-topup-1pu118k` balance before/after values `498.7967` and `5498.7967`
- `wallet_moving_ledger_missing_transaction`:
  - `ledger-13u9e9c`
  - `ledger-z3uz8g`
  - `ledger-8chqg5`
  - `ledger-1w00fpz`
  - `ledger-141yg9o`

These records must be corrected in the source export, migrated through an
approved normalization step, or explicitly reviewed before cutover. The dry run
must pass again after any correction.

The dry run also writes `money_normalization.preview.json` for these blocked
money values. Each row includes the source record id, field name, original
value, whether the source field was already a cents field, and round/floor/ceil
candidate cents. These candidates are review material only; they do not choose a
normalization policy and do not clear `non_integer_money_values`.

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

Backfill candidate preview:

- `wallet_transactions.backfill.preview.json` is local-only and review-only.
- It is generated for wallet-moving ledger entries that do not have a linked
  wallet transaction in the source export.
- Candidate ids are deterministic: `wtx_backfill_<ledger_entry_id>`.
- Candidate `transaction_type` is derived from the ledger event family:
  `credit`, `*_hold`, `*_hold_released`, `request_debit`, `*_debit`, or
  adjustment events.
- Candidate `payload.backfillCandidate` is always `true` and contains the
  ledger preview payload for reviewer context.
- This preview does not clear the
  `wallet_moving_ledger_missing_transaction` blocker. The source export,
  approved normalization step, or reviewed migration script must still create
  real wallet transactions before cutover.
- Candidates derived from rows already blocked by `non_integer_money_values`
  must not be applied without an explicit cents normalization decision.

## Money Normalization Preview

Source:

- any money field read by the dry-run mapper that cannot resolve to integer
  cents.

Target:

- no production table. `money_normalization.preview.json` is a local reviewer
  artifact.

Fields:

- `record_id`: source identity from `id`, source event id, request id, or account id.
- `field`: source field that failed integer-cents validation.
- `original_value`: original value as text.
- `already_cents`: whether the source field name looked like a cents field.
- `round_cents`, `floor_cents`, `ceil_cents`: possible integer cents outcomes.
- `payload`: cloned source row for reviewer context.

Rules:

- this preview must not be used as an automatic migration input.
- a human-approved policy must decide how to handle each row.
- after correction or approved normalization, the dry run must pass without
  `non_integer_money_values`.

Validation:

- every transaction linked to a ledger entry must resolve to a preview `ledger_entries` row.
- linked transactions and ledger entries must agree on account id, workspace id, source event id, and amount.
- linked transaction type must match the ledger event family: `credit`, `*_hold`, `*_hold_released`, `request_debit`, or `*_debit`.
- wallet-moving ledger entries in those event families must have a linked wallet transaction.
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
- charged request usage resolves to a `request_debit` ledger entry with matching account, workspace, source event, request fingerprint, and negative charged amount.
- charged request usage resolves to a `debit` wallet transaction with matching usage log id, ledger entry id, account, workspace, source event, and negative charged amount.
- request usage resolves to a `billing.request_usage_recorded` audit event targeting the usage log id.
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
- `attachmentId`
- `resourceType` -> `resource_kind`
- quantity and unit
- unit price, amount, requested amount, currency, and source event
- full source payload

Validation:

- compute usage includes `computeId` and `workspaceId`.
- storage usage includes `storageId`, optional `attachmentId`, and `workspaceId`.
- resource usage includes a positive quantity, unit, and source event id.
- `resource_usage_logs.source_event_id` is unique when present.

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
