CREATE TABLE IF NOT EXISTS ledger_entries (
  id TEXT PRIMARY KEY,
  event_type TEXT NOT NULL,
  account_id TEXT,
  user_id TEXT,
  workspace_id TEXT,
  compute_id TEXT,
  storage_id TEXT,
  attachment_id TEXT,
  source_event_id TEXT,
  request_fingerprint TEXT,
  amount_cents BIGINT NOT NULL DEFAULT 0,
  currency TEXT NOT NULL DEFAULT 'CNY',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ledger_entries_source_event_idx
  ON ledger_entries(source_event_id)
  WHERE source_event_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS ledger_entries_request_fingerprint_idx
  ON ledger_entries(request_fingerprint)
  WHERE request_fingerprint IS NOT NULL;

CREATE TABLE IF NOT EXISTS audit_events (
  id TEXT PRIMARY KEY,
  actor_id TEXT,
  action TEXT NOT NULL,
  target_kind TEXT NOT NULL,
  target_id TEXT,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS evidence_records (
  id TEXT PRIMARY KEY,
  evidence_type TEXT NOT NULL,
  account_id TEXT,
  workspace_id TEXT,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS task_receipts (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  workspace_id TEXT,
  task_id TEXT NOT NULL,
  receipt_type TEXT NOT NULL,
  status TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS wallets (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL,
  account_id TEXT NOT NULL,
  balance_cents BIGINT NOT NULL DEFAULT 0,
  frozen_cents BIGINT NOT NULL DEFAULT 0,
  total_recharged_cents BIGINT NOT NULL DEFAULT 0,
  holds JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS wallets_account_id_idx
  ON wallets(account_id);

CREATE INDEX IF NOT EXISTS wallets_user_id_idx
  ON wallets(user_id);

CREATE TABLE IF NOT EXISTS request_usage_logs (
  id TEXT PRIMARY KEY,
  account_id TEXT,
  user_id TEXT,
  workspace_id TEXT,
  request_fingerprint TEXT NOT NULL,
  units BIGINT NOT NULL DEFAULT 1,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(request_fingerprint)
);

CREATE TABLE IF NOT EXISTS resource_usage_logs (
  id TEXT PRIMARY KEY,
  account_id TEXT,
  workspace_id TEXT,
  compute_id TEXT,
  storage_id TEXT,
  resource_kind TEXT NOT NULL,
  quantity NUMERIC NOT NULL,
  unit TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS wallet_transactions (
  id TEXT PRIMARY KEY,
  account_id TEXT,
  user_id TEXT,
  transaction_type TEXT NOT NULL,
  amount_cents BIGINT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  ledger_entry_id TEXT REFERENCES ledger_entries(id),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS manual_topups (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL,
  user_id TEXT,
  operator_id TEXT NOT NULL,
  amount_cents BIGINT NOT NULL,
  currency TEXT NOT NULL DEFAULT 'CNY',
  audit_event_id TEXT REFERENCES audit_events(id),
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS billing_reconciliation_reports (
  id TEXT PRIMARY KEY,
  provider TEXT NOT NULL,
  account_id TEXT,
  status TEXT NOT NULL,
  expected_amount_cents BIGINT NOT NULL,
  actual_amount_cents BIGINT NOT NULL,
  difference_cents BIGINT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS idempotency_keys (
  key TEXT PRIMARY KEY,
  operation TEXT NOT NULL,
  result_id TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kubernetes_evidence_snapshots (
  id TEXT PRIMARY KEY,
  cluster_id TEXT NOT NULL,
  namespace TEXT NOT NULL,
  object_kind TEXT NOT NULL,
  object_name TEXT NOT NULL,
  workspace_id TEXT,
  resource_version TEXT,
  observed_generation BIGINT,
  readiness_status TEXT NOT NULL,
  redacted_object JSONB NOT NULL DEFAULT '{}'::jsonb,
  collected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
