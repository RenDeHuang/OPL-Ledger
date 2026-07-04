# OPL Ledger Local Pre-Launch Verification

Use this checklist before committing, before push, and before any cutover
discussion. It is local verification only.

## Required Commands

Run from the repository root:

```bash
GOPROXY=off go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```

Expected result:

- every command exits with status `0`;
- Go tests cover API, ledger, wallet, billing, usage, audit, evidence, auth, reconciliation, ownership, and Kubernetes evidence packages;
- Web tests pass with Vitest;
- Web production build completes with TypeScript and Vite;
- `git diff --check` prints no whitespace errors.

## Local PostgreSQL Verification

Use a local PostgreSQL database only. Do not point `DATABASE_URL` at production
for pre-launch verification.

Example:

```bash
createdb opl_ledger_local
DATABASE_URL='postgres://localhost/opl_ledger_local?sslmode=disable' \
  PORT=8788 \
  OPL_LEDGER_SERVICE_TOKEN=local-service-token \
  OPL_LEDGER_ADMIN_TOKEN=local-admin-token \
  go run ./cmd/opl-ledger-api
```

Startup should run embedded migrations. Verify health:

```bash
curl http://127.0.0.1:8788/healthz
```

Verify a mutating call requires the service token:

```bash
curl -i -X POST http://127.0.0.1:8788/api/v1/billing/topups \
  -H 'content-type: application/json' \
  -d '{"accountId":"acct_local","amountCents":100,"sourceEventId":"local_topup_1","reason":"local verification credit"}'
```

Expected: `401` when the token is configured and omitted.

Verify the service token path:

```bash
curl -i -X POST http://127.0.0.1:8788/api/v1/billing/topups \
  -H 'authorization: Bearer local-service-token' \
  -H 'content-type: application/json' \
  -d '{"accountId":"acct_local","amountCents":100,"sourceEventId":"local_topup_1","reason":"local verification credit"}'
```

Expected: `201` on first write and `200` on exact replay.

Verify an admin read:

```bash
curl -i http://127.0.0.1:8788/api/v1/audit/events?accountId=acct_local \
  -H 'authorization: Bearer local-admin-token'
```

Expected: `200`.

## No Cloud Upload Or Deploy

Pre-launch verification does not include:

- pushing container images;
- applying Kubernetes manifests;
- creating, updating, or deleting cloud resources;
- writing production secrets;
- changing DNS, TLS, TCR, TKE, Ingress, PVC, or VolumeSnapshot resources;
- switching Console/Fabric production traffic to `OPL-Ledger`.

Commands such as `kubectl apply`, `kubectl create`, `kubectl delete`, image
pushes, and production secret writes require explicit approval outside this
local verification checklist.

## Documentation Gate

Before considering cutover work:

- review `docs/prelaunch/shadow-mode.md`;
- generate a local shadow-mode comparison report;
- review `docs/prelaunch/data-migration-dry-run.md`;
- generate a local migration dry-run report;
- complete `docs/prelaunch/cutover-checklist.md`;
- complete `docs/prelaunch/rollback-checklist.md`.
