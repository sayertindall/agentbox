package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devbox/agentbox/internal/store"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Enroll("example-api", "abc", "allowlist"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	return db
}

func TestBackupCreatesVerifiedDatabaseCopy(t *testing.T) {
	db := testDB(t)
	svc := New(db, t.TempDir())
	path, err := svc.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	copyDB, err := store.Open(path)
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer copyDB.Close()
	if err := copyDB.Enroll("example-api", "other", "allowlist"); err == nil {
		t.Fatal("backup did not preserve the enrolled project")
	}
}

func TestBackupUsesTemporaryFileAndAtomicRename(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	svc := New(db, dir)
	path, err := svc.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("final backup missing: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") || strings.Contains(entry.Name(), ".tmp.") {
			t.Fatalf("temporary backup file left behind: %s", entry.Name())
		}
	}
}

func TestBackupRetentionSkipsNewestVerifiedCopy(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	svc := New(db, dir)
	first, err := svc.Create()
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(first, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	newest, err := svc.Create()
	if err != nil {
		t.Fatalf("second Create: %v", err)
	}
	if err := svc.Prune(time.Hour); err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if _, err := os.Lstat(newest); err != nil {
		t.Fatalf("newest backup was deleted: %v", err)
	}
	if _, err := os.Lstat(first); err == nil {
		t.Fatal("old backup was kept")
	}
}

func TestRestoreDrillReconcilesActiveRunsBeforeReplacement(t *testing.T) {
	db := testDB(t)
	if err := db.PutActiveRun("example-api", "devbox-run-1.service"); err != nil {
		t.Fatalf("PutActiveRun: %v", err)
	}
	dir := t.TempDir()
	svc := New(db, dir)
	if _, err := svc.Create(); err != nil {
		t.Fatalf("Create: %v", err)
	}
	original := db.Path()
	before, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("read production db: %v", err)
	}
	err = svc.Drill(func(unit string) bool { return false })
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "reconcil") {
		t.Fatalf("error = %v, want reconciliation failure", err)
	}
	after, err := os.ReadFile(original)
	if err != nil {
		t.Fatalf("reread production db: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("restore drill replaced production control state")
	}
}

func TestRestoreControlBlocksWritersUntilLiveRunReconciliation(t *testing.T) {
	db := testDB(t)
	if err := db.PutActiveRun("example-api", "devbox-run-1.service"); err != nil {
		t.Fatalf("PutActiveRun: %v", err)
	}
	svc := New(db, t.TempDir())
	path, err := svc.Create()
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	err = svc.RestoreControl(path, func(unit string) bool { return false })
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "reconcil") {
		t.Fatalf("error = %v, want reconciliation failure", err)
	}
	_, writeErr := db.WithProjectAndProfileWrite(context.Background(), store.WriteRequest{
		ProjectID:   "example-api",
		OperationID: "op-blocked",
		Digest:      "digest-blocked",
		Type:        "activate",
	}, func(tx store.Tx) error {
		return tx.Activate("gen-blocked")
	})
	if writeErr == nil || !strings.Contains(strings.ToLower(writeErr.Error()), "block") {
		t.Fatalf("writer error = %v, want writers blocked", writeErr)
	}
}
