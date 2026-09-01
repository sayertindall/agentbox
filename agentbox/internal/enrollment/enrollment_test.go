package enrollment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrollmentCreatesRandomUUIDAndHash(t *testing.T) {
	first, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second, err := New()
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if first.EnrollmentID == second.EnrollmentID {
		t.Fatal("New() returned the same enrollment ID twice")
	}
	if len(first.EnrollmentHash) != 64 || strings.Trim(first.EnrollmentHash, "0123456789abcdef") != "" {
		t.Fatalf("EnrollmentHash = %q, want lowercase SHA-256 hex", first.EnrollmentHash)
	}
	if first.EnrollmentHash != Hash(first.EnrollmentID) {
		t.Fatalf("EnrollmentHash = %q, want hash of enrollment ID", first.EnrollmentHash)
	}
}

func TestEnrollmentLoadRejectsMalformedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enrollment.json")
	if err := os.WriteFile(path, []byte(`{"enrollment_id":"not-a-uuid","enrollment_hash":"bad"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load() accepted malformed enrollment record")
	}
}
