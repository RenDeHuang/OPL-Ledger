# OPL Ledger Shadow Mode

Shadow mode means `OPL-Cloud` / `medopl-3` remains production-primary while
`OPL-Ledger` records the same billing, audit, evidence, and receipt facts for
comparison.

## Boundary

Production traffic stays on `medopl-3` until explicit cutover approval.
`OPL-Ledger` can receive mirrored or locally replayed events, but it must not
become the source of production billing truth during shadow mode.

## Local-Only Default

The default comparison path is local-only:

1. Export or fixture `medopl-3` state into `.local/shadow-mode/`.
2. Run `opl-ledger` locally with local PostgreSQL or memory storage.
3. Replay the same local events into `opl-ledger`.
4. Query `opl-ledger` from `127.0.0.1` or `localhost`.
5. Compare using `tools/compare-opl-cloud-ledger.md`.

Do not call production by default. Do not use real production tokens in local
files. Do not mutate cloud resources.

## Auth Requirements

When shadow mode calls the local Ledger API:

- mutating endpoints use `Authorization: Bearer $OPL_LEDGER_SERVICE_TOKEN`;
- operator evidence reads use `Authorization: Bearer $OPL_LEDGER_ADMIN_TOKEN`;
- tokens must stay in ignored local env files or secret managers.

## Event Coverage

Shadow mode must cover:

- manual topups;
- wallet transactions;
- request usage billing and quota snapshots;
- resource usage logs;
- hold creation, capture, exhaustion, and release;
- settlement outputs;
- reconciliation reports and guard state;
- audit events;
- evidence records;
- task evidence receipts;
- read-only Kubernetes evidence snapshots when local/staging input is approved.

## Pass Criteria

Shadow mode comparison passes only when:

- every compared record uses stable source keys instead of generated IDs;
- amounts are integer cents and currencies match;
- wallet balances and holds match;
- idempotency replays produce the same existing records;
- no evidence or audit record appears in billing ledger entries;
- reconciliation guard decisions match;
- no secret values appear in evidence snapshots;
- the local comparison report contains zero unexplained mismatches.

## Still Not Cutover

Passing shadow mode does not deploy or switch traffic. Cutover still requires
data migration dry-run, rollback checklist, production secret approval, and an
explicit human approval step.
