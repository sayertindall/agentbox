package transfer

import (
	"strings"
	"testing"
	"time"

	"devbox/agentbox/internal/manifest"
)

func TestTokenHasQuotaAndExpiry(t *testing.T) {
	token, err := Issue(100, 50, 25, time.Minute)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if token.QuotaBytes != 175 {
		t.Fatalf("quota = %d, want 175", token.QuotaBytes)
	}
	if time.Until(token.ExpiresAt) <= 0 {
		t.Fatal("token already expired")
	}
	expired, err := Issue(1, 1, 1, -time.Second)
	if err != nil {
		t.Fatalf("Issue expired: %v", err)
	}
	if err := expired.Consume(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "expir") {
		t.Fatalf("error = %v, want expired token", err)
	}
	if err := token.Consume(); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if err := token.Consume(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "one-time") && !strings.Contains(strings.ToLower(err.Error()), "used") {
		t.Fatalf("error = %v, want one-time token", err)
	}
}

func TestRsyncUsesManifestFilesFromList(t *testing.T) {
	m := manifest.Manifest{
		Version: manifest.Version,
		Entries: []manifest.Entry{
			{Path: "README.md", Kind: manifest.KindFile, SHA256: strings.Repeat("ab", 32)},
			{Path: "main.go", Kind: manifest.KindFile, SHA256: strings.Repeat("cd", 32)},
		},
	}
	list := FilesFrom(m)
	if strings.Join(list, ",") != "README.md,main.go" {
		t.Fatalf("files-from = %v", list)
	}
	args := Command("/tmp/files-from", "/src", "/staging/token/source")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--files-from=/tmp/files-from") {
		t.Fatalf("rsync args missing --files-from: %v", args)
	}
}

func TestWrapperRejectsShellAndParentPath(t *testing.T) {
	root := "/srv/devbox/staging/token"
	if err := Allow([]string{"bash", "-c", "cat /etc/passwd"}, root); err == nil {
		t.Fatal("wrapper accepted a shell")
	}
	if err := Allow([]string{"rsync", "--server", ".", root + "/../other"}, root); err == nil {
		t.Fatal("wrapper accepted a parent path")
	}
	if err := Allow([]string{"rsync", "--server", ".", root + "/source"}, root); err != nil {
		t.Fatalf("allowed dest rejected: %v", err)
	}
}
