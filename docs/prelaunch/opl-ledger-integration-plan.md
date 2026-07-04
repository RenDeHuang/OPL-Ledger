# OPL Ledger Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the Ledger-facing contracts and code paths required for Console,
Fabric, and Gateway integration without switching production traffic away from
`medopl-3` / `OPL-Cloud`.

**Architecture:** Ledger remains the accounting and evidence service. Console
owns user/operator workflows, Fabric owns resource execution, Gateway owns model
request routing, and Ledger exposes stable idempotent APIs for wallet,
billing, usage, audit, evidence, and reconciliation.

**Tech Stack:** Go `net/http`, PostgreSQL migrations, in-memory test store,
React/Vite operator UI, local JSON migration dry-run tooling.

---

## File Map

- `internal/db/migrations/0001_initial.sql`: enforce durable schema invariants.
- `internal/db/migrations_test.go`: migration contract tests.
- `internal/api/server.go`: route registration and HTTP handlers.
- `internal/api/server_test.go`: route-level behavior tests.
- `internal/ledger/*.go`: store interfaces and PostgreSQL/in-memory behavior.
- `docs/prelaunch/opl-ledger-api-contract.md`: aggregate implemented API contract.
- `docs/prelaunch/opl-ledger-console-api-contract.md`: Console-facing contract.
- `docs/prelaunch/opl-ledger-fabric-api-contract.md`: Fabric-facing contract.
- `docs/prelaunch/data-migration-dry-run.md`: migration preflight evidence.

Do not create temporary planning files. Phase-specific scratch notes should be
folded into the relevant permanent contract or deleted before commit.

## Phase 1: Lock Ledger Accounting Invariants

- [ ] Keep the existing RED test in `internal/db/migrations_test.go` that
  requires wallet/manual-topup evidence links to be non-null.
- [ ] Update `internal/db/migrations/0001_initial.sql`:
  - `wallet_transactions.ledger_entry_id TEXT NOT NULL REFERENCES ledger_entries(id)`
  - `manual_topups.ledger_entry_id TEXT NOT NULL REFERENCES ledger_entries(id)`
  - `manual_topups.wallet_transaction_id TEXT NOT NULL REFERENCES wallet_transactions(id)`
  - `manual_topups.audit_event_id TEXT NOT NULL REFERENCES audit_events(id)`
- [ ] Run:

```bash
go test ./internal/db -run TestInitialMigrationDefinesWalletTransactionAndTopUpAuditFields -count=1
```

Expected: PASS.

- [ ] Run:

```bash
GOPROXY=off go test ./...
npm test --prefix web
npm run build --prefix web
git diff --check
```

Expected: all commands pass.

- [ ] Commit:

```bash
git add internal/db/migrations/0001_initial.sql internal/db/migrations_test.go
git commit -m "feat: require complete billing evidence links"
```

## Phase 2: Stabilize Console Contract

- [ ] Review `docs/prelaunch/opl-ledger-console-api-contract.md` with Console
  call sites from `medopl-3` / `OPL-Cloud`.
- [ ] For each Console read route, confirm auth class:
  - wallet/topup/usage/audit reads require admin token when configured.
  - mutating top-up requires service token when configured.
- [ ] Add missing route tests in `internal/api/auth_test.go` if any Console
  read route lacks admin-token coverage.
- [ ] Confirm manual top-up replay behavior in `internal/api/server_test.go`:
  exact replay returns `200 OK`; conflict returns `409 Conflict`.
- [ ] Update `docs/prelaunch/opl-ledger-api-contract.md` if implementation and
  Console contract diverge.
- [ ] Run:

```bash
GOPROXY=off go test ./internal/api ./internal/ledger
```

Expected: PASS.

- [ ] Commit Console contract and route-test changes together.

## Phase 3: Stabilize Fabric Contract

- [ ] Review `docs/prelaunch/opl-ledger-fabric-api-contract.md` with Fabric
  resource lifecycle expectations.
