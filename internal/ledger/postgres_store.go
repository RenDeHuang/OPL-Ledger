package ledger

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Store interface {
	AppendEntry(context.Context, AppendEntryInput) (AppendEntryResult, error)
	ListEntries(context.Context, EntryFilter) ([]Entry, error)
	Summary(context.Context, EntryFilter) (Summary, error)
	AppendTaskReceipt(context.Context, TaskReceiptInput) (TaskReceipt, error)
	ListTaskReceipts(context.Context, TaskReceiptFilter) ([]TaskReceipt, error)
	AppendReconciliationReport(context.Context, ReconciliationReport) (ReconciliationReport, error)
	LatestReconciliationReport(context.Context) (ReconciliationReport, error)
}

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) AppendEntry(ctx context.Context, input AppendEntryInput) (AppendEntryResult, error) {
	if input.EventType == "" {
		return AppendEntryResult{}, errors.New("eventType is required")
	}
	if input.SourceEventID == "" && input.RequestFingerprint == "" {
		return AppendEntryResult{}, errors.New("sourceEventId or requestFingerprint is required")
	}
	if input.Currency == "" {
		input.Currency = "CNY"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AppendEntryResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	existing, err := s.entriesForIdempotencyKeys(ctx, tx, input)
	if err != nil {
		return AppendEntryResult{}, err
	}
	if len(existing) > 1 {
		return AppendEntryResult{}, ErrIdempotencyConflict
	}
	if len(existing) == 1 {
		entry := existing[0]
		if !sameReplayPayload(entry, input) {
			return AppendEntryResult{}, ErrIdempotencyConflict
		}
		if input.SourceEventID != "" && entry.SourceEventID == "" {
			entry.SourceEventID = input.SourceEventID
			if err := s.bindSourceEventPostgres(ctx, tx, entry.ID, input.SourceEventID); err != nil {
				return AppendEntryResult{}, err
			}
		}
		if input.RequestFingerprint != "" && entry.RequestFingerprint == "" {
			entry.RequestFingerprint = input.RequestFingerprint
			if err := s.bindRequestFingerprintPostgres(ctx, tx, entry.ID, input.RequestFingerprint); err != nil {
				return AppendEntryResult{}, err
			}
		}
		if err := tx.Commit(); err != nil {
			return AppendEntryResult{}, err
		}
		committed = true
		return AppendEntryResult{Entry: entry, Created: false}, nil
	}

	entry := Entry{
		ID:                 randomID(),
		EventType:          input.EventType,
		AccountID:          input.AccountID,
		UserID:             input.UserID,
		WorkspaceID:        input.WorkspaceID,
		ComputeID:          input.ComputeID,
		StorageID:          input.StorageID,
		AttachmentID:       input.AttachmentID,
		SourceEventID:      input.SourceEventID,
		RequestFingerprint: input.RequestFingerprint,
		AmountCents:        input.AmountCents,
		Currency:           input.Currency,
		CreatedAt:          nowUTC(),
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id,
			source_event_id, request_fingerprint, amount_cents, currency, payload, created_at
		) VALUES (
			$1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			NULLIF($9, ''), NULLIF($10, ''), $11, $12, '{}'::jsonb, $13
		)`,
		entry.ID,
		entry.EventType,
		entry.AccountID,
		entry.UserID,
		entry.WorkspaceID,
		entry.ComputeID,
		entry.StorageID,
		entry.AttachmentID,
		entry.SourceEventID,
		entry.RequestFingerprint,
		entry.AmountCents,
		entry.Currency,
		entry.CreatedAt,
	)
	if err != nil {
		return AppendEntryResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AppendEntryResult{}, err
	}
	committed = true
	return AppendEntryResult{Entry: entry, Created: true}, nil
}

func (s *PostgresStore) entriesForIdempotencyKeys(ctx context.Context, tx *sql.Tx, input AppendEntryInput) ([]Entry, error) {
	rows, err := tx.QueryContext(ctx, selectLedgerEntryColumns+` FROM ledger_entries WHERE source_event_id = $1 OR request_fingerprint = $2 ORDER BY created_at LIMIT 2`, input.SourceEventID, input.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (s *PostgresStore) bindSourceEventPostgres(ctx context.Context, tx *sql.Tx, id string, sourceEventID string) error {
	_, err := tx.ExecContext(ctx, `UPDATE ledger_entries SET source_event_id = NULLIF($1, '') WHERE id = $2`, sourceEventID, id)
	return err
}

func (s *PostgresStore) bindRequestFingerprintPostgres(ctx context.Context, tx *sql.Tx, id string, requestFingerprint string) error {
	_, err := tx.ExecContext(ctx, `UPDATE ledger_entries SET request_fingerprint = NULLIF($1, '') WHERE id = $2`, requestFingerprint, id)
	return err
}

func (s *PostgresStore) ListEntries(ctx context.Context, filter EntryFilter) ([]Entry, error) {
	where, args := ledgerEntryWhere(filter)
	query := selectLedgerEntryColumns + ` FROM ledger_entries`
	if where != "" {
		query += ` WHERE ` + where
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (s *PostgresStore) Summary(ctx context.Context, filter EntryFilter) (Summary, error) {
	entries, err := s.ListEntries(ctx, filter)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{AccountID: filter.AccountID, Currency: "CNY", EntryCount: len(entries)}
	for _, entry := range entries {
		summary.BalanceCents += entry.AmountCents
		if entry.Currency != "" {
			summary.Currency = entry.Currency
		}
	}
	return summary, nil
}

func (s *PostgresStore) AppendTaskReceipt(ctx context.Context, input TaskReceiptInput) (TaskReceipt, error) {
	if input.AccountID == "" {
		return TaskReceipt{}, errors.New("task_evidence_account_required")
	}
	if input.TaskID == "" {
		return TaskReceipt{}, errors.New("task_evidence_task_required")
	}
	if len(input.Plan) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_plan_required")
	}
	if len(input.Approval) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_approval_required")
	}
	if len(input.Environment) == 0 {
		return TaskReceipt{}, errors.New("task_evidence_environment_required")
	}
	receipt := TaskReceipt{
		ID:            randomID(),
		Type:          "task.evidence.v1",
		AccountID:     input.AccountID,
		WorkspaceID:   input.WorkspaceID,
		TaskID:        input.TaskID,
		Actor:         mapOrDefault(input.Actor, map[string]any{"type": "system", "id": "opl-ledger"}),
		Plan:          cloneMap(input.Plan),
		Approval:      cloneMap(input.Approval),
		Environment:   cloneMap(input.Environment),
		InputRefs:     cloneMapSlice(input.InputRefs),
		ExecutionRefs: cloneMapSlice(input.ExecutionRefs),
		OutputRefs:    cloneMapSlice(input.OutputRefs),
		ReviewResults: cloneMapSlice(input.ReviewResults),
		Continuation:  cloneMap(input.Continuation),
		Metadata:      cloneMap(input.Metadata),
		CreatedAt:     time.Now().UTC(),
	}
	payload, err := json.Marshal(receipt)
	if err != nil {
		return TaskReceipt{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO task_receipts (id, account_id, workspace_id, task_id, receipt_type, status, payload, created_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8)`,
		receipt.ID,
		receipt.AccountID,
		receipt.WorkspaceID,
		receipt.TaskID,
		receipt.Type,
		"recorded",
		payload,
		receipt.CreatedAt,
	)
	if err != nil {
		return TaskReceipt{}, err
	}
	return receipt, nil
}

