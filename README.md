# OPL Ledger

`OPL-Ledger` is the standalone Ledger service split from `RenDeHuang/OPL-Cloud`.

It owns billing ledger events, audit events, evidence records, receipts, idempotency, usage records, Tencent bill reconciliation, and read-only Kubernetes runtime evidence snapshots.

It does not own OPL Console workspace lifecycle screens, OPL Fabric provisioning, OPL Gateway internals, One Person Lab framework internals, or `one-person-lab-app` runtime behavior.

## Stack

- Frontend: React + TypeScript
- Backend: Go
- Database: PostgreSQL
- Kubernetes: Go `client-go`

## Run API

```bash
go run ./cmd/opl-ledger-api
```

The API listens on `:8788` by default. Set `PORT` to override.

Health:

```bash
curl http://127.0.0.1:8788/healthz
```

## Run Frontend

```bash
npm install --prefix web
npm run dev --prefix web
```

## Verify

```bash
go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```

## Source Boundaries

The first contracts are copied from the original OPL Cloud contract package. JavaScript from OPL Cloud is migration reference only; this repository's backend implementation is Go.
