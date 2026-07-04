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

For production persistence, set `DATABASE_URL`:

```bash
DATABASE_URL='postgres://opl_ledger:<local-password>@127.0.0.1:5432/opl_ledger?sslmode=disable' \
PORT=8788 go run ./cmd/opl-ledger-api
```

When `DATABASE_URL` is present, startup runs the embedded PostgreSQL migrations and stores ledger entries, task receipts, and reconciliation reports in PostgreSQL. Without `DATABASE_URL`, the service uses an in-memory store for local development and tests.

Health:

```bash
curl http://127.0.0.1:8788/healthz
```

## API

- `POST /api/v1/ledger/entries`
- `GET /api/v1/ledger/entries`
- `GET /api/v1/ledger/summary`
- `POST /api/v1/billing/topups`
- `POST /api/v1/billing/request-usage`
- `POST /api/v1/billing/reconciliation`
- `GET /api/v1/billing/reconciliation/latest`
- `POST /api/v1/ledger/task-receipts`
- `GET /api/v1/ledger/task-receipts`

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

## Pre-Launch

`OPL-Cloud` / `medopl-3` remains the primary production path until explicit cutover approval. Use [.env.example](.env.example) for local configuration shape and [docs/prelaunch/opl-ledger-prelaunch-readiness.md](docs/prelaunch/opl-ledger-prelaunch-readiness.md) for the pre-launch boundary, required configuration, non-cloud rules, and completion checklist.

## Source Boundaries

The first contracts are copied from the original OPL Cloud contract package. JavaScript from OPL Cloud is migration reference only; this repository's backend implementation is Go.
