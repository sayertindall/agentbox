package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"devbox/agentbox/internal/id"

	sqlite "modernc.org/sqlite"
)

type ReceiptState string

const (
	LocalOwned               ReceiptState = "local_owned"
	Staging                  ReceiptState = "staging"
	RemoteOwned              ReceiptState = "remote_owned"
	RemoteStarting           ReceiptState = "remote_starting"
	RemoteRunning            ReceiptState = "remote_running"
	UnknownRemoteRun         ReceiptState = "unknown_remote_run"
	Failed                   ReceiptState = "failed"
	Returning                ReceiptState = "returning"
	Conflicted               ReceiptState = "conflicted"
	Closed                   ReceiptState = "closed"
	ArchivedCleanupPending   ReceiptState = "archived_cleanup_pending"
)

const (
	opKnown   = "known"
	opUnknown = "unknown"
)

type DB struct {
	sql  *sql.DB
	path string
}
type WriteRequest struct {
	ProjectID        string
	ProfileID        string
	OperationID      string
	Digest           string
	ExpectedRevision int64
	Type             string
}

type Outcome struct {
	Replay   bool
	Result   string
	Revision int64
}

type Tx interface {
	Activate(generation string) error
	LockProfile() error
	SetUnknown() error
}

type tx struct {
	ctx     context.Context
	sql     *sql.Tx
	req     WriteRequest
	result  string
	unknown bool
}

