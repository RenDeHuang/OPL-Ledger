# OPL Ledger Cutover Checklist

This checklist defines the required gates before `OPL-Ledger` can become the
billing, audit, evidence, and receipt source of truth for `OPL-Cloud`.

Cutover is not approved by this document alone. A human must explicitly approve
production changes after all gates pass.

## Non-Negotiable Rules

- Do not run cloud mutation commands from this repository.
- Do not push images from this checklist.
- Do not write production secrets into tracked files.
- Do not switch Console production traffic until explicit approval is recorded.
- Keep `medopl-3` / `OPL-Cloud` production-primary until the final approval step.

## Source Alignment

- [ ] Active `medopl-3` commit is recorded in `docs/alignment/opl-cloud-source.md`.
- [ ] Active `OPL-Cloud` commit is recorded in `docs/alignment/opl-cloud-source.md`.
- [ ] API contract in `docs/prelaunch/opl-ledger-api-contract.md` matches implemented endpoints.
- [ ] `OPL-Ledger` version/commit for cutover is recorded.

## Local Verification

- [ ] `GOPROXY=off go test ./...` passes.
- [ ] `npm test --prefix web` passes.
- [ ] `npm run build --prefix web` passes.
- [ ] `git diff --check` passes.
- [ ] Local PostgreSQL migrations run against a local database.
- [ ] Local API smoke test passes with service/admin tokens configured.

## Functional Parity

- [ ] Manual topup writes wallet snapshot, ledger credit, wallet transaction, manual topup record, and audit event.
- [ ] Request usage writes dedup, usage log, wallet debit, wallet transaction, ledger debit, and audit event.
- [ ] Request quota rejection writes no billing state.
- [ ] Hold creation and release match the `medopl-3` behavior.
- [ ] Settlement charges available balance first, captures holds second, and never creates negative wallet balance.
- [ ] Hold exhaustion returns expected action/state intents.
- [ ] Resource usage carries compute/storage/attachment/workspace ids.
- [ ] Audit events and evidence records are append-only and do not affect billing ledger balances.
- [ ] Task receipts are idempotent and validate workspace ownership.
- [ ] Reconciliation guard blocks missing, stale, and failed reports.
- [ ] Kubernetes evidence snapshots are redacted and contain no secret values.

## Shadow Mode

- [ ] Shadow mode procedure in `docs/prelaunch/shadow-mode.md` has been followed.
- [ ] Comparison procedure in `tools/compare-opl-cloud-ledger.md` produced a local report.
- [ ] Report source is local or approved staging, not production by default.
- [ ] Mismatches are zero, or every mismatch is documented and accepted.
- [ ] No production endpoint was called by the default comparison path.

## Migration Dry Run

- [ ] `docs/prelaunch/data-migration-dry-run.md` has been followed.
- [ ] `.local/migration-dry-run/migration-report.json` exists.
- [ ] Migration report status is `pass`.
- [ ] Duplicate source keys are resolved.
- [ ] Missing foreign references are resolved.
- [ ] Money values are integer cents.
- [ ] Mixed currencies are absent.
- [ ] Workspace ids are present for billable workspace-scoped rows.
- [ ] Evidence snapshot secret scan passed.

## Configuration

- [ ] `DATABASE_URL` points to the approved production PostgreSQL target in the deployment system, not a tracked file.
- [ ] `OPL_LEDGER_SERVICE_TOKEN` is set in the approved secret manager.
- [ ] `OPL_LEDGER_ADMIN_TOKEN` is set in the approved secret manager.
- [ ] `OPL_LEDGER_SHADOW_MODE` and `OPL_LEDGER_CUTOVER_ENABLED` desired values are reviewed.
- [ ] Billing price environment variables match `medopl-3`.
- [ ] Reconciliation max age and tolerance are reviewed.
- [ ] Kubernetes evidence collection remains read-only.

## Approval

- [ ] Rollback checklist is complete and reviewed.
- [ ] Production secret changes are explicitly approved.
- [ ] Console/Fabric traffic switch is explicitly approved.
- [ ] Data migration is explicitly approved.
- [ ] Cutover time window is approved.
- [ ] Owner responsible for monitoring is assigned.

## Final Gate

Do not cut over unless every section above is complete. The final approval must
name the exact `OPL-Ledger` commit and the exact `OPL-Cloud` / `medopl-3`
version being replaced.
