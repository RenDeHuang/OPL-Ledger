# OPL Ledger Contracts

These contracts are copied from the original `RenDeHuang/OPL-Cloud` implementation and form the first machine-readable Ledger boundary for `RenDeHuang/OPL-Ledger`.

## Included

- `opl-cloud-billing-ledger-contract.json`: billing ledger semantics.
- `opl-cloud-evidence-ledger-contract.json`: evidence and receipt semantics.
- `opl-cloud-storage-backup-contract.json`: backup and restore receipt evidence references.
- `opl-cloud-deployment-contract.json`: deployment and runtime verification evidence references.

## Ownership

`OPL-Ledger` owns Ledger records, receipts, evidence, idempotency, and reconciliation behavior derived from these contracts.

`OPL-Ledger` does not own OPL Console, OPL Fabric, OPL Gateway, OPL Workspace runtime, or One Person Lab framework internals.