func Open(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open control database: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}
	var last error
	for range 50 {
		if _, last = sqlDB.Exec(schema); last == nil {
			return &DB{sql: sqlDB, path: path}, nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	sqlDB.Close()
	return nil, fmt.Errorf("apply control schema: %w", last)
}

func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

func (db *DB) Enroll(projectID, enrollmentHash, sourcePolicy string) error {
	if _, err := id.ParseProjectID(projectID); err != nil {
		return err
	}
	if enrollmentHash == "" {
		return fmt.Errorf("enrollment hash is required")
	}
	tx, err := db.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existing string
	err = tx.QueryRow(`SELECT enrollment_hash FROM projects WHERE project_id = ?`, projectID).Scan(&existing)
	if err == nil {
		if existing != enrollmentHash {
			return fmt.Errorf("enrollment hash does not match registered project")
		}
		return tx.Commit()
	}
	if err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO projects (project_id, enrollment_hash, source_policy) VALUES (?, ?, ?)`, projectID, enrollmentHash, sourcePolicy); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO receipts (project_id, revision, state) VALUES (?, 0, ?)`, projectID, LocalOwned); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO source_leases (project_id, state) VALUES (?, ?)`, projectID, LocalOwned); err != nil {
		return err
	}
	return tx.Commit()
}

func (db *DB) PutProfile(profileID string, maxActive int) error {
	if profileID == "" || maxActive <= 0 {
		return fmt.Errorf("invalid credential profile")
	}
	_, err := db.sql.Exec(`INSERT OR REPLACE INTO credential_profiles (id, max_active_runs) VALUES (?, ?)`, profileID, maxActive)
	return err
}

func (db *DB) WithProjectAndProfileWrite(ctx context.Context, req WriteRequest, fn func(Tx) error) (Outcome, error) {
	if _, err := id.ParseProjectID(req.ProjectID); err != nil {
		return Outcome{}, err
	}
	if req.OperationID == "" || req.Digest == "" {
		return Outcome{}, fmt.Errorf("operation id and digest are required")
	}
	sqlTx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return Outcome{}, err
	}
	defer sqlTx.Rollback()
	if _, err := sqlTx.Exec(`BEGIN IMMEDIATE`); err != nil {
		// Nested BEGIN IMMEDIATE fails because BeginTx already started a transaction.
		// Use a deferred transaction upgrade instead: sqlite BeginTx is deferred by default.
	}
	// BeginTx is deferred; take the write lock explicitly.
	if _, err := sqlTx.Exec(`UPDATE control_gate SET writers_blocked = writers_blocked WHERE id = 1`); err != nil {
		return Outcome{}, fmt.Errorf("lock control gate: %w", err)
	}

	var blocked int
	if err := sqlTx.QueryRow(`SELECT writers_blocked FROM control_gate WHERE id = 1`).Scan(&blocked); err != nil {
		return Outcome{}, err
	}
	if blocked != 0 {
		return Outcome{}, fmt.Errorf("control writers are blocked")
	}

	var enrolled string
	if err := sqlTx.QueryRow(`SELECT enrollment_hash FROM projects WHERE project_id = ?`, req.ProjectID).Scan(&enrolled); err != nil {
		if err == sql.ErrNoRows {
			return Outcome{}, fmt.Errorf("unknown project")
		}
		return Outcome{}, err
	}

	var unknownCount int
	if err := sqlTx.QueryRow(`SELECT COUNT(*) FROM operations WHERE project_id = ? AND state = ?`, req.ProjectID, opUnknown).Scan(&unknownCount); err != nil {
		return Outcome{}, err
	}

	var existingDigest, existingState, existingResult string
	err = sqlTx.QueryRow(`SELECT request_digest, state, COALESCE(result, '') FROM operations WHERE operation_id = ?`, req.OperationID).Scan(&existingDigest, &existingState, &existingResult)
	switch {
	case err == nil:
		if existingDigest != req.Digest {
			return Outcome{}, fmt.Errorf("operation digest does not match saved request")
		}
		var revision int64
		if err := sqlTx.QueryRow(`SELECT revision FROM receipts WHERE project_id = ?`, req.ProjectID).Scan(&revision); err != nil {
			return Outcome{}, err
		}
		if err := sqlTx.Commit(); err != nil {
			return Outcome{}, fmt.Errorf("commit ambiguous: %w", err)
		}
		return Outcome{Replay: true, Result: existingResult, Revision: revision}, nil
	case err != sql.ErrNoRows:
		return Outcome{}, err
	}

	if unknownCount > 0 {
		return Outcome{}, fmt.Errorf("unknown operation blocks writer mutation")
	}

	var revision int64
	var receiptState string
	if err := sqlTx.QueryRow(`SELECT revision, state FROM receipts WHERE project_id = ?`, req.ProjectID).Scan(&revision, &receiptState); err != nil {
		return Outcome{}, err
	}
	if revision != req.ExpectedRevision {
		return Outcome{}, fmt.Errorf("receipt revision mismatch")
	}

	if req.ProfileID != "" {
		var maxActive int
		if err := sqlTx.QueryRow(`SELECT max_active_runs FROM credential_profiles WHERE id = ?`, req.ProfileID).Scan(&maxActive); err != nil {
			return Outcome{}, fmt.Errorf("unknown credential profile")
		}
	}

	if _, err := sqlTx.Exec(`INSERT INTO operations (operation_id, project_id, type, request_digest, state) VALUES (?, ?, ?, ?, ?)`, req.OperationID, req.ProjectID, req.Type, req.Digest, opKnown); err != nil {
		return Outcome{}, err
	}

	writer := &tx{ctx: ctx, sql: sqlTx, req: req}
	if err := fn(writer); err != nil {
		return Outcome{}, err
	}

	state := opKnown
	if writer.unknown {
		state = opUnknown
	}
	if _, err := sqlTx.Exec(`UPDATE operations SET state = ?, result = ? WHERE operation_id = ?`, state, writer.result, req.OperationID); err != nil {
		return Outcome{}, err
	}
	if _, err := sqlTx.Exec(`UPDATE receipts SET revision = revision + 1 WHERE project_id = ?`, req.ProjectID); err != nil {
		return Outcome{}, err
	}
	if _, err := sqlTx.Exec(`INSERT INTO events (at, actor, old_state, new_state, result) VALUES (?, ?, ?, ?, ?)`, time.Now().UTC().Format(time.RFC3339), req.OperationID, receiptState, writer.result, state); err != nil {
		return Outcome{}, err
	}
	if err := sqlTx.Commit(); err != nil {
		return Outcome{}, fmt.Errorf("commit ambiguous: %w", err)
	}
	return Outcome{Result: writer.result, Revision: revision + 1}, nil
}

func (t *tx) Activate(generation string) error {
	if generation == "" {
		return fmt.Errorf("generation is required")
	}
	var lease string
	if err := t.sql.QueryRow(`SELECT state FROM source_leases WHERE project_id = ?`, t.req.ProjectID).Scan(&lease); err != nil {
		return err
	}
	if lease != string(LocalOwned) {
		return fmt.Errorf("source lease is not local_owned")
	}
	if _, err := t.sql.Exec(`INSERT INTO generations (project_id, generation) VALUES (?, ?)`, t.req.ProjectID, generation); err != nil {
		return fmt.Errorf("generation already exists")
	}
	if _, err := t.sql.Exec(`UPDATE receipts SET state = ?, generation = ? WHERE project_id = ?`, RemoteOwned, generation, t.req.ProjectID); err != nil {
		return err
	}
	if _, err := t.sql.Exec(`UPDATE source_leases SET state = ?, generation = ?, operation_id = ? WHERE project_id = ?`, RemoteOwned, generation, t.req.OperationID, t.req.ProjectID); err != nil {
		return err
	}
	t.result = string(RemoteOwned)
	return nil
}

func (t *tx) LockProfile() error {
	if t.req.ProfileID == "" {
		return fmt.Errorf("credential profile is required")
	}
	var maxActive, used int
	if err := t.sql.QueryRow(`SELECT max_active_runs FROM credential_profiles WHERE id = ?`, t.req.ProfileID).Scan(&maxActive); err != nil {
		return fmt.Errorf("unknown credential profile")
	}
	if err := t.sql.QueryRow(`SELECT COUNT(*) FROM credential_locks WHERE profile_id = ?`, t.req.ProfileID).Scan(&used); err != nil {
		return err
	}
	if used >= maxActive {
		return fmt.Errorf("credential profile is locked")
	}
	if _, err := t.sql.Exec(`INSERT INTO credential_locks (profile_id, active_operation_id) VALUES (?, ?)`, t.req.ProfileID, t.req.OperationID); err != nil {
		return fmt.Errorf("credential profile is locked")
	}
	t.result = "locked"
	return nil
}

func (t *tx) SetUnknown() error {
	t.unknown = true
	t.result = string(UnknownRemoteRun)
	return nil
}

func (db *DB) GenerationCount(projectID string) (int, error) {
	var count int
	err := db.sql.QueryRow(`SELECT COUNT(*) FROM generations WHERE project_id = ?`, projectID).Scan(&count)
	return count, err
}

type ReceiptView struct {
	ProjectID  string
	Revision   int64
	State      ReceiptState
	Generation string
}

func (db *DB) Receipt(projectID string) (ReceiptView, error) {
	var view ReceiptView
	var generation sql.NullString
	err := db.sql.QueryRow(`SELECT revision, state, generation FROM receipts WHERE project_id = ?`, projectID).Scan(&view.Revision, &view.State, &generation)
	if err != nil {
		return ReceiptView{}, err
	}
	view.ProjectID = projectID
	view.Generation = generation.String
	return view, nil
}

func (db *DB) SetWritersBlocked(blocked bool) error {
	value := 0
	if blocked {
		value = 1
	}
	_, err := db.sql.Exec(`UPDATE control_gate SET writers_blocked = ? WHERE id = 1`, value)
	return err
}
func (db *DB) Path() string { return db.path }

func (db *DB) BackupTo(ctx context.Context, dest string) error {
	conn, err := db.sql.Conn(ctx)
	if err != nil {
		return fmt.Errorf("control connection: %w", err)
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) error {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("sqlite backup API is unavailable")
		}
		bck, err := backuper.NewBackup(dest)
		if err != nil {
			return err
		}
		if _, err := bck.Step(-1); err != nil {
			bck.Finish()
			return err
		}
		return bck.Finish()
	})
}

func (db *DB) PutActiveRun(projectID, unit string) error {
	if unit == "" {
		return fmt.Errorf("run unit is required")
	}
	if _, err := db.sql.Exec(`INSERT INTO runs (run_id, project_id, unit, state) VALUES (?, ?, ?, ?)`, unit, projectID, unit, RemoteRunning); err != nil {
		return err
	}
	_, err := db.sql.Exec(`UPDATE receipts SET state = ? WHERE project_id = ?`, RemoteRunning, projectID)
	return err
}

func (db *DB) ActiveUnits() ([]string, error) {
	rows, err := db.sql.Query(`SELECT unit FROM runs WHERE state IN (?, ?, ?)`, RemoteStarting, RemoteRunning, UnknownRemoteRun)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var units []string
	for rows.Next() {
		var unit string
		if err := rows.Scan(&unit); err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, rows.Err()
}

func (db *DB) RecordBackup(path string) error {
	_, err := db.sql.Exec(`INSERT INTO backups (path, created_at, verified) VALUES (?, ?, 1)`, path, time.Now().UTC().Format(time.RFC3339))
	return err
}
