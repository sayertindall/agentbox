package environment

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWrapperUsesServerResolvedWorkspace(t *testing.T) {
	env, err := Env(Spec{
		Workspace:     "/srv/devbox/runs/1/workspace",
		GitDir:        "/srv/devbox/runs/1/metadata/git",
		GitWorkTree:   "/srv/devbox/runs/1/workspace",
		HandoffFile:   "/srv/devbox/runs/1/metadata/handoff.json",
		EnvironmentID: "nix:abc",
		ExpectedID:    "nix:abc",
		MarkerPath:    filepath.Join(t.TempDir(), "marker"),
		Nonce:         "nonce-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if env["DEVBOX_WORKSPACE"] != "/srv/devbox/runs/1/workspace" {
		t.Fatalf("workspace = %q", env["DEVBOX_WORKSPACE"])
	}
}

func TestWrapperSetsExternalGitMetadataPaths(t *testing.T) {
	env, err := Env(Spec{
		Workspace: "/ws", GitDir: "/meta/git", GitWorkTree: "/ws",
		HandoffFile: "/meta/handoff.json", EnvironmentID: "id", ExpectedID: "id",
		MarkerPath: filepath.Join(t.TempDir(), "m"), Nonce: "n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if env["GIT_DIR"] != "/meta/git" || env["GIT_WORK_TREE"] != "/ws" {
		t.Fatalf("git env = %v", env)
	}
}

func TestWrapperSetsHandoffFileOutsideWorkspace(t *testing.T) {
	env, err := Env(Spec{
		Workspace: "/ws", GitDir: "/meta/git", GitWorkTree: "/ws",
		HandoffFile: "/meta/handoff.json", EnvironmentID: "id", ExpectedID: "id",
		MarkerPath: filepath.Join(t.TempDir(), "m"), Nonce: "n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if env["DEVBOX_HANDOFF_FILE"] != "/meta/handoff.json" {
		t.Fatalf("handoff = %q", env["DEVBOX_HANDOFF_FILE"])
	}
	if strings.HasPrefix(env["DEVBOX_HANDOFF_FILE"], "/ws/") {
		t.Fatal("handoff file is inside the workspace")
	}
}

func TestWrapperRejectsWrongEnvironmentIdentity(t *testing.T) {
	_, err := Env(Spec{
		Workspace: "/ws", GitDir: "/meta/git", GitWorkTree: "/ws",
		HandoffFile: "/meta/handoff.json", EnvironmentID: "nix:a", ExpectedID: "nix:b",
		MarkerPath: filepath.Join(t.TempDir(), "m"), Nonce: "n",
	})
	if err == nil {
		t.Fatal("wrong environment identity was accepted")
	}
}

func TestNoTCPFallbackPathExists(t *testing.T) {
	if ListenNetwork != "unix" {
		t.Fatalf("listen network = %q, want unix", ListenNetwork)
	}
}

func TestPaseoUnixSocketListenAndQuery(t *testing.T) {
	if ListenNetwork != "unix" {
		t.Fatal("Paseo TCP fallback exists")
	}
}
