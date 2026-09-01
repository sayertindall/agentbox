package returning

import (
	"fmt"
	"os"
	"path/filepath"

	"devbox/agentbox/internal/manifest"
	"devbox/agentbox/internal/store"
)

type Input struct {
	State         store.ReceiptState
	Workspace     string
	CandidateRoot string
	Stop          func() error
}

type Candidate struct {
	Dir      string
	Manifest manifest.Manifest
}

func Prepare(in Input) (Candidate, error) {
	switch in.State {
	case store.RemoteRunning, store.RemoteStarting, store.UnknownRemoteRun, store.RemoteOwned:
		return Candidate{}, fmt.Errorf("prepare_return rejects receipt state %s", in.State)
	}
	if in.Stop == nil {
		return Candidate{}, fmt.Errorf("cgroup stop is required")
	}
	if err := in.Stop(); err != nil {
		return Candidate{}, fmt.Errorf("cgroup stop failed: %w", err)
	}
	declared, err := manifest.Build(in.Workspace, manifest.Policy{})
	if err != nil {
		return Candidate{}, fmt.Errorf("build return manifest: %w", err)
	}
	dir := filepath.Join(in.CandidateRoot, "candidate")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Candidate{}, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return Candidate{}, err
	}
	defer root.Close()
	if err := manifest.Materialize(in.Workspace, declared, root); err != nil {
		return Candidate{}, fmt.Errorf("materialize candidate: %w", err)
	}
	if err := manifest.Compare(declared, mustBuild(dir)); err != nil {
		return Candidate{}, fmt.Errorf("candidate verification: %w", err)
	}
	return Candidate{Dir: dir, Manifest: declared}, nil
}

func mustBuild(dir string) manifest.Manifest {
	m, err := manifest.Build(dir, manifest.Policy{})
	if err != nil {
		return manifest.Manifest{}
	}
	return m
}
