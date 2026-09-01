package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"devbox/agentbox/internal/manifest"
	"devbox/agentbox/internal/quota"
	"devbox/agentbox/internal/transfer"
)

func TestExtraStagingPathNeverActivates(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	declared, err := manifest.Build(source, manifest.Policy{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, "extra.txt"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actual, err := manifest.Build(source, manifest.Policy{})
	if err != nil {
		t.Fatalf("Build actual: %v", err)
	}
	err = manifest.Compare(declared, actual)
	if err == nil || !strings.Contains(err.Error(), "extra.txt") {
		t.Fatalf("error = %v, want extra path rejected", err)
	}
}

func TestOverQuotaStagingNeverActivates(t *testing.T) {
	q := quota.NewMemory()
	if err := q.Assign("/staging/token", 4); err != nil {
		t.Fatal(err)
	}
	activated := false
	if err := q.Charge("/staging/token", 8); err == nil {
		activated = true
	}
	if activated {
		t.Fatal("over-quota staging activated a generation")
	}
	_ = transfer.FilesFrom(manifest.Manifest{})
}

func TestTransferAuthorizedKeyDisablesForwarding(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "deploy", "ssh", "authorized_keys.transfer.example"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, flag := range []string{"no-port-forwarding", "no-agent-forwarding", "no-X11-forwarding", "no-pty", "no-user-rc"} {
		if !strings.Contains(text, flag) {
			t.Fatalf("transfer authorized_keys missing %s", flag)
		}
	}
}
