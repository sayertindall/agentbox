package integration

import (
	"os"
	"path/filepath"
	"testing"

	"devbox/agentbox/internal/activation"
	"devbox/agentbox/internal/manifest"
)

func TestStageRejectsExtraMissingOrMismatchedPath(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	declared, err := manifest.Build(src, manifest.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extra.txt"), []byte("nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := activation.Stage(src, declared); err == nil {
		t.Fatal("extra staging path activated")
	}
}
