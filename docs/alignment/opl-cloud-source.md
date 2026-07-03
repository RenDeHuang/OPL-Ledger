# OPL Cloud Alignment Source

This `OPL-Ledger` baseline is aligned to the following original `OPL-Cloud` source snapshot.

## Source Repository

- Repository: `https://github.com/RenDeHuang/OPL-Cloud`
- Branch observed locally: `main`
- Commit: `974d14ffa15ba302878e6681128b05aef8be8e2a`
- Short commit: `974d14f`
- Commit subject: `fix: avoid unique ledger index on legacy rows`
- Commit time: `2026-07-03T17:29:06+08:00`

## Ledger Baseline

- Repository: `https://github.com/RenDeHuang/OPL-Ledger`
- Branch: `main`
- Baseline commit before this alignment note: `16eca26232c9ca2bc67294d48c6a24b24cf16d29`
- Short commit: `16eca26`
- Commit subject: `fix: enforce ledger append idempotency contract`
- Commit time: `2026-07-03T21:22:02+08:00`

## Copied Contract Snapshot

The following files were copied byte-for-byte from `OPL-Cloud` at source commit `974d14f`:

- `packages/contracts/opl-cloud-billing-ledger-contract.json`
- `packages/contracts/opl-cloud-evidence-ledger-contract.json`
- `packages/contracts/opl-cloud-storage-backup-contract.json`
- `packages/contracts/opl-cloud-deployment-contract.json`

They now live under `OPL-Ledger/contracts/`.

## Alignment Rule

When `OPL-Cloud` is updated to call standalone `OPL-Ledger`, treat `974d14f` as the source baseline for the split. Any later `OPL-Cloud` changes to billing ledger, evidence ledger, storage backup evidence, deployment evidence, idempotency, or reconciliation should be reviewed against this document and ported deliberately into `OPL-Ledger`.
