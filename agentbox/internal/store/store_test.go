package store

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Enroll("example-api", "abc", "allowlist"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if err := db.PutProfile("codex-primary", 1); err != nil {
		t.Fatalf("PutProfile: %v", err)
	}
	return db
}

func TestEnrollRejectsMismatchedEnrollmentHash(t *testing.T) {
	db := openTestDB(t)
	if err := db.Enroll("example-api", "abc", "allowlist"); err != nil {
		t.Fatalf("idempotent enroll: %v", err)
	}
	err := db.Enroll("example-api", "other", "allowlist")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "enrollment") {
		t.Fatalf("error = %v, want mismatched enrollment hash", err)
	}
}

func TestDuplicateOperationSameDigestReturnsSavedResult(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	req := WriteRequest{ProjectID: "example-api", OperationID: "op-1", Digest: "digest-1", Type: "activate"}
	first, err := db.WithProjectAndProfileWrite(ctx, req, func(tx Tx) error {
		return tx.Activate("gen-1")
	})
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	second, err := db.WithProjectAndProfileWrite(ctx, req, func(tx Tx) error {
		t.Fatal("duplicate digest must not run the writer again")
		return tx.Activate("gen-2")
	})
	if err != nil {
		t.Fatalf("duplicate digest: %v", err)
	}
	if !second.Replay {
		t.Fatal("duplicate digest did not replay the saved result")
	}
	if first.Revision != second.Revision {
		t.Fatalf("revision changed on replay: %d vs %d", first.Revision, second.Revision)
	}
	count, err := db.GenerationCount("example-api")
	if err != nil {
		t.Fatalf("GenerationCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("generation count = %d, want 1", count)
	}
}

func TestDuplicateOperationDifferentDigestFails(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	req := WriteRequest{ProjectID: "example-api", OperationID: "op-1", Digest: "digest-1", Type: "activate"}
	if _, err := db.WithProjectAndProfileWrite(ctx, req, func(tx Tx) error {
		return tx.Activate("gen-1")
	}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	req.Digest = "digest-2"
	_, err := db.WithProjectAndProfileWrite(ctx, req, func(tx Tx) error {
		return tx.Activate("gen-2")
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "digest") {
		t.Fatalf("error = %v, want digest mismatch", err)
	}
}

func TestRevisionMismatchBlocksMutation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.WithProjectAndProfileWrite(ctx, WriteRequest{ProjectID: "example-api", OperationID: "op-1", Digest: "d1", Type: "activate"}, func(tx Tx) error {
		return tx.Activate("gen-1")
	}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	_, err := db.WithProjectAndProfileWrite(ctx, WriteRequest{ProjectID: "example-api", OperationID: "op-2", Digest: "d2", ExpectedRevision: 0, Type: "activate"}, func(tx Tx) error {
		return tx.Activate("gen-2")
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "revision") {
		t.Fatalf("error = %v, want revision mismatch", err)
	}
}

func TestUnknownOperationBlocksWriterMutation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.WithProjectAndProfileWrite(ctx, WriteRequest{ProjectID: "example-api", OperationID: "op-1", Digest: "d1", Type: "activate"}, func(tx Tx) error {
		return tx.SetUnknown()
	}); err != nil {
		t.Fatalf("unknown write: %v", err)
	}
	_, err := db.WithProjectAndProfileWrite(ctx, WriteRequest{ProjectID: "example-api", OperationID: "op-2", Digest: "d2", ExpectedRevision: 1, Type: "activate"}, func(tx Tx) error {
		return tx.Activate("gen-1")
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unknown") {
		t.Fatalf("error = %v, want unknown operation to block writers", err)
	}
}

func TestConcurrentActivationAllowsExactlyOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	setup, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := setup.Enroll("example-api", "abc", "allowlist"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	setup.Close()

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var wins int
	for i, gen := range []string{"gen-a", "gen-b"} {
		wg.Add(1)
		go func(i int, gen string) {
			defer wg.Done()
			db, err := Open(path)
			if err != nil {
				t.Errorf("Open %d: %v", i, err)
				return
			}
			defer db.Close()
			<-start
			_, err = db.WithProjectAndProfileWrite(context.Background(), WriteRequest{ProjectID: "example-api", OperationID: "op-" + gen, Digest: "d-" + gen, Type: "activate"}, func(tx Tx) error {
				return tx.Activate(gen)
			})
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i, gen)
	}
	close(start)
	wg.Wait()
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1", wins)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db.Close()
	count, err := db.GenerationCount("example-api")
	if err != nil {
		t.Fatalf("GenerationCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("generation count = %d, want 1", count)
	}
}

func TestConcurrentRunLocksCredentialProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	setup, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := setup.Enroll("example-api", "abc", "allowlist"); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if err := setup.PutProfile("codex-primary", 1); err != nil {
		t.Fatalf("PutProfile: %v", err)
	}
	setup.Close()

	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var wins int
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			db, err := Open(path)
			if err != nil {
				t.Errorf("Open %d: %v", i, err)
				return
			}
			defer db.Close()
			<-start
			_, err = db.WithProjectAndProfileWrite(context.Background(), WriteRequest{
				ProjectID:   "example-api",
				ProfileID:   "codex-primary",
				OperationID: "op-run-" + string(rune('a'+i)),
				Digest:      "d-run-" + string(rune('a'+i)),
				Type:        "start_run",
			}, func(tx Tx) error {
				return tx.LockProfile()
			})
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if wins != 1 {
		t.Fatalf("credential lock winners = %d, want exactly 1", wins)
	}
}
