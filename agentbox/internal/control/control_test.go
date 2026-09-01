package control

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromEnvUsesXDGConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DEVBOX_CONTROL", "")
	t.Setenv("DEVBOX_TRANSFER", "")
	if _, err := FromEnv(); err != ErrUnavailable {
		t.Fatalf("empty home: %v", err)
	}
	dir := filepath.Join(home, ".config", "agentbox")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("control = \"devbox-control\"\ntransfer = \"devbox-transfer\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if c.Host != "devbox-control" || c.Transfer != "devbox-transfer" {
		t.Fatalf("config = %+v", c)
	}
}
