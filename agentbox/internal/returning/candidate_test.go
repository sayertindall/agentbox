package returning

import (
	"os"
	"path/filepath"
	"testing"

	"devbox/agentbox/internal/store"
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

func TestPrepareReturnRejectsActiveStartingAndUnknownRun(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	for _, state := range []store.ReceiptState{store.RemoteRunning, store.RemoteStarting, store.UnknownRemoteRun} {
		_, err := Prepare(Input{State: state, Workspace: ws, CandidateRoot: t.TempDir(), Stop: func() error { return nil }})
		if err == nil {
			t.Fatalf("Prepare accepted state %s", state)
		}
	}
}

func TestPrepareReturnStopsCgroupBeforeCandidate(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	stopped := false
	cand, err := Prepare(Input{
		State:         store.Failed,
		Workspace:     ws,
		CandidateRoot: t.TempDir(),
		Stop: func() error {
			if stopped {
				t.Fatal("stop called twice")
			}
			stopped = true
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("cgroup was not stopped")
	}
	if _, err := os.Lstat(filepath.Join(cand.Dir, "main.go")); err != nil {
		t.Fatal(err)
	}
}

func TestReturnManifestIncludesAllowedAgentCreatedFile(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	write(t, ws, "agent.txt", "new\n")
	write(t, ws, ".env", "SECRET=1\n")
	cand, err := Prepare(Input{State: store.Failed, Workspace: ws, CandidateRoot: t.TempDir(), Stop: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, e := range cand.Manifest.Entries {
		paths[e.Path] = true
	}
	if !paths["agent.txt"] || !paths["main.go"] {
		t.Fatalf("return manifest = %v", paths)
	}
	if paths[".env"] {
		t.Fatal(".env entered the return manifest")
	}
}

func TestCandidateContainsOnlyReturnManifestPaths(t *testing.T) {
	ws := t.TempDir()
	write(t, ws, "main.go", "package main\n")
	write(t, ws, ".env", "SECRET=1\n")
	cand, err := Prepare(Input{State: store.Failed, Workspace: ws, CandidateRoot: t.TempDir(), Stop: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(cand.Dir, ".env")); err == nil {
		t.Fatal(".env copied into the candidate")
	}
}