func (s *PostgresStore) ListTaskReceipts(ctx context.Context, filter TaskReceiptFilter) ([]TaskReceipt, error) {
	if filter.AccountID == "" {
		return nil, errors.New("task_evidence_account_required")
	}
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("workspace_id", filter.WorkspaceID)
	add("task_id", filter.TaskID)
	query := `SELECT payload FROM task_receipts`
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, " AND ")
	}
	query += ` ORDER BY created_at, id`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var receipts []TaskReceipt
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var receipt TaskReceipt
		if err := json.Unmarshal(payload, &receipt); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func (s *PostgresStore) AppendReconciliationReport(ctx context.Context, report ReconciliationReport) (ReconciliationReport, error) {
	if report.ID == "" {
		report.ID = randomID()
	}
	if report.Provider == "" {
		report.Provider = "manual"
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(report.Payload)
	if err != nil {
		return ReconciliationReport{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO billing_reconciliation_reports (
			id, provider, account_id, status, expected_amount_cents, actual_amount_cents, difference_cents, payload, created_at
		) VALUES ($1, $2, '', $3, $4, $5, $6, $7, $8)`,
		report.ID,
		report.Provider,
		report.Status,
		report.ExpectedAmountCents,
		report.LedgerAmountCents,
		report.DifferenceCents,
		payload,
		report.CreatedAt,
	)
	if err != nil {
		return ReconciliationReport{}, err
	}
	return report, nil
}

func (s *PostgresStore) LatestReconciliationReport(ctx context.Context) (ReconciliationReport, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, provider, status, expected_amount_cents, actual_amount_cents, difference_cents, payload, created_at
		FROM billing_reconciliation_reports
		ORDER BY created_at DESC, id DESC
		LIMIT 1`)
	var report ReconciliationReport
	var payload []byte
	err := row.Scan(
		&report.ID,
		&report.Provider,
		&report.Status,
		&report.ExpectedAmountCents,
		&report.LedgerAmountCents,
		&report.DifferenceCents,
		&payload,
		&report.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReconciliationReport{}, errors.New("billing_reconciliation_report_missing")
	}
	if err != nil {
		return ReconciliationReport{}, err
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &report.Payload); err != nil {
			return ReconciliationReport{}, err
		}
	}
	return report, nil
}

const selectLedgerEntryColumns = `SELECT id, event_type, account_id, user_id, workspace_id, compute_id, storage_id, attachment_id, source_event_id, request_fingerprint, amount_cents, currency, created_at`

func ledgerEntryWhere(filter EntryFilter) (string, []any) {
	var clauses []string
	var args []any
	add := func(column string, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	add("account_id", filter.AccountID)
	add("user_id", filter.UserID)
	add("workspace_id", filter.WorkspaceID)
	add("compute_id", filter.ComputeID)
	add("storage_id", filter.StorageID)
	add("attachment_id", filter.AttachmentID)
	add("source_event_id", filter.SourceEventID)
	return strings.Join(clauses, " AND "), args
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

type ledgerEntryScanner interface {
	Scan(dest ...any) error
}

func scanEntry(scanner ledgerEntryScanner) (Entry, error) {
	var entry Entry
	var accountID sql.NullString
	var userID sql.NullString
	var workspaceID sql.NullString
	var computeID sql.NullString
	var storageID sql.NullString
	var attachmentID sql.NullString
	var sourceEventID sql.NullString
	var requestFingerprint sql.NullString
	err := scanner.Scan(
		&entry.ID,
		&entry.EventType,
		&accountID,
		&userID,
		&workspaceID,
		&computeID,
		&storageID,
		&attachmentID,
		&sourceEventID,
		&requestFingerprint,
		&entry.AmountCents,
		&entry.Currency,
		&entry.CreatedAt,
	)
	if err != nil {
		return Entry{}, err
	}
	entry.AccountID = accountID.String
	entry.UserID = userID.String
	entry.WorkspaceID = workspaceID.String
	entry.ComputeID = computeID.String
	entry.StorageID = storageID.String
	entry.AttachmentID = attachmentID.String
	entry.SourceEventID = sourceEventID.String
	entry.RequestFingerprint = requestFingerprint.String
	return entry, nil
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
