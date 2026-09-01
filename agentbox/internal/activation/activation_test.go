package activation

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"devbox/agentbox/internal/manifest"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func openRoot(t *testing.T, path string) *os.Root {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

func materialize(t *testing.T, src, dst string) manifest.Manifest {
	t.Helper()
	m, err := manifest.Build(src, manifest.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Materialize(src, m, openRoot(t, dst)); err != nil {
		t.Fatal(err)
	}
	return m
}

func gitOutput(t *testing.T, gitDir, workTree string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"--git-dir", gitDir, "--work-tree", workTree}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=devbox",
		"GIT_AUTHOR_EMAIL=devbox@example.invalid",
		"GIT_COMMITTER_NAME=devbox",
		"GIT_COMMITTER_EMAIL=devbox@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func TestStageRejectsExtraMissingOrMismatchedPath(t *testing.T) {
	src := t.TempDir()
	write(t, src, "main.go", "package main\n")
	declared, err := manifest.Build(src, manifest.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	write(t, src, "extra.txt", "nope\n")
	if err := Stage(src, declared); err == nil {
		t.Fatal("Stage accepted an extra path")
	}

	missing := t.TempDir()
	if err := Stage(missing, declared); err == nil {
		t.Fatal("Stage accepted a missing tree")
	}

	mismatch := t.TempDir()
	write(t, mismatch, "main.go", "package other\n")
	if err := Stage(mismatch, declared); err == nil {
		t.Fatal("Stage accepted mismatched content")
	}
}

func TestActivationSourceWorkspaceHasOnlyManifestPaths(t *testing.T) {
	source := t.TempDir()
	write(t, source, "main.go", "package main\n")
	write(t, source, "sub/a.txt", "a\n")
	write(t, source, ".env", "TOKEN=secret\n")
	srcStaging := filepath.Join(t.TempDir(), "source")
	baseStaging := filepath.Join(t.TempDir(), "baseline")
	srcM := materialize(t, source, srcStaging)
	baseM := materialize(t, source, baseStaging)

	result, err := Activate(Input{
		ProjectID:        "example-api",
		SourceStaging:    srcStaging,
		BaselineStaging:  baseStaging,
		SourceManifest:   srcM,
		BaselineManifest: baseM,
		Packet:           []byte(`{"version":1}`),
		GenerationsRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	entries, err := os.ReadDir(result.Workspace)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name()] = true
	}
	if !names["main.go"] || !names["sub"] {
		t.Fatalf("workspace entries = %v", names)
	}
	if names[".env"] || names[".git"] || names["packet.json"] {
		t.Fatalf("workspace leaked runtime or secret paths: %v", names)
	}
}

func TestActivationPlacesPacketOutsideWorkspace(t *testing.T) {
	source := t.TempDir()
	write(t, source, "main.go", "package main\n")
	srcStaging := filepath.Join(t.TempDir(), "source")
	baseStaging := filepath.Join(t.TempDir(), "baseline")
	srcM := materialize(t, source, srcStaging)
	result, err := Activate(Input{
		ProjectID:        "example-api",
		SourceStaging:    srcStaging,
		BaselineStaging:  baseStaging,
		SourceManifest:   srcM,
		BaselineManifest: materialize(t, source, baseStaging),
		Packet:           []byte(`{"task":"fix it"}`),
		GenerationsRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(result.Workspace, "packet.json")); err == nil {
		t.Fatal("packet landed inside the source workspace")
	}
	data, err := os.ReadFile(result.Packet)
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if string(data) != `{"task":"fix it"}` {
		t.Fatalf("packet = %q", data)
	}
	if rel, _ := filepath.Rel(result.Workspace, result.Packet); rel == "packet.json" || !strings.HasPrefix(rel, "..") {
		t.Fatalf("packet %s is not outside workspace %s", result.Packet, result.Workspace)
	}
}

func TestActivationPlacesGitDirectoryOutsideWorkspace(t *testing.T) {
	source := t.TempDir()
	write(t, source, "main.go", "package main\n")
	srcStaging := filepath.Join(t.TempDir(), "source")
	baseStaging := filepath.Join(t.TempDir(), "baseline")
	srcM := materialize(t, source, srcStaging)
	result, err := Activate(Input{
		ProjectID:        "example-api",
		SourceStaging:    srcStaging,
		BaselineStaging:  baseStaging,
		SourceManifest:   srcM,
		BaselineManifest: materialize(t, source, baseStaging),
		Packet:           []byte(`{}`),
		GenerationsRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(result.Workspace, ".git")); err == nil {
		t.Fatal(".git landed inside the source workspace")
	}
	if _, err := os.Lstat(filepath.Join(result.GitDir, "HEAD")); err != nil {
		t.Fatalf("synthetic git dir missing HEAD: %v", err)
	}
}

func TestActivationRemovesDeletedBaselinePath(t *testing.T) {
	source := t.TempDir()
	write(t, source, "keep.txt", "keep\n")
	write(t, source, "gone.txt", "gone\n")
	baseStaging := filepath.Join(t.TempDir(), "baseline")
	baseM := materialize(t, source, baseStaging)
	if err := os.Remove(filepath.Join(source, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	srcStaging := filepath.Join(t.TempDir(), "source")
	srcM := materialize(t, source, srcStaging)
	result, err := Activate(Input{
		ProjectID:        "example-api",
		SourceStaging:    srcStaging,
		BaselineStaging:  baseStaging,
		SourceManifest:   srcM,
		BaselineManifest: baseM,
		Packet:           []byte(`{}`),
		GenerationsRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(result.Workspace, "gone.txt")); err == nil {
		t.Fatal("deleted baseline path survived in the workspace")
	}
	if got := gitOutput(t, result.GitDir, result.Workspace, "diff", "--name-only"); !strings.Contains(got, "gone.txt") {
		t.Fatalf("synthetic diff = %q, want gone.txt deleted", got)
	}
}

func TestActivationExcludesTrackedSecret(t *testing.T) {
	source := t.TempDir()
	write(t, source, "main.go", "package main\n")
	write(t, source, ".env", "TOKEN=secret\n")
	srcStaging := filepath.Join(t.TempDir(), "source")
	baseStaging := filepath.Join(t.TempDir(), "baseline")
	srcM := materialize(t, source, srcStaging)
	result, err := Activate(Input{
		ProjectID:        "example-api",
		SourceStaging:    srcStaging,
		BaselineStaging:  baseStaging,
		SourceManifest:   srcM,
		BaselineManifest: materialize(t, source, baseStaging),
		Packet:           []byte(`{}`),
		GenerationsRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(result.Workspace, ".env")); err == nil {
		t.Fatal(".env entered the activated workspace")
	}
}

func TestNonGitActivationCreatesSyntheticDiff(t *testing.T) {
	source := t.TempDir()
	write(t, source, "main.go", "package main\n")
	srcStaging := filepath.Join(t.TempDir(), "source")
	baseStaging := filepath.Join(t.TempDir(), "baseline")
	srcM := materialize(t, source, srcStaging)
	result, err := Activate(Input{
		ProjectID:        "example-api",
		SourceStaging:    srcStaging,
		BaselineStaging:  baseStaging,
		SourceManifest:   srcM,
		BaselineManifest: materialize(t, source, baseStaging),
		Packet:           []byte(`{}`),
		GenerationsRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if log := gitOutput(t, result.GitDir, result.Workspace, "log", "--oneline"); strings.TrimSpace(log) == "" {
		t.Fatal("non-Git activation produced no synthetic commit")
	}
	if diff := gitOutput(t, result.GitDir, result.Workspace, "diff"); strings.TrimSpace(diff) != "" {
		t.Fatalf("non-Git synthetic diff = %q, want empty before provider edits", diff)
	}
}