- [ ] Confirm existing handlers cover Fabric semantics:
  - `POST /api/v1/billing/holds`
  - `POST /api/v1/billing/holds/release`
  - `POST /api/v1/billing/settlements`
  - `POST /api/v1/billing/resource-usage`
  - `POST /api/v1/ledger/evidence-records`
  - `POST /api/v1/ledger/kubernetes-evidence-snapshots`
- [ ] Maintain route alias handlers for Fabric semantic calls:
  - `POST /api/v1/fabric/resource-preflight`
  - `POST /api/v1/fabric/resource-create-failed`
  - `POST /api/v1/fabric/resource-created`
  - `POST /api/v1/fabric/resource-usage-tick`
  - `POST /api/v1/fabric/resource-settlement`
  - `POST /api/v1/fabric/resource-stopped`
  - `POST /api/v1/fabric/resource-destroyed`
- [ ] Write tests that prove each alias maps to the same
  store behavior as the canonical billing route.
- [ ] Confirm settlement action intents are explicit enough for Fabric to act
  on `stop_compute` and storage freeze/destroy semantics.
- [ ] Run:

```bash
GOPROXY=off go test ./internal/api ./internal/billing ./internal/ledger
```

Expected: PASS.

- [ ] Commit Fabric contract and route changes together.

## Phase 4: Gateway Request Usage Integration

- [ ] Treat Gateway as the caller of `POST /api/v1/billing/request-usage`.
- [ ] Confirm request payload includes:
  - `accountId`
  - `userId`
  - `workspaceId`
  - `requestId`
  - `sourceEventId`
  - `requestFingerprint`
  - provider/model
  - token counts
  - requested and charged cents
- [ ] Confirm quota rejection writes no dedup, wallet, ledger, usage,
  transaction, or audit state.
- [ ] Confirm exact replay returns the original usage result and does not
  double-debit.
- [ ] Add API contract examples for Gateway if missing from
  `docs/prelaunch/opl-ledger-api-contract.md`.
- [ ] Run:

```bash
GOPROXY=off go test ./internal/api ./internal/usage ./internal/ledger
```

Expected: PASS.

## Phase 5: Migration and Shadow Mode

- [ ] Use only local exports or copied snapshots from `medopl-3`.
- [ ] Run dry-run:

```bash
go run ./cmd/opl-ledger-migration-dry-run \
  -input .local/migration-dry-run/input \
  -output .local/migration-dry-run
```

Expected before cutover: report status `pass`.

- [ ] Do not use `wallet_transactions.backfill.preview.json` or
  `money_normalization.preview.json` as automatic migration input.
- [ ] Resolve current blockers:
  - non-integer money values require approved cents normalization.
  - wallet-moving ledger entries require real wallet transactions.
- [ ] Run shadow mode with Console/Fabric/Gateway still using production
  `medopl-3` as primary.
- [ ] Compare Ledger outputs against current production billing behavior.
- [ ] Update `docs/prelaunch/cutover-checklist.md` only after shadow results are
  reviewed.

## Phase 6: Cleanup Rules

- [ ] Delete local `.local/` outputs before any commit unless the file is an
  intentionally ignored local artifact.
- [ ] Do not commit copied production snapshots, account ids, user ids, tokens,
  provider task payloads with secrets, or Kubernetes secrets.
- [ ] Remove phase-only scripts after their result is represented by permanent
  code, tests, or documentation.
- [ ] Keep only stable contracts, tests, migrations, and implementation files.

## Done Criteria

- Console contract is written and matches implemented routes.
- Fabric contract is written and maps to implemented routes or tested aliases.
- DB schema enforces complete billing evidence links.
- `GOPROXY=off go test ./...` passes.
- `npm test --prefix web` passes.
- `npm run build --prefix web` passes.
- Local migration dry-run has no blockers before cutover.
- No cloud deployment or mutation has occurred.
