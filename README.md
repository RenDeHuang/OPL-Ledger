# OPL Ledger

`OPL-Ledger` is the standalone Ledger service split from `RenDeHuang/OPL-Cloud`.

It owns billing ledger events, audit events, evidence records, receipts, idempotency, usage records, Tencent bill reconciliation, and read-only Kubernetes runtime evidence snapshots.

It does not own OPL Console lifecycle screens, OPL Fabric provisioning, OPL Gateway internals, One Person Lab framework internals, or `one-person-lab-app` runtime behavior.

## Stack

- Frontend: React + TypeScript
- Backend: Go
- Database: PostgreSQL
- Kubernetes: Go `client-go`

## First Verification

```bash
go test ./...
git diff --check
```
