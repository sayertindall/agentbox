package credential

import (
	"testing"
)

func TestCredentialProfileAllowsOneActiveRun(t *testing.T) {
	store := t.TempDir()
	profile, err := New(Profile{
		ID:                         "codex-primary",
		Provider:                   "codex",
		StoreDir:                   store,
		Adaptor:                    "systemd-credentials",
		EgressPolicyID:             "codex",
		MaxActiveRuns:              1,
		Verified:                   true,
		RevocationChecked:          true,
		EgressChecked:              true,
		Material:                   []byte("sk-test"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := profile.Lock("op-1"); err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if err := profile.Lock("op-2"); err == nil {
		t.Fatal("second lock succeeded")
	}
	if err := profile.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	if err := profile.Lock("op-2"); err != nil {
		t.Fatalf("lock after unlock: %v", err)
	}
}

func TestUnverifiedProfileCannotStart(t *testing.T) {
	_, err := New(Profile{ID: "x", Provider: "codex", StoreDir: t.TempDir(), Adaptor: "systemd-credentials", MaxActiveRuns: 1, Material: []byte("x")})
	if err == nil {
		t.Fatal("unverified profile was accepted")
	}
}
