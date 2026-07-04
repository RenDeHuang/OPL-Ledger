# OPL Ledger Rollback Checklist

This checklist defines how to return billing traffic to the existing
`OPL-Cloud` / `medopl-3` billing path if `OPL-Ledger` cutover is unsafe.

Rollback must preserve billing evidence and avoid silent double-charging.

## Rollback Triggers

- [ ] Ledger API is unavailable or returns repeated 5xx responses.
- [ ] Wallet balances diverge from `medopl-3`.
- [ ] Request usage billing diverges or misses deduplication.
- [ ] Manual topup replay creates a conflict or duplicate credit.
- [ ] Reconciliation guard blocks production unexpectedly.
- [ ] Evidence/audit/task receipt writes fail in a way that blocks user operations.
- [ ] PostgreSQL migration or query performance degrades production behavior.
- [ ] Security issue with service/admin tokens is suspected.

## Immediate Actions

- [ ] Stop sending new production billing writes to `OPL-Ledger`.
- [ ] Route Console/Fabric billing calls back to `medopl-3`.
- [ ] Keep `OPL-Ledger` database intact for forensic comparison.
- [ ] Do not delete failed Ledger records.
- [ ] Do not rerun migration against production without review.
- [ ] Record rollback start time and triggering reason.

## Data Protection

- [ ] Preserve `OPL-Ledger` PostgreSQL snapshot.
- [ ] Preserve `medopl-3` current state snapshot.
- [ ] Export recent Ledger audit/evidence/task receipts for comparison.
- [ ] Export recent wallet, usage, and reconciliation rows.
- [ ] Record the last source event id accepted by Ledger before rollback.

## OPL Cloud Billing Path

- [ ] Confirm `medopl-3` wallet balances are still authoritative.
- [ ] Confirm Console request usage writes go to `medopl-3`.
- [ ] Confirm manual topups go to `medopl-3`.
- [ ] Confirm workspace lifecycle billing guard behavior is restored to the old path.
- [ ] Confirm no new production traffic is using Ledger service/admin tokens.

## Duplicate Charge Review

- [ ] Compare source event ids accepted by Ledger during the failed window.
- [ ] Compare request fingerprints accepted by Ledger during the failed window.
- [ ] Identify any event accepted by both Ledger and `medopl-3`.
- [ ] Identify any event accepted by neither system.
- [ ] Prepare manual correction entries only after review.

## Communication

- [ ] Notify engineering owner.
- [ ] Notify operations owner.
- [ ] Record customer-impact window if any.
- [ ] Record whether user-facing billing data may be delayed.

## Re-Entry Gate

Do not attempt cutover again until:

- [ ] root cause is documented;
- [ ] a regression test exists for the failure;
- [ ] local verification passes;
- [ ] shadow-mode comparison passes again;
- [ ] migration dry-run passes again if data shape changed;
- [ ] a human explicitly approves retrying production cutover.

## Final Rollback State

Rollback is complete only when:

- [ ] `medopl-3` is production-primary again;
- [ ] new Ledger writes are stopped or back in shadow mode;
- [ ] evidence needed for reconciliation is preserved;
- [ ] duplicate/missing billing events are reviewed;
- [ ] the exact rollback commit/version and timestamps are recorded.
