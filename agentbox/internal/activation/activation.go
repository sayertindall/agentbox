package activation

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"devbox/agentbox/internal/id"
	"devbox/agentbox/internal/manifest"
)

type Input struct {
	ProjectID        string
	SourceStaging    string
	BaselineStaging  string
	SourceManifest   manifest.Manifest
	BaselineManifest manifest.Manifest
	Packet           []byte
	GenerationsRoot  string
}

type Result struct {
	Generation string
	Workspace  string
	Packet     string
	GitDir     string
}

func Stage(tree string, declared manifest.Manifest) error {
	actual, err := manifest.Build(tree, manifest.Policy{})
	if err != nil {
		return fmt.Errorf("rewalk staging: %w", err)
	}
	if err := manifest.Compare(declared, actual); err != nil {
		return fmt.Errorf("staging does not match manifest: %w", err)
	}
	return nil
}

func Activate(in Input) (Result, error) {
	projectID, err := id.ParseProjectID(in.ProjectID)
	if err != nil {
		return Result{}, err
	}
	if in.GenerationsRoot == "" {
		return Result{}, fmt.Errorf("generations root is required")
	}
	if err := Stage(in.SourceStaging, in.SourceManifest); err != nil {
		return Result{}, fmt.Errorf("source staging: %w", err)
	}
	if err := Stage(in.BaselineStaging, in.BaselineManifest); err != nil {
		return Result{}, fmt.Errorf("baseline staging: %w", err)
	}

	generation, err := newID()
	if err != nil {
		return Result{}, err
	}
	root := filepath.Join(in.GenerationsRoot, projectID.String(), generation)
	workspace := filepath.Join(root, "source")
	packetPath := filepath.Join(root, "packet.json")
	gitDir := filepath.Join(root, "git")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return Result{}, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(root)
		}
	}()

	if err = materializeTree(in.BaselineStaging, in.BaselineManifest, workspace); err != nil {
		return Result{}, fmt.Errorf("materialize baseline: %w", err)
	}
	if err = initSyntheticGit(gitDir, workspace); err != nil {
		return Result{}, err
	}
	if err = clearDir(workspace); err != nil {
		return Result{}, err
	}
	if err = materializeTree(in.SourceStaging, in.SourceManifest, workspace); err != nil {
		return Result{}, fmt.Errorf("materialize source: %w", err)
	}
	if err = Stage(workspace, in.SourceManifest); err != nil {
		return Result{}, fmt.Errorf("activated workspace: %w", err)
	}
	if err = os.WriteFile(packetPath, in.Packet, 0o600); err != nil {
		return Result{}, fmt.Errorf("write packet: %w", err)
	}
	return Result{Generation: generation, Workspace: workspace, Packet: packetPath, GitDir: gitDir}, nil
}

func materializeTree(src string, m manifest.Manifest, dest string) error {
	root, err := os.OpenRoot(dest)
	if err != nil {
		return err
	}
	defer root.Close()
	return manifest.Materialize(src, m, root)
}

func initSyntheticGit(gitDir, workTree string) error {
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		return err
	}
	if err := runGit(gitDir, workTree, "init", "-q"); err != nil {
		return err
	}
	if err := runGit(gitDir, workTree, "add", "-A"); err != nil {
		return err
	}
	return runGit(gitDir, workTree, "commit", "-q", "--allow-empty", "-m", "sanitized baseline")
}

func runGit(gitDir, workTree string, args ...string) error {
	full := append([]string{"--git-dir", gitDir, "--work-tree", workTree, "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=devbox",
		"GIT_AUTHOR_EMAIL=devbox@example.invalid",
		"GIT_COMMITTER_NAME=devbox",
		"GIT_COMMITTER_EMAIL=devbox@example.invalid",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %v: %w: %s", args, err, out)
	}
	return nil
}

func clearDir(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func newID() (string, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
