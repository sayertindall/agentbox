package environment

import (
	"fmt"
	"os"
	"strings"
)

const ListenNetwork = "unix"

type Spec struct {
	Workspace     string
	GitDir        string
	GitWorkTree   string
	HandoffFile   string
	EnvironmentID string
	ExpectedID    string
	MarkerPath    string
	Nonce         string
}

func Env(spec Spec) (map[string]string, error) {
	if spec.EnvironmentID == "" || spec.EnvironmentID != spec.ExpectedID {
		return nil, fmt.Errorf("environment identity does not match")
	}
	for _, path := range []string{spec.Workspace, spec.GitDir, spec.GitWorkTree, spec.HandoffFile, spec.MarkerPath} {
		if path == "" || strings.Contains(path, "://") {
			return nil, fmt.Errorf("wrapper does not accept a network-provided path")
		}
		if !strings.HasPrefix(path, "/") {
			return nil, fmt.Errorf("wrapper paths must be server-resolved absolute paths")
		}
	}
	if spec.GitDir == spec.Workspace+"/.git" {
		return nil, fmt.Errorf("GIT_DIR must be outside the source workspace")
	}
	if strings.HasPrefix(spec.HandoffFile, spec.Workspace+"/") {
		return nil, fmt.Errorf("handoff file must be outside the source workspace")
	}
	if spec.MarkerPath != "" && spec.Nonce != "" {
		if err := os.WriteFile(spec.MarkerPath, []byte(spec.Nonce), 0o600); err != nil {
			return nil, err
		}
	}
	return map[string]string{
		"DEVBOX_WORKSPACE":     spec.Workspace,
		"GIT_DIR":              spec.GitDir,
		"GIT_WORK_TREE":        spec.GitWorkTree,
		"DEVBOX_HANDOFF_FILE":  spec.HandoffFile,
		"PASEO_HOME":           "",
	}, nil
}
